# Portunus

Dual-pane file transfer client for SFTP and S3. Built with [Wails v3](https://v3.wails.io) (Go backend, React + TypeScript frontend).

Status: scaffold. See [docs/mvp-plan.md](docs/mvp-plan.md).

## Requirements

Tooling is pinned in `mise.toml` (Go, Node, pnpm, wails3 CLI, golangci-lint):

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
mise run package      # .app/.dmg on macOS, AppImage/deb/rpm on Linux
mise run check        # lint + typecheck + tests + format check (CI entry point)
mise run fmt          # format Go and frontend
mise run bindings     # regenerate frontend/bindings from Go services
```

`frontend/bindings` and `frontend/dist` are generated and gitignored. `main.go` embeds `frontend/dist`, so Go compiles only after a frontend build; `mise run test` handles that.

Go lint and tests run with the `purego` build tag. That's Wails' CGO-free backend, so they work on machines without GTK/WebKit headers. Real builds (`wails3 build`) use cgo.

## Layout

```
main.go           Wails bootstrap
build/            Platform build assets and Taskfiles (wails3 build/package)
frontend/         React + Vite app (pnpm, oxlint, oxfmt)
docs/             Plans and design notes
```
