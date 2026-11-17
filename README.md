<p align="center">
  <img src="docs/icon-512.png" width="160" alt="Portunus icon: the Temple of Portunus">
</p>

<h1 align="center">Portunus</h1>

Dual-pane file transfer client for SFTP and S3. Built with [Wails v3](https://v3.wails.io) (Go backend, React + TypeScript frontend). macOS first, Linux supported.

Status: MVP. It runs on macOS and Linux. Builds are on the [Releases](https://github.com/w33ble/portunus/releases) page.

## Features

- Two panes, each on any connection: local disk, an SSH host, or an S3 bucket.
- Back and forward through each pane's folder history: the header arrows, ⌘[ / ⌘] (Alt+← / Alt+→ on Linux), the mouse's back and forward buttons, or a trackpad swipe. On macOS the swipe follows "Swipe between pages" in System Settings → Trackpad (two fingers, three fingers, or both); on Linux it's a two-finger sideways swipe. Settings can turn swipes off. Going back reselects what was selected in that folder. Buttons and swipes act on the pane under the pointer, keys on the active pane. Changing a pane's connection starts a new history.
- Type to find: in a file list, typing a name jumps to the first entry that starts with it, as in Finder. ⌘F (Ctrl+F) opens a filter that narrows the list with fuzzy, case- and accent-insensitive matching, best matches first. Typing `/` or `~` starts a path.
- Quick preview: Space shows the entry under the cursor in place of the other pane, on any connection. Text and code show the first 1 MB, images up to 20 MB, and PDFs up to 50 MB when the webview has a PDF viewer. Code and config files are syntax highlighted (JSON, YAML, TOML, INI, XML and HTML, CSS, Markdown, shell, Go, TypeScript and JavaScript, Python, Rust, C and C++, Java, Ruby, PHP, SQL, Dockerfile, Makefile, diff, nginx), chosen from the file name, extension or a shebang line, never by guessing from content. Anything else, and text over 200 KB, stays plain. The highlighter loads on first use, and HTML always shows as source. Folders show how many items they hold and the total size of their files. Binary and oversized files show metadata only. The preview follows the arrow keys and reads only the entry the cursor stops on. Space or Esc closes it.
- SSH hosts come from `~/.ssh/config`, including `Include` and `Match`. Settings are resolved by `ssh -G`, so behavior matches the `ssh` command.
- Manage connections (from the connection picker): one window lists Local, every SSH host and every S3 profile. Anything that comes from `~/.ssh/config` is shown read-only; edit that file to change a host. Each connection can have a colour tag, a folder it opens at instead of home, its own parallel-transfer limit, its own hidden-files setting, and can be hidden from the picker. A quick connect can be saved as a named host, which you can rename, re-point or delete later. Edits are saved explicitly, and leaving a connection with unsaved changes asks first.
- Bookmarks: ⌘D (Ctrl+D) bookmarks the pane's folder or removes its bookmark. The bookmarks button in the pane header lists favourites (pinned bookmarks from any connection, which connect first when needed), the connection's bookmarks and the folders visited on it this session. Bookmarks survive restarts; an unsaved quick connect has to be saved before its folders can be bookmarked.
- Authentication through ssh-agent, `IdentityAgent` (1Password, Secretive), and key files (passphrase prompted only when needed), then keyboard-interactive and password. `ProxyJump` works; `ProxyCommand` doesn't yet.
- Host keys are checked against `known_hosts`. Unknown keys get a trust prompt and are saved to `~/.ssh/known_hosts`, or to the app's own file if that isn't writable. A changed key always blocks the connection.
- On macOS, keys stored with `ssh-add --apple-use-keychain` are loaded into the agent before any passphrase prompt.
- S3: AWS shared profiles or access keys, custom endpoints and path-style addressing for S3-compatible services. New profiles can start from a preset for Amazon S3, Backblaze B2, Cloudflare R2, DigitalOcean Spaces, Hetzner, IDrive e2, Scaleway, Wasabi or a local MinIO/RustFS: pick a region and the endpoint and addressing are filled in. Secrets go in the OS keychain.
- Transfers: drag between panes, drop from Finder, or press F5. Directories copy recursively, with parallel transfers (4 total and 2 per connection by default, adjustable in the Transfers tab, and per connection in Manage connections). Files are written to a temp name and renamed into place, so a failed or cancelled transfer leaves nothing half-written.
- Move: F6 or "Move to other pane" moves the selection to the other pane's folder. Holding Cmd (macOS) or Shift (Linux) while dragging moves instead of copies, onto a pane or onto a folder name. Within one connection a move is a rename: instant, and nothing is copied. Between connections, and for S3 folders, each file is copied first and deleted from the source only after the copy is in place. A folder is removed once everything in it has moved, so a canceled, failed or skipped move leaves every unmoved item where it was.
- When a destination already exists, you choose overwrite, skip, or cancel, with an option to apply the choice to every conflict in that transfer. A skipped item stays at its source when moving.
- Themes: Settings → Appearance lists System (Portunus Light or Dark, following the OS appearance live) and light and dark versions of Portunus, Solarized, Nord, Catppuccin (Latte and Mocha), Everforest and Tokyo Night (Day and Night), each shown as a small preview. A pick applies at once and is remembered; the window opens in it with no flash of the default colours.

## Install

Download the latest build from [Releases](https://github.com/w33ble/portunus/releases).

- **macOS** (Apple silicon and Intel): open the `.dmg` and drag Portunus to Applications. The app isn't signed or notarized, so macOS quarantines it and **Open Anyway** in Privacy & Security isn't enough. Clear the quarantine flag once in Terminal, then open it as usual:

  ```sh
  xattr -dr com.apple.quarantine /Applications/Portunus.app
  ```

- **Debian/Ubuntu**: `sudo apt install ./portunus_*.deb`. **Fedora/RHEL**: `sudo dnf install ./portunus-*.rpm`. Needs GTK 4 and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13+, Fedora 40+).
- **Other Linux**: the `.tar.gz` contains the binary.

## Requirements

Tooling is pinned in `mise.toml` (Go, Node, pnpm, wails3 CLI, golangci-lint, gohawk, govulncheck):

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
mise run lint:vuln    # govulncheck: known vulnerabilities in Go code the app calls
mise run deps:outdated  # available updates for Go modules compiled into the app
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
- `mise run e2e` builds the app in Wails server mode (`-tags server`), starts sshd and RustFS with throwaway fixtures, and drives the real UI with Playwright in Docker: connect, trust a host key, copy with conflicts, drag, move between local, SFTP and S3 and within one connection, delete, rename, an S3 round trip, quick previews of text, highlighted code, an image and a PDF on local, SFTP and S3, back/forward by swipe, mouse button and keyboard, and picking a theme (applied at once, kept after a reload, and painted before the app's script loads). Screenshots land in `e2e/out/`.
- The same run opens every dialog, the browser with a selection, the Transfers view, and a highlighted Go, diff and Dockerfile preview, in every theme and checks contrast: axe-core's `color-contrast` rule for enabled text, plus a custom pass for what axe skips (disabled controls at least 3:1, placeholders 4.5:1, icon buttons 3:1). The measurements land in `e2e/out/contrast.json`. Run it alone with `E2E_FLOWS=contrast mise run e2e`; flows are the names in `e2e/flows/`.

### CI

GitHub Actions (`.github/workflows`):

- `ci.yml` runs on every pull request, every push to `main`, and weekly: `mise run check`, the race detector, gohawk, govulncheck, and a native cgo build on Ubuntu (GTK4/WebKitGTK 6). The govulncheck job also writes `mise run deps:outdated` to its job summary. macOS builds aren't in CI; they're checked locally.
- `e2e.yml` runs the end-to-end suite on pushes to `main` and on manual dispatch. Screenshots are uploaded as an artifact.
- `release.yml` cuts releases (see below). Both `ci.yml` and `e2e.yml` are reusable (`workflow_call`), and the release requires both to pass.

CI sets `PORTUNUS_REQUIRE_INTEGRATION=1`, which turns integration-test skips (missing sshd, ssh, or Docker) into failures.

### Versions and releases

Every build is stamped with a version: `portunus --version`, the About box and Settings all show it.

- **Dev builds** (`mise run dev`, `mise run build`, `mise run package`) are `dev-<commit>`, with `-dirty` when the working tree has uncommitted changes. Linux packages use `0.0.0-dev+<commit>` so they still sort.
- **Releases** set `PORTUNUS_VERSION` (e.g. `1.2.0`). The release workflow does this from the tag. The version goes into the binary, the macOS `Info.plist`, and the `.deb`/`.rpm` metadata. `build/config.yml`'s `version` is only a template default.

To cut a release, push a semver tag:

```sh
git tag v0.2.0 && git push origin v0.2.0
```

`release.yml` runs CI and e2e, then builds:

- macOS: a universal `.app` (arm64 + x86_64), as a `.zip` and a `.dmg`. Ad-hoc signed only, so users clear the quarantine flag before the first launch (see [Install](#install)).
- Linux: `.deb`, `.rpm` and `.tar.gz` for amd64, plus arm64 when the repo is public (hosted arm64 runners are free only for public repos).
- `THIRD_PARTY_NOTICES.md` (`build/release/third-party-notices.sh`), which is also bundled in the `.app` and the Linux packages, and `SHA256SUMS`.

It then opens a **draft** GitHub Release with generated notes, for you to review and publish. Tags with a pre-release suffix (`v0.2.0-rc.1`) are marked as pre-releases. For a dry run, use Actions → Release → Run workflow with a version: everything is built and uploaded as workflow artifacts, and nothing is published.

### Environment

- `PORTUNUS_SSH_CONFIG`: use this ssh config file instead of `~/.ssh/config`. It's passed to `ssh -G -F`, which also skips `/etc/ssh/ssh_config`.
- `PORTUNUS_S3_IMAGE`: S3-compatible server image for the S3 tests and e2e (default `rustfs/rustfs:1.0.5`).
- `PORTUNUS_REQUIRE_INTEGRATION`: when set, integration tests fail instead of skipping if sshd, ssh, or Docker is missing.

### App data

The XDG layout is used on macOS and Linux alike. A relative `XDG_*` value is ignored, as the spec says.

| | Location | Contents |
|---|---|---|
| Config | `$XDG_CONFIG_HOME/portunus`, else `~/.config/portunus` | `settings.json`, `profiles.json` (S3 profiles), `connections.json` (saved SSH hosts, per-connection settings, bookmarks), fallback `known_hosts` |
| State | `$XDG_STATE_HOME/portunus`, else `~/.local/state/portunus` | `state.json`: the main window's size, position and maximised flag |

Files are JSON, written atomically. Secrets (S3 secret keys) stay in the OS keychain, never in these files. The window reopens where you left it; if its monitor is gone it is centered on the primary screen. On Linux Wayland only the size is restored, because compositors place windows themselves.

## Layout

```
main.go                  Wails bootstrap, file-drop forwarding
internal/services        Wails-bound services, connection registry, error mapping
internal/transfer        transfer engine: jobs, scheduling, conflicts, progress
internal/preview         quick preview: type detection, size limits, text truncation
internal/swipe           macOS swipe gestures (AppKit event monitor) and the "Swipe between pages" setting
internal/vfs             filesystem contract + conformance suite
internal/vfs/localfs     local disk
internal/vfs/sftpfs      SFTP (pkg/sftp)
internal/vfs/s3fs        S3 (aws-sdk-go-v2, own multipart writer)
internal/sshcfg          ~/.ssh/config host list, ssh -G resolution
internal/sshconn         SSH dialing, auth, known_hosts, ProxyJump
internal/prompt          backend → UI questions (host key, secrets, conflicts)
internal/appdirs         config and state directories (XDG on every platform)
internal/profiles        S3 profiles + keychain secrets
internal/connprefs       saved SSH hosts, per-connection settings, bookmarks
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
