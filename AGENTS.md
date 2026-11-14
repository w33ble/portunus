# Repository Guidelines

## Project Overview

Portunus is a dual-pane file transfer desktop app: each pane is local disk, an SFTP host (from `~/.ssh/config`) or an S3 bucket, and files move between panes by drag, drop or F5. Built on Wails v3: Go backend, React 19 + TypeScript frontend. macOS first, Linux supported, Windows untested. `docs/mvp-plan.md` is the frozen design record; its **Deviations** and **Decisions** sections hold later changes.

## Architecture & Data Flow

```
React (frontend/src) ──api/ wrappers──▶ generated bindings ──▶ internal/services (thin, Wails-bound)
        ▲                                                              │
        └──── push events (transfers, connections, drops, prompts) ◀───┤
                                                                       ▼
                         vfs (localfs | sftpfs | s3fs) ◀── transfer engine, prompt, sshconn, profiles, settings
```

- **Services stay thin** (`internal/services/app.go`): domain logic lives in `internal/{vfs,transfer,preview,prompt,sshcfg,sshconn,profiles,settings,state}`. `main.go` injects an `Emit` closure that services use to push events.
- **One `vfs.FS` contract** (`internal/vfs/vfs.go`) is the pivot for services, the engine and tests:
  - Paths are absolute and `/`-separated on every backend, Windows included. Use package `path`, never `filepath`.
  - Writers stage, then `Commit()` or `Abort()`. Commit refuses unless `Written() == opts.Size` (`ErrSizeMismatch`). SFTP stages to `.<name>.portunus-tmp` and commits with `posix-rename@openssh.com`.
  - Errors wrap `vfs.Err{NotExist,Exist,Permission,Unsupported}`. Branch with `errors.Is`, never on message text.
  - Copy with `WriterTo`/`ReaderFrom` or `io.CopyBuffer` (1 MiB). Plain `io.Copy` throttles SFTP badly.
- **Connections** (`services/registry.go`): one live connection per target ID (`local`, `ssh:<alias|user@host:port>`, `s3:<profile>`). Dials are single-flight; a dropped transport redials once. Two panes on the same target share a `connId`. SFTP uses two sessions per host, one to browse and one to transfer.
- **Listings are pulled, not pushed**: `FileService.List` returns the first page plus a cursor, and the frontend calls `ListNext` (`services/listing.go`, `frontend/src/api/listing.ts`). Cursors not pulled for 60 s are reaped. `stores/panes.ts` batches pages every 60 ms, drops stale `requestId`s, and on refresh keeps old entries in `listing.replacing` until the new listing completes.
- **Previews are an HTTP route, not a binding**: `PreviewService` (`services/preview.go`) is an `http.Handler` mounted at `/preview` through `ServiceOptions.Route`, so `<img>` and `<iframe>` load bytes directly. `internal/preview` picks the kind from the name and sniffs text. The frontend (`api/preview.ts`, `components/preview/usePreview.ts`) aborts its fetch when the cursor moves; the canceled request context closes the backend reader. Space toggles it; `useUi.preview` names the source pane and the other `Pane` renders `hidden`.
- **Syntax highlighting** is frontend-only and lazy: `lib/syntax.ts` is the pure language choice (file name, then extension, then a shebang; never content detection; plain above `HIGHLIGHT_LIMIT_BYTES`). `components/preview/highlighter.ts` dynamic-imports highlight.js core and one grammar per language, so each is its own Vite chunk. `useHighlighted` shows plain text first and swaps in markup only if it still belongs to the same text. Only highlight.js output is set as HTML. To add a language, extend `LANGUAGES` and the tables in `syntax.ts` and add its `import()` to `GRAMMARS`.
- **Transfer engine** (`internal/transfer`):
  - A job is one user action, walked lazily into tasks (`walk.go`).
  - `sched.go` takes global and per-connection permits together, so remote-to-remote copies can't deadlock.
  - `gate.go` handles conflict prompts one at a time.
  - `progress.go` is the only emitter, batching every 250 ms.
  - Cancel aborts writers. Retry requeues failures. Resume re-walks canceled jobs, skips finished paths and restarts in-flight files at byte 0.
- **Prompts** (`internal/prompt`): `Ask*` emits an event and blocks until `Respond*`, context cancellation or `Close`. `Pending()` lets a reloaded frontend recover them. A host-key mismatch is a hard error (`HostKeyMismatchError`) and never becomes a prompt.
- **Errors across the boundary**: `services.Classify` gives an `ErrorKind` (`not_exist`, `exist`, `permission`, `unsupported`, `not_connected`, `hostkey_mismatch`, `canceled`, `other`), and `MarshalError` sends `{kind,message}` as the rejection cause. The frontend parses it with `api/errors.ts` `toAppError`. `useUi.showError` ignores `canceled`.
- **Frontend startup** (`stores/bootstrap.ts`): subscribe to events first, then load the `Jobs`, `Pending` and `Connections` snapshots, so a reload loses nothing.
- **Persistence**: `internal/appdirs` is the only XDG path resolver, on macOS too. JSON stores use `encoding/json/v2` and are written via `internal/atomicfile`. S3 secrets live only in the OS keychain (session-memory fallback).

## Key Directories

| Path | Purpose |
|---|---|
| `internal/services/` | Wails-bound services, connection registry, listing cursors, error classification, event names |
| `internal/vfs/{localfs,sftpfs,s3fs}` | Backends; `vfs/vfstest` is the shared conformance suite |
| `internal/transfer/` | Job engine: walk, scheduler, conflict gate, progress, resume |
| `internal/sshcfg`, `internal/sshconn` | `ssh -G` resolution and host picker; SSH auth chain, ProxyJump, known_hosts |
| `internal/testutil/` | `Unavailable` skip helper, `sshdtest` (real/in-process sshd, agent, proxy) |
| `frontend/src/api/` | The only importer of `frontend/bindings`; wraps calls, errors, events |
| `frontend/src/stores/` | zustand stores (`panes`, `connections`, `transfers`, `prompts`, `settings`, `ui`) and action modules (`fileOps.ts`) |
| `frontend/src/lib/` | Pure helpers (selection, sort, history, listing, match/type-ahead, paths, tabStops, syntax) |
| `frontend/src/components/{pane,dialogs,shell,transfers,ui}` | UI; `ui/Modal.tsx` and `ui/controls.module.css` are shared |
| `e2e/` | Playwright flows against a server-mode build, real sshd and RustFS (Docker) |
| `build/` | Wails Taskfiles, platform metadata (Info.plist, nfpm), icons, release scripts |

Generated and gitignored: `frontend/bindings/`, `frontend/dist/`, `bin/`, `THIRD_PARTY_NOTICES.md`.

## Development Commands

`mise` is the command surface. The Taskfiles and `wails3` run underneath.

```sh
mise install             # pinned Go, Node, pnpm, wails3, golangci-lint, gohawk
mise run dev             # wails3 dev, hot reload (Vite on 9245)
mise run bindings        # regenerate frontend/bindings after changing Go service methods, structs or events
mise run check           # everything CI runs: lint, typecheck, Go + vitest, format diff
mise run fmt             # golangci-lint fmt + oxfmt
mise run test:race       # CGO_ENABLED=1, needs a C compiler
mise run lint:gohawk     # concurrency and resource-lifetime analysis
mise run e2e             # Linux + Docker + /usr/sbin/sshd
mise run build           # cgo build -> bin/portunus
```

- **Go needs a frontend build first**: `main.go` embeds `all:frontend/dist`, so Go can't compile until `frontend:build` has run. That's why `lint`/`test`/`check` depend on it, and `frontend:build` in turn depends on `bindings`.
- **All Go lint, test and binding generation use `-tags purego`** (Wails' CGO-free backend, so no GTK/WebKit headers are needed). Only real builds use cgo. `-tags server` (Wails server mode, a browser-only build) is used just by `e2e/run.sh`.
- **`mise run bindings` deletes `frontend/bindings` first** (`-clean=true`). Avoid running it during a frontend build.
- **golangci-lint holds a machine-wide lock**: two worktrees linting at once fails with "parallel golangci-lint is running". Retry.

## Code Conventions & Common Patterns

**Go**
- **Linters and formatting**: `.golangci.yaml` enables `errorlint`, `nilerr`, `noctx`, `exhaustive`, `sloglint`, `gosec` and `modernize`. goimports uses local prefix `github.com/w33ble/portunus`.
- **Errors**: wrap sentinels with `%w`.
- **Context, logging, locking**: pass `context.Context` through. Log with `slog` (`InfoContext`). Guard maps and job state with `sync.Mutex`; use atomics for counters.
- **Stdlib**: use the modern packages (`slices`, `cmp`, `encoding/json/v2`). Every `internal/` package has a package doc comment.

**Frontend**
- **TypeScript**: strict, with `noUncheckedIndexedAccess` and `verbatimModuleSyntax` (use `import type`). Imports are relative; there are no path aliases. Exhaustive `switch` statements end in a `never` check.
- **Formatting and lint**: oxfmt (100 columns, single quotes, trailing commas `es5`); oxlint runs only the correctness and suspicious categories.
- **Stores**: components read with selectors (or `useShallow`). Event handlers and action functions read `useStore.getState()` to avoid stale closures. Reset a store with `setState(getInitialState(), true)`.
- **Dialogs**: open with `useUi.getState().openDialog({ kind })`, and add the kind to the `DialogHost` switch. Every dialog renders through `Modal` (native `<dialog>` plus a manual Tab trap, because WKWebView skips buttons when macOS keyboard navigation is off).
- **Styling**: one CSS Module per component, plus the shared `ui/controls.module.css`. Take every colour from the tokens in `styles.css` (light/dark via `prefers-color-scheme`); code colours are the `--syntax-*` tokens, mapped to `.hljs-*` classes in `PreviewPanel.module.css` with `:global`. Buttons with white text use `--accent-fill`; `--accent` is for rings, icons and text. Global `:focus-visible` draws a box-shadow ring, so a component with its own focus cue overrides it.
- **File list** (`components/pane`): rows are virtualized with TanStack Virtual at a fixed 22 px, with `role="listbox"`/`"option"`. Selection is pruned to visible entries, so filtered-out items drop out. The pane is a CSS size container: columns hide via `@container` below 440px and 320px.
- **Keyboard**: pane-wide shortcuts (⌘L, ⌘R, ⌘⇧N, ⌘F, history) live in `Pane.tsx`; list keys (arrows, type-ahead, Space, F2, F5, Delete, Tab) live in `useFileListKeys.ts`.
- **History gestures**: mouse back/forward (`button` 3/4) are handled in `Pane.tsx`. Swipes follow the macOS "Swipe between pages" setting:
  - Three-finger swipes are discrete OS events. `internal/swipe` (an AppKit local event monitor, cgo, darwin only) reports them; `main.go` forwards them as `nav:swipe`, and `swipeFromOs` applies them to the pane under the pointer.
  - Two-finger swipes are ordinary horizontal `wheel` events, handled in-page only when `SettingsService.SwipeFromScroll` says the OS wants them (always on Linux). `lib/swipe.ts` is the pure detector (thresholds, momentum lockout, ignoring targets that scroll sideways); `useSwipeNavigation.ts` attaches it non-passively and draws the edge arrow straight on the DOM, so the file list doesn't re-render per event.
  - History entries (`lib/history.ts`) keep the selection, which is restored once the listing completes.
- **`data-pane`**: it marks both the pane `<section>` and folder drop targets inside it. Select panes with `section[data-pane]`.

**Process**
- **Commits and PR titles**: Conventional Commits with a scope, e.g. `fix(frontend): …`, `feat(transfers): …`, `test(e2e): …`. PR titles end in `#<issue>` when an issue exists. PRs are squash-merged.
- **PR body**: first line `Fixes #N` (or `Refs #N` for follow-ups), then `## Summary`, `## Verification` (only checks that were actually run), and `## Still to verify on macOS` for things that need the native window.
- **Issues**: bugs use **Cause / Fix / Acceptance**; enhancements use **Proposal or Options / Acceptance**.
- **Screenshots**: commit them to the `pr-assets` branch and link `…/blob/pr-assets/<dir>/<file>.png?raw=true`.
- **Docs to keep in sync**: update the README **Features**, **Install**, **Tests** and **CI** sections when behaviour changes. The README **Install** section mirrors the release-notes template in `.github/workflows/release.yml`. Record deliberate design departures under **Deviations** in `docs/mvp-plan.md`.

## Important Files

| File | Why |
|---|---|
| `main.go`, `window.go` | Wails bootstrap, `frontend/dist` embed, event emit wiring, window geometry |
| `internal/services/app.go` | Service construction and registration |
| `internal/vfs/vfs.go` | Backend contract and error sentinels |
| `internal/services/errors.go`, `frontend/src/api/errors.ts` | The two halves of the error model |
| `frontend/src/api/index.ts` | Every backend call the UI makes |
| `frontend/src/stores/panes.ts` | Pane model: listing, selection, sort, history, filter |
| `frontend/src/styles.css` | Theme tokens |
| `mise.toml` | Tool pins and every dev task |
| `Taskfile.yml`, `build/*/Taskfile.yml` | Version stamping (`PORTUNUS_VERSION`, ldflags into `internal/version`) and packaging |
| `.github/workflows/{ci,e2e,release}.yml` | CI on PRs; e2e on push to main only; tag `v*` makes a draft release |

## Runtime/Tooling Preferences

- **Toolchain** (pinned in `mise.toml`): Go 1.27, Node 24, pnpm 12, wails3 v3.1.0, golangci-lint 2.15. Use pnpm (`pnpm --dir frontend …`), not npm.
- **Packaging and supply chain**: `.npmrc` in `frontend/` and `e2e/` sets `minimum-release-age=10080`, so a just-published package version won't install; that's intended. Add dependencies with `pnpm add` to get the latest allowed version. Every GitHub Action is pinned to a commit SHA.
- **macOS packaging**: the minimum is macOS 12.0, so CSS features need WebKit 15.4+ and can't go newer. The app is ad-hoc signed and not notarized. The bundle is `Portunus.app` (`APP_BUNDLE_NAME` in `Taskfile.yml`), while the executable inside it, the Linux binary and the packages stay lowercase `portunus` (`APP_NAME`).
- **Verifying UI changes**: build the server mode (`CGO_ENABLED=0 go build -tags server .`, served on `WAILS_SERVER_PORT`) and drive it with Playwright WebKit. That's the closest stand-in for the macOS WKWebView available on Linux.

## Testing & QA

| Scope | Command |
|---|---|
| Go, one package / test | `go test -tags purego ./internal/transfer -run '^TestName$' -v` |
| Frontend, one file / test | `pnpm --dir frontend exec vitest run src/lib/sort.test.ts -t 'name'` |
| Fail instead of skip without sshd or Docker (as CI does) | `PORTUNUS_REQUIRE_INTEGRATION=1 go test -tags purego ./internal/...` |
| e2e subset | `E2E_FLOWS="layout dialogs" mise run e2e` |

- **Go**:
  - **Style**: `t.Parallel()`, `t.Context()` and `t.TempDir()` everywhere. Test names are behavioural sentences (`TestCancelMidFileLeavesNoTargetAndClosesBothEnds`). Use table tests with `t.Run`.
  - **External dependencies**: guard them with `testutil.Unavailable(t, …)`. Real SSH comes from `internal/testutil/sshdtest`; S3 tests start RustFS in `s3fs/s3server_test.go`.
  - **Transfer tests**: use `fakefs_test.go` (an in-memory `vfs.FS` with fault, hook and latency injection) and `harness_test.go` (`newHarness`, `waitFinal`, `assertQuiescent`, `scriptAsker`).
  - **New backends**: call `vfstest.Run` from `TestConformance` with the right `Caps`, rather than writing the same cases by hand.
  - **Timing assertions**: keep throughput floors loose, and skip under `-race` with the `race_on_test.go`/`race_off_test.go` pair.
- **Frontend** (vitest + happy-dom):
  - **Mocking**: mock only the wire boundary, with `vi.mock('../api', async () => (await import('./testApi')).createApiMock())`. Helpers: `entry()`, `localConnection()`, `deferred()`, `bindingError()`.
  - **Integration tests**: UI tests go in `App.test.tsx`, which stubs layout, `ResizeObserver` and `<dialog>`, and emits events through a hoisted registry.
  - **Timers**: use fake timers wherever batching or timers are involved.
- **e2e** (`e2e/flows/*.mjs`):
  - **Writing a flow**: call `flow(name, fn, { engine: 'webkit' | 'chromium' })` from `lib.mjs`. To add one, create the file and add its name to the `E2E_FLOWS` default in `e2e/run.sh`.
  - **Order matters**: flows share fixtures and app state. `contrast` must run before `s3`, because it checks the empty S3-profiles state.
  - **Contrast**: `contrast.mjs`/`a11y.mjs` run axe `color-contrast` on every dialog and on highlighted previews in light and dark, plus custom floors (disabled 3:1, placeholder 4.5:1, icons 3:1). New dialogs or colours have to pass it.
- **What good tests look like**: they assert observable behaviour, and a fix comes with a test that fails without it. There's no coverage target. CI runs `check`, `race`, `gohawk` and a cgo build on every PR; e2e runs only after merge to main.
