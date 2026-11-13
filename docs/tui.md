# Interactive workspace

[Documentation index](README.md)

The Bubble Tea interface keeps browsing, task progress, prompts and results in
one screen. Select accounts and chats from lists, use action buttons, and inspect
results without returning to a CLI prompt.

```sh
ta                         # open the workspace
ta tui                     # open it explicitly
ta --ns work tui           # start with a saved account namespace
ta --proxy socks5://localhost:1080 tui
```

From a local checkout, build with `make build` and run `./bin/ta`. Opening the
workspace does not connect to Telegram or lock a saved session. Selecting an
account or chat browser starts its read operation; backups and other changes go
through configuration and review. Ordinary CLI commands remain available, and
`ta` displays help when its input or output is redirected.

## Browse and act

Start with the dashboard's buttons or the first three Quick start workflows:

| Browser | Results and actions |
| --- | --- |
| Chats | Search and select chats, inspect details, then choose Archive, Export or Members. The selected chat ID is filled in for you. |
| Accounts | See saved namespaces and their status. Use account applies the selected namespace to subsequent workflows. Busy or unavailable accounts cannot be activated. |
| Scheduled backups | Select a schedule to run its backup, view logs or remove the schedule. Running and removal require review; removal keeps archives and logs. |

The chat browser shows each page as it arrives, including archived chats. It
reports connection and loading stages instead of waiting for every chat and
forum topic. Connection setup is limited to 30 seconds, page requests to 20
seconds, and a browse to 90 seconds. If loading is slow, **Stop loading** keeps
received chats and enables their action buttons. Errors with no results open
Activity so the reason is visible. The browser uses the same proxy settings and
fallback environment variables as archive operations.

Lists support ↑/↓, mouse selection, wheel scrolling and `/` search. Enter or the
Details button opens the selected item's details. The **Results**, **Activity**
and **Details** tabs are also clickable. Account results remain available when
another saved account fails to load.

After opening an archive, export or schedule action from a list, **Back to list**
restores the original results, search and selection. You do not need to fetch the
list again. Follow-up forms use the current account namespace.

Reopening the same chat browser in this workspace shows its last successful
list immediately, with a saved time. Choose **Refresh** to fetch current chats.
The cache holds up to eight lists in memory, keyed by account, filters and
connection settings. Nothing is saved to a chat-cache file, and restarting ta
requires a fresh load. Login, recovery and migration tasks clear cached lists.

Offline statistics have summary cards and selectable totals and breakdowns.
Other tools retain their output in the Activity view. You can edit settings,
review another run, or return Home from the result screen.

## Buttons and navigation

Click a bottom action button, or use **F6** to move focus between the current
panel and its buttons. When buttons have focus, Tab/Shift+Tab or ←/→ selects one
and Enter activates it. On task and result screens, Tab also focuses the buttons.
Narrow terminals scroll the button row to keep the focused action visible.

The workspace keeps every public command, including extensions, in its command
catalogue. Tab or ←/→ changes dashboard sections; `/` searches every section.
Installed extensions appear when the workspace starts; reopen it after changing
the installed extensions.

Quick start also includes QR login, a single-chat archive, all joined channels,
a complete incremental backup, account data and recurring backups. Presets fill
in options for you to review and change.

## Configure and review

In a form, select a field and press Enter to edit it. Boolean switches toggle
immediately. Space or ←/→ cycles fixed choices, such as an output format or
login method. A dot marks explicitly set options; an asterisk marks required
fields. Ctrl+D restores the default.

Common options appear first. Ctrl+A shows every public option, including proxy,
storage, concurrency, filters and connection settings. `/` searches all options,
including advanced ones. Descriptions appear below the selected field.

List options use commas, for example `photos,videos` or `pdf,zip`. Quote a list
item containing a comma: `"Friends, family",telegram`. For positional arguments,
separate values with spaces and quote paths containing spaces:
`"exports/Book Club/result.json" "exports/Work/result.json"`.

While editing a path, Tab completes a unique matching file or directory name.
`~/` can be expanded this way. Paths are passed as typed; shell variables,
wildcards, command substitution and pipes are not evaluated.

Choose **Review** or press Ctrl+R to validate inputs, dates and supported choices.
The review shows the task and edited settings. **Run** or Enter starts it;
**Edit** or Esc returns to the form. Press `c` to show or hide an optional command
preview. Proxy credentials, passcodes, passwords and tokens are hidden in forms
and previews.

Unchanged settings inherit startup flags and exported `TA_*` settings. Edited
global options apply to later workflows in this workspace session. Nothing is
saved as a new configuration file.

## Tasks, prompts and cancellation

Tasks run inside the workspace. Archive progress shows the current file or
history scan, completion, speed and ETA when available. Activity output updates
without taking over the terminal. When a task finishes, its result stays visible;
there is no extra Enter-to-return prompt.

If a login or another task asks for input, choose **Respond** or press `i`.
Type into the displayed prompt and press Enter. Password prompts mask their
input; account choices accept arrow keys. Press Esc to return to workspace
controls. QR codes also appear in the task panel; enlarge the terminal enough
to show the entire code when logging in by QR.

Choose **Cancel**, press Esc outside input mode, or press Ctrl+C to stop the
active task. The workspace remains open and retains its output. Cancellation
allows a short graceful shutdown before stopping an unresponsive worker. A
cancelled backup can be resumed using its normal checkpoints.

↑/↓ or the mouse wheel scrolls activity; **Follow latest** or `f` resumes following
new output. Activity retains the latest 2,000 lines in memory. Structured list
results are limited to 4 MiB; refine the query if a list exceeds that limit.
Exported archive files are unaffected by these display limits.

Embedded interactive prompts are tested on macOS and Linux. Windows uses piped
task output; prompts that require a terminal may need explicit form values or
the corresponding direct CLI command.

For the native Telegram client on macOS, follow the
[macOS login guide](macos.md) for Full Disk Access and local app passcodes. Quit
native Telegram before using a login imported from it.

Importing a login does not copy Telegram's proxy settings. If direct connections
are blocked, start the workspace with a working SOCKS5 or HTTP proxy. For
example, if your local proxy listens on port 10808:

```sh
./bin/ta --proxy=socks5://127.0.0.1:10808
```

The setting carries into the chat browser and other workflows. You can also set
`TA_PROXY` before launching ta, or use **Connection settings** after a failure to
edit the proxy for the current workspace session.

## Keyboard shortcuts

| Context | Key | Action |
| --- | --- | --- |
| Workspace | ↑/↓ or j/k | Select a workflow |
| Workspace | Tab, Shift+Tab, ←/→ | Change section |
| Workspace or result list | / | Search |
| Workspace | Enter | Browse or open options |
| Workspace | q | Quit |
| Panels | F6 | Focus/unfocus bottom buttons |
| Buttons | Tab/Shift+Tab, ←/→ | Select an action |
| Buttons | Enter | Activate |
| Options | ↑/↓, Tab/Shift+Tab | Select a field |
| Options | Enter | Edit text or toggle a switch |
| Options | Space or ←/→ | Toggle or cycle choices |
| Options | Ctrl+A / Ctrl+D | All options / reset field |
| Options | Ctrl+R | Validate and review |
| Editing | Enter / Esc | Save / cancel an edit |
| Editing | Ctrl+U / Ctrl+K / Ctrl+W | Clear / delete to end / delete a word |
| Editing | Home/End, ←/→ | Move cursor |
| Editing | Tab | Complete a unique path |
| Search | Enter / Esc | Finish / clear search |
| Review | Enter / Esc / c | Run / edit / toggle command preview |
| Results | ↑/↓, PageUp/PageDown | Select a row or scroll |
| Results | Enter / t / l / d | Details / Results / Activity / Details |
| Results | a / s | Archive selected chat / use selected account |
| Results | e / r / Esc | Edit / review another run / return Home |
| Active task | i / Esc | Respond to a prompt / leave input mode |
| Active task | Ctrl+C | Cancel and keep workspace open |
| Active task | f | Follow latest output |
| Idle screens | Ctrl+C | Quit |
| Panels | ? | Open keyboard help when not editing or responding |
