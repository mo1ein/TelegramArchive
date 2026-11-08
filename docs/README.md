# Documentation

[Project README](../README.md)

Install ta with Go 1.25.8 or newer:

```sh
go install github.com/mo1ein/TelegramArchive/cmd/ta@latest
```

The command guides use the installed `ta` command.
See [installation and PATH setup](../README.md#installation), or run
`go install ./cmd/ta` from a checkout to install unpublished local changes.

## Archiving and backups

| Guide | Covers |
| --- | --- |
| [Archiving](archiving.md) | Chat selection, media filters, resume, refresh, progress, JSON/HTML, rendering and statistics |
| [Backups](backups.md) | Account data, Saved Messages, incremental sync, cron jobs and scheduling |
| [Accounts and settings](configuration.md) | Login modes, namespaces, concurrent terminals and development checks |
| [Global options](global-options.md) | Proxy, storage, concurrency, connection settings and environment variables |
| [Desktop coverage](desktop-compatibility.md) | Supported formats, fields and API limits |

## Transfers and tools

| Guide | Covers |
| --- | --- |
| [Download](download.md) | Message links, JSON inputs, filters, albums, resume and HTTP serving |
| [Upload](upload.md) | Files, directories, destinations, captions, routing and photos |
| [Forward](forward.md) | Message sources, direct/clone modes, edits, routing and dry runs |
| [List chats](list-chats.md) | Chat IDs, JSON output and expression filters |
| [Export messages](export-messages.md) | Download manifests, time/ID ranges, topics and content filters |
| [Export members](export-members.md) | Member lists and raw output |
| [Filename templates](templates.md) | Download filename variables and functions |
| [Expressions](expressions.md) | Filter, caption and routing syntax |
| [Session tools](session-tools.md) | Back up, recover or migrate saved logins/settings |
| [Extensions](extensions.md) | Install, run, upgrade and remove additional commands |
| [Shell completion](shell-completion.md) | Bash, Zsh, Fish and PowerShell setup |
| [Troubleshooting](troubleshooting.md) | Connections, session locks, clocks and takeout |

Use `archive` for a content backup. `chat export` creates a download manifest.
`backup` saves session/settings data; `account` saves Telegram account content.
