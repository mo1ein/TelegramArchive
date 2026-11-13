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

From a local checkout, build with `make build` and run `./bin/ta`. Use
`make install` to update the `ta` executable on your PATH as well; building
`bin/ta` alone leaves an older installed copy in place. Reopen the workspace
after updating. Opening the workspace does not connect to Telegram or lock a
saved session. Opening a browser or the **Chats to back up** selector starts
its read operation; backups and other changes go through configuration and
review. Ordinary CLI commands remain available, and `ta` displays help when its
input or output is redirected.

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
seconds, and a browse to 90 seconds. You can select a received chat and choose
**Archive** or **Export** immediately; the browser stops its lookup before
opening that chat's settings. **Stop loading** also keeps received chats.
Errors with no results open Activity so the reason is visible. The browser uses
the same proxy settings and fallback environment variables as archive operations.

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

Quick start also includes QR login, Saved Messages, all joined channels,
a complete incremental backup, account data and recurring backups. Presets fill
in options for you to review and change.

## Configure and review

Choice fields use compact dropdowns attached to their argument's value column.
Click a field to open its list, click each checkbox to select several items,
then click **Done** or **Review** to save. This entire flow works with the mouse.
For the keyboard, Enter opens the list, ↑/↓ moves through items, each Enter
checks or unchecks the highlighted item, and ← saves and closes the list.
The list shows up to four options
with a scrollbar and position indicator; use ↑/↓, PageUp/PageDown, Home/End or
the mouse wheel over the list to scroll. Other arguments stay in the form.
Multi-select items use **[✓]** checkboxes and a distinct color; clicking the
label or checkbox toggles a choice, as do Enter and Space. Clicked items stay in place
under the pointer. Selected names stay visible in the field and in its **Selected:** line.
Ctrl+S shows only your checked items; press it again to show every option.
Tab saves and moves to the next setting; Shift+Tab moves back. ← or **Done**
saves without moving; Esc undoes the current selection. Ctrl+R saves and opens
Review. The other settings and page actions stay visible while choosing.

Type or paste to filter the list; ↓ returns from the search box to the choices.
Filtering and scrolling keep items checked outside the current view. **All**,
**Clear** and **Done** sit beside the field; Ctrl+A and Ctrl+D select all or clear
while choosing. Scrolling outside the list saves it and moves through arguments.
Small terminals reduce the dropdown height to keep page actions visible.
Media offers **All media**, **No media**, or individual categories.
Single-choice fields use a dropdown and apply the highlighted option with Enter.
Export format offers **JSON**, **HTML**, and **Both**; Both exports JSON and HTML.

**Chats to back up** loads the selected account's chat names directly when you
open the field. You can search and check chats as pages arrive, including
archived chats and Saved Messages. Choices save chat IDs while showing names.
**Stop loading** keeps received chats available; **Reload chats** or Ctrl+L
retries the lookup. Completed lists are reused for the same account and
connection settings. Changing accounts clears account-specific chat choices.
Choose one account to browse its chats; for several accounts, use all chats or
add chat names manually.

**Since (date/time)** and **Until (date/time)** appear in the main backup forms.
Leave them empty for no limit. Since is inclusive and Until is exclusive.
Enter or **Calendar** opens a calendar beside the field; shorter terminals show
a compact calendar view. Click a day to select it, or use the arrow keys and
Enter. PageUp/PageDown, the month arrows, or the wheel over the calendar change
months. **Today** highlights today; **Clear** removes the date limit. Esc cancels
without changing the saved value. Tab saves the highlighted day and moves on.
**Type date/time** keeps manual entry available: use `2026-11-01` for a date,
`2026-11-01T18:30:00+03:30` for a time with a time zone, or Unix seconds.
Calendar dates use midnight UTC, matching the CLI's date format.

Lists with custom values, including extensions, MIME filters, chat names and
source links, use an Add-item multi-select. Type or paste one item and click
**Add** to add and check it, then repeat for more items. Enter also adds items.
Use **Done**, Tab or
Ctrl+R to save; pending text is included. Commas inside an added item remain
part of that item. **Clear** removes an optional list's values. Sensitive lists
keep a masked text editor.

Path fields have **Browse…** and **Type path** controls. Press Enter or click
**Browse…** to choose a file or folder. On macOS this opens the system dialog;
backup destinations use a folder chooser, input files use an Open dialog, and
output files use a Save dialog. Fields that accept several files allow multiple
selections. The chooser starts near the current path, including unfinished
text edits. Cancel keeps the existing value and any text you were editing.
Alt+O also opens Browse while typing.

Remote sessions and systems without a graphical chooser use a terminal browser.
Enter or click opens a folder; Space selects an item. Use **Use this folder**
for the current folder or **Use selected** for checked files. Type to filter,
use ← to go up, and Ctrl+H to show hidden items. The Save browser offers F2 to
edit the filename, then **Save here** to select its destination. Choosing a
save path prepares the command for Review; it does not write the output file.

Each workflow starts with its essential settings. Labels and editable values
share a compact argument table, with the selected field's description below it.
Start typing or paste to edit a text field directly in its value column.
Tab saves and moves to the next setting; Shift+Tab moves back. Invalid values
keep the editor open with an explanation. Enter saves; Esc undoes the current
edit. Ctrl+R also saves an active edit before opening the review.

Boolean switches and fixed choices use visible selectors. Space, Enter or ←/→
changes them. Choosing all chats clears an earlier specific-chat selection,
and choosing a specific chat switches back to that scope. An asterisk marks
required settings. Ctrl+D restores a setting's default.

Ctrl+A or **More options** shows every public option, including account,
proxy, storage, concurrency, filters and connection settings. Ctrl+F searches
all options, including advanced ones. Connection details that do not apply to
the selected login method stay out of the essential form.

On the command line, list options use commas, for example `photos,videos` or
`pdf,zip`. Quote a list item containing a comma: `"Friends, family",telegram`.
For positional arguments,
separate values with spaces and quote paths containing spaces:
`"exports/Book Club/result.json" "exports/Work/result.json"`.

While editing a path, Alt+/ completes a unique matching file or directory name.
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

### Back up several accounts or chats

Choose **Back up multiple accounts**, then open **Accounts to back up** and
check the saved accounts you want. Account names are discovered locally without
connecting to Telegram. Ctrl+L while choosing accounts refreshes the saved list.
You can also check rows with Space in **Your Telegram accounts** and choose
**Back up selected**. Busy accounts cannot be checked in the account browser.

Review shows each account and its destination. Multi-account backups run once,
one account at a time, with the same backup settings. Each gets a separate
folder under `<output>/accounts/`, so archives from different accounts do not
mix. A single-account backup keeps its usual output folder. The running screen
identifies the current account, and results retain a completed, failed, cancelled
or not-run status for every selected account. One failure does not prevent the
remaining accounts from running. Cancelling stops the active backup and the
remaining queue. Use separate scheduled backups for recurring account backups.

In **Browse your chats**, check several rows with Space and choose **Archive
selected**. The resulting form also offers the loaded chat names as a checkbox
list. These chats stay tied to the account used to browse them. Returning to the
browser restores its checks, search and selection.

Tasks run inside the workspace. Backups use an animated Bubbles progress bar
with the actual percentage, completed file count, transferred size, speed and
ETA when available. History scans show a spinner and discovered work until a
total is known. They do not show a guessed percentage. Activity output updates
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
| Options | Type, paste, or Enter | Edit text or change a selection |
| Options | Space or ←/→ | Toggle or cycle choices |
| Options | Ctrl+A / Ctrl+D | All options / reset field |
| Options | Ctrl+R | Validate and review |
| Choice dropdown | ↑/↓ or mouse wheel over list | Scroll options within the field |
| Multi-select dropdown | Enter / Space | Toggle the highlighted item; keep choosing |
| Choice dropdown | Tab / Shift+Tab | Save and move to the next / previous setting |
| Multi-select dropdown | ← | Save choices and return to the form |
| Single-choice dropdown | Enter | Apply the highlighted option |
| Choice dropdown | Esc | Undo the current selection |
| Choice dropdown | Ctrl+A / Ctrl+D | Select all / clear multi-selections |
| Choice dropdown | Ctrl+S | Show selected items / all options |
| Custom list | Type or paste, then Enter | Add and check one literal item |
| Account dropdown | Ctrl+L | Refresh saved accounts |
| Chat dropdown | Ctrl+L / Reload chats | Reload the selected account's chats |
| Dates | Enter / Calendar | Open the date picker |
| Calendar | Arrow keys / Enter | Move between days / choose a day |
| Calendar | PageUp / PageDown | Previous / next month |
| Calendar | Ctrl+PageUp / Ctrl+PageDown | Previous / next year |
| Calendar | T / Delete / I | Highlight today / clear the limit / type a timestamp |
| Calendar | Esc | Cancel without changing the date |
| Paths | Enter / Alt+O / Browse… | Open the file or folder chooser |
| Path browser | Enter / Space | Open a folder / select an item |
| Path browser | Ctrl+Enter / Esc | Confirm / cancel the selection |
| Path browser | Ctrl+H / F2 | Show hidden items / edit a save filename |
| Editing | Enter / Esc | Save / cancel an edit |
| Editing | Ctrl+U / Ctrl+K / Ctrl+W | Clear / delete to end / delete a word |
| Editing | Home/End, ←/→ | Move cursor |
| Editing | Tab / Shift+Tab | Save and move to the next / previous setting |
| Editing | Alt+/ | Complete a unique path |
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
