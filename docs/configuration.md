# Accounts and settings

[Back to README](../README.md)

Log in with QR, phone/code or a Telegram Desktop session:

```sh
ta login -T qr
ta login -T code
ta login -T desktop
```

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

Login also prints your account ID and username. Choose QR, phone/code or Desktop session import with `-T`.
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
