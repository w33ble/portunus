// Package services wires backend packages together and exposes them to the frontend
// as Wails-bound services. Services stay thin: validation and orchestration only.
package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/settings"
	"github.com/w33ble/portunus/internal/sshcfg"
	"github.com/w33ble/portunus/internal/sshconn"
	"github.com/w33ble/portunus/internal/swipe"
	"github.com/w33ble/portunus/internal/transfer"
	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/localfs"
	"github.com/w33ble/portunus/internal/vfs/s3fs"
	"github.com/w33ble/portunus/internal/vfs/sftpfs"
)

// Emitter delivers an event to the frontend.
type Emitter func(name string, data any)

type Config struct {
	// ConfigDir holds settings.json, profiles.json, and the fallback known_hosts.
	ConfigDir string
	Emit      Emitter
	Secrets   profiles.SecretStore
	Logger    *slog.Logger
	// ThemeChanged, if set, is called with the new theme ID after a save changes it.
	ThemeChanged func(id string)
}

// App owns every backend component for one running application.
type App struct {
	emit Emitter
	log  *slog.Logger

	themeChanged func(id string)

	configDir string
	secrets   profiles.SecretStore
	broker    *prompt.Broker
	settings  *settings.Store
	profiles  *profiles.Store
	resolver  sshcfg.Resolver
	sshErr    error  // non-nil when no ssh binary was found
	sshConfig string // explicit config file (PORTUNUS_SSH_CONFIG); "" means ssh's defaults
	dialer    atomic.Pointer[sshconn.Dialer]
	reg       *registry
	lists     *listings
	engine    *transfer.Engine
}

func New(cfg Config) (*App, error) {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	a := &App{emit: cfg.Emit, log: log, themeChanged: cfg.ThemeChanged, configDir: cfg.ConfigDir, secrets: cfg.Secrets}
	a.broker = prompt.New(prompt.Emitter(cfg.Emit))

	var err error
	if a.settings, err = settings.Open(cfg.ConfigDir); err != nil {
		return nil, err
	}
	if a.profiles, err = profiles.Open(cfg.ConfigDir, cfg.Secrets); err != nil {
		return nil, err
	}

	sshPath, err := sshcfg.LocateSSH()
	if err != nil {
		a.sshErr = fmt.Errorf("OpenSSH client not found, so ~/.ssh/config hosts are unavailable (quick connect still works): %w", err)
	}
	// An explicit config is passed to `ssh -G -F`, which also skips /etc/ssh/ssh_config,
	// so it's opt-in for non-default setups and tests only.
	a.sshConfig = os.Getenv("PORTUNUS_SSH_CONFIG")
	a.resolver = sshcfg.Resolver{SSHPath: sshPath, ConfigPath: a.sshConfig}
	a.applySettings(a.settings.Get())

	a.lists = newListings(listIdleTimeout)
	a.reg = newRegistry(localfs.New(), a.dial, func(conns []Connection) {
		a.emit(EventConnections, conns)
	})

	cur := a.settings.Get()
	a.engine = transfer.New(transfer.Options{
		Resolve: func(id string) (vfs.FS, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			return a.reg.FS(ctx, id)
		},
		Conflicts: a.broker,
		Emit:      func(u transfer.Update) { a.emit(EventTransfers, u) },
		Limits:    transfer.Limits{Global: cur.GlobalTransfers, PerConn: cur.PerConnectionTransfers},
		Logger:    log,
	})

	return a, nil
}

// Services returns the Wails-bound services.
func (a *App) Services() []application.Service {
	return []application.Service{
		application.NewService(&ConnectionService{app: a}),
		application.NewService(&FileService{app: a}),
		application.NewService(&TransferService{app: a}),
		application.NewService(&PromptService{app: a}),
		application.NewServiceWithOptions(&PreviewService{app: a}, application.ServiceOptions{Route: PreviewRoute}),
		application.NewService(&SettingsService{app: a}),
		application.NewServiceWithOptions(&ThemeService{app: a}, application.ServiceOptions{Route: ThemeScriptRoute}),
		application.NewService(&VersionService{}),
	}
}

// Close cancels listings and transfers, fails pending prompts, and closes every connection.
func (a *App) Close() {
	a.broker.Close()
	a.lists.closeAll()
	a.engine.Close()
	a.reg.CloseAll()
}

// HandleDrop forwards OS file drops to the frontend, tagged with the drop target's pane and,
// when dropped onto a folder name, that folder's path.
func (a *App) HandleDrop(paths []string, attrs map[string]string) {
	if len(paths) == 0 {
		return
	}
	norm := make([]string, len(paths))
	for i, p := range paths {
		norm[i] = filepath.ToSlash(p)
	}
	a.emit(EventDrop, DropFiles{Paths: norm, Pane: attrs["data-pane"], Dir: attrs["data-path"]})
}

// HandleSwipe forwards an OS back/forward swipe to the frontend, which applies it to the
// pane under the pointer.
func (a *App) HandleSwipe(d swipe.Direction) {
	a.emit(EventSwipe, d)
}

func (a *App) applySettings(s settings.Settings) {
	a.dialer.Store(&sshconn.Dialer{
		Resolve:           a.resolver.Resolve,
		Prompter:          a.broker,
		AppKnownHosts:     filepath.Join(a.configDir, "known_hosts"),
		LoadAppleKeychain: s.LoadAppleKeychain,
		Logger:            a.log,
	})
	if a.engine != nil {
		a.engine.SetLimits(transfer.Limits{Global: s.GlobalTransfers, PerConn: s.PerConnectionTransfers})
	}
}

func (a *App) dial(ctx context.Context, t Target) (dialed, error) {
	switch t.Kind {
	case TargetSSH:
		return a.dialSSH(ctx, t)
	case TargetS3:
		return a.dialS3(ctx, t)
	case TargetLocal:
		return dialed{}, errors.New("local connection is always open")
	default:
		return dialed{}, fmt.Errorf("unknown target kind %q", t.Kind)
	}
}

func (a *App) dialSSH(ctx context.Context, t Target) (dialed, error) {
	quickNoSSH := a.sshErr != nil && t.Alias == ""
	if a.sshErr != nil && !quickNoSSH {
		return dialed{}, a.sshErr
	}

	var (
		opts  sshcfg.Options
		err   error
		label = t.Alias
	)
	switch {
	case t.Alias != "":
		opts, err = a.resolver.Resolve(ctx, t.Alias)
	case quickNoSSH:
		opts, err = quickOptionsForCurrentUser(t)
	default:
		opts, err = a.resolver.ResolveQuick(ctx, t.User, t.Host, t.Port)
	}
	if t.Alias == "" {
		label = t.Host
		if t.User != "" {
			label = t.User + "@" + t.Host
		}
	}
	if err != nil {
		return dialed{}, err
	}

	client, err := a.dialer.Load().Dial(ctx, opts)
	if err != nil {
		return dialed{}, err
	}

	fs, err := sftpfs.New(client)
	if err != nil {
		_ = client.Close()
		return dialed{}, fmt.Errorf("start sftp on %s: %w", label, err)
	}

	lost := make(chan struct{})
	go func() {
		_ = client.Wait()
		close(lost)
	}()

	return dialed{fs: fs, label: label, lost: lost}, nil
}

func (a *App) dialS3(ctx context.Context, t Target) (dialed, error) {
	p, ok := a.profiles.Get(t.ProfileID)
	if !ok {
		return dialed{}, fmt.Errorf("s3 profile %q not found", t.ProfileID)
	}

	cfg, err := a.profiles.S3Config(t.ProfileID)
	if err != nil {
		return dialed{}, err
	}

	fs, err := s3fs.New(ctx, cfg)
	if err != nil {
		return dialed{}, fmt.Errorf("connect to %s: %w", p.Name, err)
	}
	return dialed{fs: fs, label: p.Name}, nil
}
