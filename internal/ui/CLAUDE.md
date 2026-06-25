# internal/ui — working guide

Read the repo-root `CLAUDE.md` first for the hard invariants (the privilege
boundary, the single `cmd.Wait()`, the stale-channel guard, "passwords never
touch plaintext storage", English-only). This file is the *how we write code in
the UI tree* guide. The architecture is modelled on Charm's own `crush` TUI
(`internal/ui/AGENTS.md` there) — they wrote bubbletea/lipgloss v2, so they set
the pattern.

## Package map

- `common/` — reusable view **components** (`Dialog`, `TitledBox`,
  `Overlay`/`Center`, the theme). Import these; add new reusable pieces here.
- `utils/` — bubbletea glue: the file-chooser command and the log-stream pump
  (`PickFile`, `WaitForLog`, the `LogMsg`/`LogClosedMsg` messages).
- `model/` — the brain: `UI` (the sole bubbletea model) plus the panels it
  drives (`Sidebar`, `Terminal`, `AuthModal`, `AddModal`).

Import direction is one-way and cycle-free: `common` and `utils` depend on
nothing in the UI tree; `model` imports both.

## Architecture

`UI` (in `model/ui.go`) is the **sole** bubbletea model — the brain. Keep state
and logic there. The panels are not independent Elm models; they are imperative
helpers `UI` owns and calls directly.

- **Centralized routing.** One `switch` in `UI.Update` dispatches everything.
  Order: global messages first (`tea.WindowSizeMsg`, `utils.LogMsg`,
  `utils.LogClosedMsg`) so the live stream and layout flow regardless of mode;
  then the open-modal dispatch (`m.mode`); then normal-mode keys; then list
  navigation. A modal owns all input except the global log stream.
- **Panels are imperative.** `Terminal` is the reference: no `Update(tea.Msg)`,
  just methods `UI` calls (`AppendLog`, `ShowBuffer`, `StartConnection`,
  `MarkClosed`, `State`, `View`). New panels follow this — expose methods (and
  return a `tea.Cmd` when a side effect is needed), render via `View(...)`, and
  let `UI.Update` decide when to call in.
- **Components are external.** Reusable view pieces live in `common/` and are
  imported. Build new reusable pieces there, not in `model/`. Glue commands
  (file chooser, log pump) live in `utils/`.

## Rules (from crush's AGENTS.md, they hold here)

- Never do IO or expensive work in `Update` — always a `tea.Cmd`
  (`utils.PickFile`, `utils.WaitForLog`). Never mutate state inside a command;
  mutate in `Update` in response to the message the command emits.
- Don't nest models / don't add a new mini-Elm sub-model. Add a file, add an
  imperative helper.
- Keep things simple; don't overcomplicate.

## Panels

| Panel        | File                 | Shape                                  |
| ------------ | -------------------- | -------------------------------------- |
| `Terminal`   | `model/terminal.go`  | Imperative ✓ (the model to copy)       |
| `Sidebar`    | `model/sidebar.go`   | Imperative + a legacy `Update` forwarder to the embedded `list` — **migration target** |
| `AuthModal`  | `model/authmodal.go` | Imperative + a legacy `Update` — **migration target** |
| `AddModal`   | `model/addmodal.go`  | Imperative + a legacy `Update` — **migration target** |

**Migration target:** strip the `Update(tea.Msg) (T, tea.Cmd)` Elm signature
from `Sidebar`/`AuthModal`/`AddModal` and have `UI.Update` call targeted
imperative methods instead (the crush way: components have no `Update`).

## Conventions

- **Constants** live in small, documented declarations grouped by purpose — a
  doc comment per constant or group. No single mega `const()`. State enums are a
  typed `uint8` with a doc comment (see `appMode`).
- **Modes** are the overlay stack-of-one: `appMode` + an inline `*View()` method
  (`forgetView`, `disconnectView`) or a sub-model modal (`auth`, `add`). A full
  `dialog.Overlay` stack (crush has one) is overkill at four modes — add it only
  when popups need to stack.
- **Stale-channel guard (invariant #4):** every `utils.LogMsg`/`LogClosedMsg`
  carries its source channel; drop it when `msg.Ch != m.logCh`. Keep this on any
  new log-channel message — it is what stops a connection switch from mixing old
  and new output.
