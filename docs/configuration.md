# Accounts and settings

[Back to README](../README.md)

Log in with QR, phone/code or a Telegram Desktop session (the default):

```sh
ta login -T qr
ta login -T code
ta login -T desktop
```

Desktop import supports both the `tdata` format from
[Telegram Desktop](https://desktop.telegram.org/) and the Postbox session format
of native [Telegram for macOS](https://macos.telegram.org/), including the website
download. It discovers the installed client's data without changing the default
login method.

For the native website client on macOS, follow the
[macOS login guide](macos.md) for Full Disk Access and the full import process.
Verify its session before saving a login, using an explicit path to select it
when Telegram Desktop is also installed:

```sh
ta login --check -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"
ta login -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"
```

`--check` decrypts and validates the saved authorization locally, reports the
number of usable accounts, and exits without writing a ta login or connecting
to Telegram. If the native client has a local app passcode, an interactive
terminal asks for it with a masked prompt. This is the local app passcode, not
the two-step verification password. Scripts can supply it with `-p`.
The TUI masks this field and hides it in command previews.

Import opens Telegram's files read-only, authenticates encrypted database pages,
and reads only account metadata and authorization tables. It leaves the source
session intact. Quit the native Telegram app before using the imported login in
other ta commands: the two clients share its authorization key.

If macOS denies access to the Group Containers directory, enable **Full Disk
Access** for the app running the command (Terminal, iTerm or Codex), quit and
reopen that app, then retry. This is an operating-system permission; choosing a
different directory cannot grant it.

To import Desktop data from a custom location, use
`ta login -d "/path/to/Telegram Desktop"` (the `tdata` directory itself is also
accepted). A native data directory can also be supplied with
`ta login -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"`.
The group directory, `accounts-metadata` directory and `atomic-state` file are
accepted too. QR and phone/code login do not require either desktop app.

On macOS, discovery checks both `~/Library/Application Support/Telegram Desktop`
and `~/Library/Containers/org.telegram.desktop/Data/Library/Application Support/Telegram Desktop`.
It also checks the native client's `stable`, `appstore` and `beta` data channels
under its Group Containers directory. Read permission errors are reported
instead of being treated as missing data. Synthetic SQLCipher 3/4 fixtures cover
passcodes, integrity checks, committed WAL snapshots and session storage; client
database/schema changes that are not recognized fail without modifying the source.

List saved logins or choose a namespace:

```sh
ta accounts
ta -n work login -T qr
```

`accounts` lists saved namespaces, account IDs and usernames.
Sessions already used by another process appear as `[busy]`; the command keeps
listing the remaining namespaces. Other account errors appear as `[unavailable]`
and are reported after the list. An active session's identity becomes available
when its current command finishes.

Different namespaces can log in to the same Telegram account. Matching account
IDs mean the same account, even when the namespace names differ. Existing archives
belong to the recorded Telegram account ID; use that account to resume them or
choose a separate output folder for another account.

Native macOS import prints the imported account ID. Other login methods also
retrieve the username. Choose QR, phone/code or Desktop session import with `-T`.
Fresh sessions and logs go under `~/.ta/`. Existing ta session stores are reused.
Commands accept flags and exported `TA_*` settings. Explicit flags take priority.
There is no `.env` loading. Docker and Python are not required.

```sh
TA_LIMIT=4 TA_THREADS=4 ta archive --chat telegram --media all --output ./data
```

Global settings include `TA_PROXY`, `TA_THREADS`, `TA_LIMIT`, `TA_POOL`, `TA_NS` and `TA_RECONNECT_TIMEOUT`.
`ALL_PROXY`, `HTTPS_PROXY` and `HTTP_PROXY` are proxy fallbacks.
Use `--storage` for custom session storage, or `--debug` for diagnostic logs.
Run `ta --help` for all global options.
See [global options](global-options.md) for the full flag and environment reference.

Each concurrent terminal needs its own namespace and independent login.
You can use the same Telegram account in both terminals:

```sh
ta --ns terminal2 login -T qr
ta --ns terminal2 archive --chat "Another group" --output data
```

Keep using that namespace in that terminal. Copying a session or importing the same Desktop key shares its authorization.
Telegram can invalidate a key used by concurrent main connections
([session rules](https://core.telegram.org/api/errors#406-not-acceptable)).
The session lock reports a busy namespace. Finish its command before reusing that login.
Different chats can share an output root; concurrent exports of the same chat need separate roots.
`sync` locks the entire output root.

Old `legacy` storage locks all namespaces. Once commands finish, migrate it with:

```sh
ta migrate --to "type=bolt,path=$HOME/.ta/data"
```

Default storage selection then uses that directory.
Update `--storage` or `TA_STORAGE` too if you set either explicitly.

## Other commands and development

Run `ta` or `ta tui` to configure and run commands in the
[interactive workspace](tui.md). Start it with `ta --ns work tui` to select a
saved account, or edit the namespace field in any workflow. The workspace opens
without connecting to Telegram or locking a saved session.

| Command | Purpose |
| --- | --- |
| `chat ls`, `chat users` | List chats or export channel users |
| `chat export` | Create a download manifest; use `archive` for a chat backup |
| `download`, `upload`, `forward` | Transfer files or forward messages |
| `backup`, `recover`, `migrate` | Back up, restore or migrate sessions/settings; `backup` uses a `.backup.ta` file |
| `extension` | Manage extensions |
| `completion` | Generate shell completions |
| `version` | Print build information and Telegram API layer |

Run `ta <command> --help` for command options.
The [transfer and tool guides](README.md#transfers-and-tools) include worked examples.
To build and check the project:

```sh
make build
make test
make race
make vet
make bench
```

Tests cover exports, filters, resume, recovery, scheduling and offline rendering.
Browser checks cover the HTML reader. Live Telegram comparisons require your own login.
The transfer core is pinned to v0.20.4 with gotd/td v0.145.1 (Telegram layer 225).
Update these dependencies together to keep inherited commands compatible.
