# internal/ui — working guide

Read the repo-root `CLAUDE.md` first for the hard invariants (the privilege
boundary, the single `cmd.Wait()`, the stale-channel guard, "passwords never
touch plaintext storage", English-only). This file is the *how we write code in
the UI tree* guide. The architecture is modelled on Charm's own `crush` TUI
(`internal/ui/AGENTS.md` there) — they wrote bubbletea/lipgloss v2, so they set
the pattern.

## Package map

- `common/` — **stateless** view primitives (`Popup`, `TitledBox`,
  `Overlay`/`Center`, the theme). Import these; add new primitives here.
- `dialog/` — **stateful** overlay components (`Confirm`, `Menu`, `FilePicker`,
  `Credentials`, `Rename`). Built on `common.Popup`; the UI builds, drives, and
  centers them.
- `utils/` — bubbletea glue: the log-stream pump (`WaitForLog`, the
  `LogMsg`/`LogClosedMsg` messages). The file chooser now lives in
  `dialog.FilePicker`.
- `model/` — the brain: `UI` (the sole bubbletea model) plus the panels it
  drives (`Sidebar`, `Log`). The overlays — credential prompt, confirm,
  file picker — are `dialog` components the UI drives (the import flow lives in
  `add.go`).

Import direction is one-way and cycle-free: `common` depends on nothing in the
UI tree; `dialog` and `utils` build on `common`; `model` imports all three.

## Architecture

`UI` (in `model/ui.go`) is the **sole** bubbletea model — the brain. Keep state
and logic there. The panels are not independent Elm models; they are imperative
helpers `UI` owns and calls directly.

- **Centralized routing.** One `switch` in `UI.Update` dispatches everything.
  Order: global messages first (`tea.WindowSizeMsg`, `utils.LogMsg`,
  `utils.LogClosedMsg`) so the live stream and layout flow regardless of mode;
  then the open-modal dispatch (`m.mode`); then normal-mode keys; then list
  navigation. A modal owns all input except the global log stream.
- **Panels are imperative.** `Log` is the reference: no `Update(tea.Msg)`,
  just methods `UI` calls (`AppendLog`, `ShowBuffer`, `StartConnection`,
  `MarkClosed`, `State`, `View`). New panels follow this — expose methods (and
  return a `tea.Cmd` when a side effect is needed), render via `View(...)`, and
  let `UI.Update` decide when to call in.
- **Components are external.** Stateless view primitives live in `common/`;
  stateful overlay components (confirm popups, pickers) live in `dialog/`. Build
  new reusable pieces there, not in `model/`. Glue commands live in `utils/`.

## Rules (from crush's AGENTS.md, they hold here)

- Never do IO or expensive work in `Update` — always a `tea.Cmd`
  (`dialog.FilePicker.Open`, `utils.WaitForLog`). Never mutate state inside a
  command; mutate in `Update` in response to the message the command emits.
- Don't nest models / don't add a new mini-Elm sub-model. Add a file, add an
  imperative helper.
- Keep things simple; don't overcomplicate.

## Panels

| Panel        | File                 | Shape                                          |
| ------------ | -------------------- | ---------------------------------------------- |
| `Log`        | `model/log.go`       | Imperative ✓ — targeted methods; `Scroll(msg)` forwards nav keys to the viewport when the pane is focused |
| `Sidebar`    | `model/sidebar.go`   | Imperative ✓ — hand-rolled (no bubbles list/delegate); `UI.Update` drives the cursor via `Move`, reads `shared.connected` by pointer |

(The credential prompt and import picker are not panels: they are
`dialog.Credentials` and `dialog.FilePicker`, overlay components the `UI` drives
— see the `dialog` package map above.) No panel or dialog has an Elm
`Update(tea.Msg) (T, tea.Cmd)`. A component that must forward a raw message to a
wrapped bubble (`dialog.Credentials` → its textinputs) exposes a
pointer-receiver `Handle(msg tea.Msg) tea.Cmd` that mutates in place; `UI.Update`
calls it and does not reassign. Never add a sub-model that returns a new copy of
itself.

`UI` is a **pointer** model (`*UI`, pointer receivers). That lets panels read
global state by pointer rather than being pushed to: `shared{connected}` is held
on `UI` and handed to the sidebar as `&m.sh`; the sidebar reads it live at render
time, so flipping the field repaints the ● — no setter, no rebuild. The sidebar
is hand-rolled because the app has exactly one list; if a second list view ever
lands, extract a small list component (model it on crush's `Item.Render(width)`,
not its 23 KB machinery).

## Conventions

- **Constants** live in small, documented declarations grouped by purpose — a
  doc comment per constant or group. No single mega `const()`. State enums are a
  typed `uint8` with a doc comment (see `appMode`).
- **Modes** are the overlay stack-of-one: `appMode` selects which overlay owns
  input. Yes/no popups share one `modeConfirm` backed by a reusable
  `dialog.Confirm` (built with an `onYes` closure); stateful modals (`auth`,
  `add`, `menu`, `rename`) keep their own mode. Overlays swap rather than stack —
  `menu`'s `f`/`r` hand off to `modeConfirm`/`modeRename` by flipping `appMode`,
  they do not layer. A full `dialog.Overlay` stack (crush has one) is overkill
  while only one popup shows at a time — add it when popups must stack.
- **Pane focus** is orthogonal to modes: a `pane` field (`focusSidebar`/
  `focusLog`) picks which panel gets navigation keys in `modeNormal` (an
  open overlay still wins). `tab` focuses the log pane so its viewport
  scrolls (`Log.Scroll`); `tab`/`esc` return to the sidebar. Use a
  dedicated key, not `enter` — `enter` is connect/reconnect and overloading it
  was ambiguous (we can't reliably tell a live tunnel from a closed `logCh`,
  e.g. a `daemon` config). Keep focus a plain field, not an `appMode` — it is
  not an overlay and the global log stream keeps flowing under it.
- **Stale-channel guard (invariant #4):** every `utils.LogMsg`/`LogClosedMsg`
  carries its source channel; drop it when `msg.Ch != m.logCh`. Keep this on any
  new log-channel message — it is what stops a connection switch from mixing old
  and new output.
