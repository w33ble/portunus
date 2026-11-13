# Portunus

Dual-pane file transfer client for SFTP and S3. Built with [Wails v3](https://v3.wails.io) (Go backend, React + TypeScript frontend). macOS first, Linux supported.

Status: MVP feature-complete on the `feat/mvp` branch and covered by an automated end-to-end smoke test. Native window behavior (drag and drop in WebKit, Finder drops, titlebar) still needs a run on a real desktop. See [docs/mvp-plan.md](docs/mvp-plan.md).

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

Tooling is pinned in `mise.toml` (Go, Node, pnpm, wails3 CLI, golangci-lint, zig):

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
mise run test:race    # Go tests under the race detector (zig cc supplies cgo)
mise run e2e          # end-to-end smoke (Linux, needs Docker and /usr/sbin/sshd)
mise run fmt          # format Go and frontend
mise run bindings     # regenerate frontend/bindings from Go services
```

`frontend/bindings` and `frontend/dist` are generated and gitignored. `main.go` embeds `frontend/dist`, so Go compiles only after a frontend build; `mise run test` handles that.

Go lint and tests run with the `purego` build tag. That's Wails' CGO-free backend, so they work on machines without GTK/WebKit headers. Real builds (`wails3 build`) use cgo.

### Tests

- Go unit and integration tests run against real servers. SFTP tests start an unprivileged OpenSSH `sshd`. S3 tests start MinIO in Docker and skip when Docker isn't available. A shared conformance suite (`internal/vfs/vfstest`) runs against every backend.
- Frontend tests use vitest (`pnpm --dir frontend test`).
- `mise run e2e` builds the app in Wails server mode (`-tags server`), starts sshd and MinIO with throwaway fixtures, and drives the real UI with Playwright in Docker: connect, trust a host key, copy with conflicts, drag, delete, rename, and an S3 round trip. Screenshots land in `e2e/out/`.

### Environment

- `PORTUNUS_SSH_CONFIG`: use this ssh config file instead of `~/.ssh/config`. It's passed to `ssh -G -F`, which also skips `/etc/ssh/ssh_config`.
- `PORTUNUS_MINIO_IMAGE`: MinIO image for the S3 tests and e2e.

App data lives in `~/Library/Application Support/Portunus` (macOS) or `~/.config/Portunus` (Linux): `settings.json`, `profiles.json`, and a fallback `known_hosts`.

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
internal/profiles        S3 profiles + keychain secrets
internal/settings        user preferences
frontend/                React + Vite app (pnpm, oxlint, oxfmt, vitest)
e2e/                     end-to-end harness
build/                   platform build assets and Taskfiles
```
