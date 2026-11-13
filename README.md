<p align="center">
  <img src="docs/icon-512.png" width="160" alt="Portunus icon: the Temple of Portunus">
</p>

<h1 align="center">Portunus</h1>

Dual-pane file transfer client for SFTP and S3. Built with [Wails v3](https://v3.wails.io) (Go backend, React + TypeScript frontend). macOS first, Linux supported.

Status: MVP. It runs on macOS and Linux; there are no packaged releases yet. See [docs/mvp-plan.md](docs/mvp-plan.md).

## Features

- Two panes, each on any connection: local disk, an SSH host, or an S3 bucket.
- SSH hosts come from `~/.ssh/config`, including `Include` and `Match`. Settings are resolved by `ssh -G`, so behavior matches the `ssh` command.
- Authentication through ssh-agent, `IdentityAgent` (1Password, Secretive), and key files (passphrase prompted only when needed), then keyboard-interactive and password. `ProxyJump` works; `ProxyCommand` doesn't yet.
- Host keys are checked against `known_hosts`. Unknown keys get a trust prompt and are saved to `~/.ssh/known_hosts`, or to the app's own file if that isn't writable. A changed key always blocks the connection.
- On macOS, keys stored with `ssh-add --apple-use-keychain` are loaded into the agent before any passphrase prompt.
- S3: AWS shared profiles or access keys, custom endpoints and path-style addressing for S3-compatible services. Secrets go in the OS keychain.
- Transfers: drag between panes, drop from Finder, or press F5. Directories copy recursively, with parallel transfers (4 total and 2 per connection by default, adjustable in the Transfers tab). Files are written to a temp name and renamed into place, so a failed or cancelled transfer leaves nothing half-written.
- When a destination already exists, you choose overwrite, skip, or cancel, with an option to apply the choice to every conflict in that transfer.

## Requirements

Tooling is pinned in `mise.toml` (Go, Node, pnpm, wails3 CLI, golangci-lint, gohawk):

```sh
mise install
```

Platform dependencies:

- **macOS**: Xcode Command Line Tools (`xcode-select --install`).
- **Linux**: `build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev` (Debian/Ubuntu names). `wails3 doctor` lists what's missing.

## Development

```sh
mise run dev          # app with hot reload (wails3 dev)
mise run build        # production binary in bin/
mise run package      # .app/.dmg on macOS, deb/rpm on Linux
mise run check        # lint + typecheck + Go and frontend tests + format check
mise run test:race    # Go tests under the race detector (needs a C compiler; CI always runs it)
mise run lint:gohawk  # gohawk static analysis: goroutine, lock, and resource-lifetime bugs
mise run e2e          # end-to-end smoke (Linux, needs Docker and /usr/sbin/sshd)
mise run fmt          # format Go and frontend
mise run bindings     # regenerate frontend/bindings from Go services
mise run icons        # regenerate app icons from build/icon/ (needs ImageMagick 7)
```

App icon sources live in `build/icon/`: `master-1024.png` (transparent artwork) and `portunus.svg` (the layer of the macOS 26 Icon Composer project in `build/appicon.icon`). `mise run icons` renders the tile used for `.icns`, `.ico`, the DMG, and Linux. `build/darwin/Assets.car`, the compiled macOS 26 icon, can only be produced on macOS. Builds regenerate it there, so commit it after a macOS build when the icon changes.

`frontend/bindings` and `frontend/dist` are generated and gitignored. `main.go` embeds `frontend/dist`, so Go compiles only after a frontend build; `mise run test` handles that.

Go lint and tests run with the `purego` build tag. That's Wails' CGO-free backend, so they work on machines without GTK/WebKit headers. Real builds (`wails3 build`) use cgo.

### Tests

- Go unit and integration tests run against real servers. SFTP tests start an unprivileged OpenSSH `sshd`. S3 tests start RustFS (an S3-compatible server) in Docker and skip when Docker isn't available. A shared conformance suite (`internal/vfs/vfstest`) runs against every backend.
- Frontend tests use vitest (`pnpm --dir frontend test`).
- `mise run e2e` builds the app in Wails server mode (`-tags server`), starts sshd and RustFS with throwaway fixtures, and drives the real UI with Playwright in Docker: connect, trust a host key, copy with conflicts, drag, delete, rename, and an S3 round trip. Screenshots land in `e2e/out/`.

### CI

GitHub Actions (`.github/workflows`):

- `ci.yml` runs on every pull request and every push to `main`: `mise run check`, the race detector, gohawk, and a native cgo build on Ubuntu (GTK4/WebKitGTK 6). macOS builds aren't in CI; they're checked locally.
- `e2e.yml` runs the end-to-end suite on pushes to `main` and on manual dispatch. It's also callable via `workflow_call`, so a release workflow can require it. Screenshots are uploaded as an artifact.

CI sets `PORTUNUS_REQUIRE_INTEGRATION=1`, which turns integration-test skips (missing sshd, ssh, or Docker) into failures.

### Environment

- `PORTUNUS_SSH_CONFIG`: use this ssh config file instead of `~/.ssh/config`. It's passed to `ssh -G -F`, which also skips `/etc/ssh/ssh_config`.
- `PORTUNUS_S3_IMAGE`: S3-compatible server image for the S3 tests and e2e (default `rustfs/rustfs:1.0.5`).
- `PORTUNUS_REQUIRE_INTEGRATION`: when set, integration tests fail instead of skipping if sshd, ssh, or Docker is missing.

### App data

The XDG layout is used on macOS and Linux alike. A relative `XDG_*` value is ignored, as the spec says.

| | Location | Contents |
|---|---|---|
| Config | `$XDG_CONFIG_HOME/portunus`, else `~/.config/portunus` | `settings.json`, `profiles.json`, fallback `known_hosts` |
| State | `$XDG_STATE_HOME/portunus`, else `~/.local/state/portunus` | `state.json`: the main window's size, position and maximised flag |

Files are JSON, written atomically. Secrets (S3 secret keys) stay in the OS keychain, never in these files. The window reopens where you left it; if its monitor is gone it is centered on the primary screen. On Linux Wayland only the size is restored, because compositors place windows themselves.

## Layout

```
main.go                  Wails bootstrap, file-drop forwarding
internal/services        Wails-bound services, connection registry, error mapping
internal/transfer        transfer engine: jobs, scheduling, conflicts, progress
internal/vfs             filesystem contract + conformance suite
internal/vfs/localfs     local disk
internal/vfs/sftpfs      SFTP (pkg/sftp)
internal/vfs/s3fs        S3 (aws-sdk-go-v2, own multipart writer)
internal/sshcfg          ~/.ssh/config host list, ssh -G resolution
internal/sshconn         SSH dialing, auth, known_hosts, ProxyJump
internal/prompt          backend → UI questions (host key, secrets, conflicts)
internal/appdirs         config and state directories (XDG on every platform)
internal/profiles        S3 profiles + keychain secrets
internal/settings        user preferences
internal/state           window geometry: persistence, placement on connected screens
internal/atomicfile      temp-file-and-rename writes
frontend/                React + Vite app (pnpm, oxlint, oxfmt, vitest)
e2e/                     end-to-end harness
build/                   platform build assets and Taskfiles
```

## License

Portunus is licensed under the [Mozilla Public License 2.0](LICENSE) (MPL-2.0). You can use it, and combine it with code under other licenses, but changes to Portunus's own source files must be shared under the same license.

Per MPL-2.0 Exhibit A, this notice covers every source file in the repository:

> This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
