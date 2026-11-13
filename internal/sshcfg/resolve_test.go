package sshcfg

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testResolver(t *testing.T) Resolver {
	t.Helper()
	ssh, err := LocateSSH()
	if err != nil {
		t.Skip("ssh not available")
	}
	home := fixtureHome(t)
	return Resolver{
		SSHPath:    ssh,
		ConfigPath: filepath.Join(home, ".ssh", "resolve.conf"),
		Home:       home,
		LocalUser:  "localu",
	}
}

func TestResolveAlias(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	o, err := r.Resolve(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	ssh := filepath.Join(r.Home, ".ssh")
	if o.Alias != "web" || o.HostName != "h.example.com" || o.User != "bob" || o.Port != 2200 {
		t.Errorf("basics = %+v", o)
	}
	wantIDs := []string{
		filepath.Join(ssh, "id_here"),
		filepath.Join(ssh, "id_pubonly"), // only .pub exists: agent-backed key
		filepath.Join(ssh, "key_h.example.com_bob_2200"),
	}
	if !equal(o.IdentityFiles, wantIDs) {
		t.Errorf("IdentityFiles = %q, want %q", o.IdentityFiles, wantIDs)
	}
	if want := []string{filepath.Join(ssh, "id_here-cert.pub")}; !equal(o.CertificateFiles, want) {
		t.Errorf("CertificateFiles = %q", o.CertificateFiles)
	}
	checkAliasPolicy(t, o, ssh)
}

func checkAliasPolicy(t *testing.T, o Options, ssh string) {
	t.Helper()
	if o.HostKeyAlias != "al-h.example.com-web" || o.KnownHostsKey() != o.HostKeyAlias {
		t.Errorf("HostKeyAlias = %q", o.HostKeyAlias)
	}
	if got := o.UserKnownHostsFiles; len(got) != 2 || filepath.Base(got[0]) != "kh1" || filepath.Base(got[1]) != "kh 2" {
		t.Errorf("UserKnownHostsFiles = %q", got)
	}
	checkAliasConn(t, o)
}

func checkAliasConn(t *testing.T, o Options) {
	t.Helper()
	if len(o.GlobalKnownHostsFiles) == 0 {
		t.Errorf("GlobalKnownHostsFiles empty")
	}
	if !strings.HasSuffix(filepath.ToSlash(o.IdentityAgent), "agents/h.example.com.sock") {
		t.Errorf("IdentityAgent = %q", o.IdentityAgent)
	}
	if o.ProxyJump != "jump@bastion:2222" || o.ProxyCommand != "" {
		t.Errorf("proxy = %q / %q", o.ProxyJump, o.ProxyCommand)
	}
	if o.ConnectTimeout != 7*time.Second || o.ServerAliveInterval != 15*time.Second || o.ServerAliveCountMax != 4 {
		t.Errorf("timeouts = %v %v %d", o.ConnectTimeout, o.ServerAliveInterval, o.ServerAliveCountMax)
	}
	if o.StrictHostKeyChecking != "accept-new" || !o.HashKnownHosts || !o.IdentitiesOnly {
		t.Errorf("flags = %q %v %v", o.StrictHostKeyChecking, o.HashKnownHosts, o.IdentitiesOnly)
	}
}

func TestResolveDefaultsAndMissingIdentities(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	o, err := r.Resolve(context.Background(), "agentenv")
	if err != nil {
		t.Fatal(err)
	}
	if o.IdentityAgent != "" {
		t.Errorf("IdentityAgent = %q, want empty for SSH_AUTH_SOCK", o.IdentityAgent)
	}
	if len(o.IdentityFiles) != 0 {
		t.Errorf("default identity files not in fixture home must be filtered: %q", o.IdentityFiles)
	}
	if o.ConnectTimeout != 0 || o.HostKeyAlias != "" || o.ProxyJump != "" {
		t.Errorf("unexpected defaults: %+v", o)
	}
	if o.HostName != "agentenv" || o.Port != 22 {
		t.Errorf("host/port = %q %d", o.HostName, o.Port)
	}
}

func TestResolveIdentityAgentNone(t *testing.T) {
	t.Parallel()
	o, err := testResolver(t).Resolve(context.Background(), "agentnone")
	if err != nil {
		t.Fatal(err)
	}
	if o.IdentityAgent != "none" {
		t.Errorf("IdentityAgent = %q", o.IdentityAgent)
	}
}

func TestResolveProxyCommandPassthrough(t *testing.T) {
	t.Parallel()
	o, err := testResolver(t).Resolve(context.Background(), "pc")
	if err != nil {
		t.Fatal(err)
	}
	if o.ProxyCommand != "nc %h %p" {
		t.Errorf("ProxyCommand = %q", o.ProxyCommand)
	}
}

func TestResolveMatchExecConfig(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	hosts, err := listHosts(r.ConfigPath, r.Home)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) == 0 {
		t.Fatal("listing a config with Match exec returned nothing")
	}
	o, err := r.Resolve(context.Background(), "mexec")
	if err != nil {
		t.Fatal(err)
	}
	if o.User == "never" {
		t.Error("Match exec \"false\" block applied")
	}
}

func TestResolveQuick(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	o, err := r.ResolveQuick(context.Background(), "zed", "quick.example.com", 2022)
	if err != nil {
		t.Fatal(err)
	}
	if o.User != "zed" || o.HostName != "quick.example.com" || o.Port != 2022 || o.Alias != "quick.example.com" {
		t.Errorf("quick = %+v", o)
	}
	o, err = r.ResolveQuick(context.Background(), "", "web", 0)
	if err != nil {
		t.Fatal(err)
	}
	if o.HostName != "h.example.com" || o.Port != 2200 || o.User != "bob" {
		t.Errorf("config applies to quick host: %+v", o)
	}
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	if _, err := r.Resolve(context.Background(), "-oProxyCommand=evil"); err == nil {
		t.Error("option-looking destination accepted")
	}
	if _, err := r.Resolve(context.Background(), ""); err == nil {
		t.Error("empty destination accepted")
	}
	bad := r
	bad.ConfigPath = filepath.Join(t.TempDir(), "missing")
	if _, err := bad.Resolve(context.Background(), "web"); err == nil {
		t.Error("missing -F config accepted")
	}
	if _, err := (Resolver{}).Resolve(context.Background(), "web"); err == nil {
		t.Error("empty SSHPath accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Resolve(ctx, "web"); err == nil {
		t.Error("canceled context accepted")
	}
}

// ssh -G expands tokens in ControlPath itself, so it is ground truth for our expander.
func TestTokenExpansionMatchesSSH(t *testing.T) {
	t.Parallel()
	r := testResolver(t)
	cmdOut, err := r.runRaw(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	var ctl string
	for l := range strings.SplitSeq(cmdOut, "\n") {
		if v, ok := strings.CutPrefix(l, "controlpath "); ok {
			ctl = v
		}
	}
	if ctl == "" {
		t.Fatal("no controlpath in ssh -G output")
	}
	o, err := r.Resolve(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	// the fixture's identity template mirrors ControlPath's token set under a different prefix
	e := expander{home: r.Home, user: r.LocalUser, host: o.HostName, port: o.Port, remote: o.User, alias: "tok"}
	e.init()
	u, err := user.Current()
	if err == nil {
		e.user = u.Username // ssh substitutes the real local user
	}
	got := e.tokens("/ctl/%n_%l_%L_%i_%u_%C_%%_%h_%p_%r")
	if got != ctl {
		t.Fatalf("expander = %q\nssh -G   = %q", got, ctl)
	}
}

func TestExpandTokens(t *testing.T) {
	t.Parallel()
	e := expander{home: "/h", user: "lu", host: "host", port: 22, remote: "ru", alias: "al"}
	e.init()
	cases := map[string]string{
		"%h|%p|%r|%u|%n|%d|%%": "host|22|ru|lu|al|/h|%",
		"%z-%":                 "%z-%",
		"plain":                "plain",
	}
	for in, want := range cases {
		if got := e.tokens(in); got != want {
			t.Errorf("tokens(%q) = %q, want %q", in, got, want)
		}
	}
	if got := e.path("~/x/%h"); got != filepath.Join("/h", "x", "host") {
		t.Errorf("path = %q", got)
	}
}

func TestIdentityAgentEnvForms(t *testing.T) {
	t.Setenv("PORTUNUS_TEST_SOCK", "/run/x.sock")
	e := expander{home: "/h"}
	for in, want := range map[string]string{
		"":                         "",
		"SSH_AUTH_SOCK":            "",
		"$SSH_AUTH_SOCK":           "",
		"none":                     "none",
		"$PORTUNUS_TEST_SOCK":      "/run/x.sock",
		"$PORTUNUS_TEST_UNSET_VAR": "none",
		"/a/b.sock":                "/a/b.sock",
	} {
		if got := e.agent(in); got != want {
			t.Errorf("agent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathListWithSpaces(t *testing.T) {
	t.Parallel()
	e := expander{home: "/h"}
	got := e.pathList("/a/my keys/kh /b ~/c")
	want := []string{"/a/my keys/kh", "/b", filepath.Join("/h", "c")}
	if !equal(got, want) {
		t.Errorf("pathList = %q, want %q", got, want)
	}
}

func TestParseSeconds(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{"": 0, "none": 0, "NONE": 0, "30": 30 * time.Second} {
		got, err := parseSeconds(in)
		if err != nil || got != want {
			t.Errorf("parseSeconds(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := parseSeconds("1h"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestLocateSSH(t *testing.T) {
	t.Parallel()
	p, err := LocateSSH()
	if err != nil {
		t.Skip("ssh not available")
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("LocateSSH returned %q: %v", p, err)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()
	p, err := DefaultConfigPath()
	if err != nil {
		t.Skip("no home")
	}
	if filepath.Base(p) != "config" || filepath.Base(filepath.Dir(p)) != ".ssh" {
		t.Errorf("path = %q", p)
	}
}
