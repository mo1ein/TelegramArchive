# ta (Telegram Archive)

<img align="right" src="docs/assets/ta-gopher.svg" width="235" alt="A rosy-cheeked backup gopher hugging a ta archive box in a little paper-plane hat">

ta backs up Telegram chats, groups and channels as JSON or offline HTML.
It downloads media, keeps checkpoints and adds new messages on later runs.
Back up and download files from restricted channels your account can access.
Inspired by [tdl](https://github.com/iyear/tdl), ta uses its transfer and session components.

## Supported features

- Archive chats, groups and public/private channels.
- Restricted/protected-content channel backups and downloads.
- Desktop-style JSON and offline HTML with local media.
- File-type, date and size filters.
- Parallel downloads with progress, resume and recovery.
- Account data and Saved Messages backups.
- Incremental sync, refresh and cron schedules.
- Offline rendering, statistics, uploads and forwarding.

## Quick start

Build with Go 1.25.8 or newer:

```sh
make build
./bin/ta login -T qr                  # use -T code for phone/code login
./bin/ta archive --all --types channels --media all --format both
```

This exports every joined channel, including private channels and archived dialogs.
Files go to `./data`. Open `data/export_results.html` to browse them.
Resume is on by default. Rerun the same command to continue or fetch new messages.

## Common commands

```sh
# A private group by name; usernames, URLs and IDs also work.
./bin/ta archive --chat "Book Club" --media all

# PDF/ZIP files from joined channels, with date and size limits.
./bin/ta archive --all --types channels --media documents --extensions pdf,zip \
  --since 2026-01-01 --until 2026-02-01 --min-file-size 1024 \
  --max-file-size 104857600 --format html

# Account data, including Saved Messages, in a separate account directory.
./bin/ta account --include all --media all --format both

# Sync all chats and account data once.
./bin/ta sync --everything --format both

# Back up now, then let system cron sync this channel every six hours.
./bin/ta cron add my-channel --chat "channel_username" --every 6h \
  --media all --format both --output ./data --run-now
```

Use `--output` to choose a destination. Keep the same destination and filters when resuming.
Use a separate output root for each Telegram account.
Cron requires a working cron service and the `crontab` command.

## HTML preview

Fictional meme demo in Telegram Desktop's export style. Profile photos added for illustration.
Click a preview to enlarge it.

<table>
  <tr><th>Chat export</th><th>Account export</th></tr>
  <tr>
    <td width="50%" valign="top"><a href="docs/assets/html-chat.png"><img src="docs/assets/html-chat.png" width="390" alt="Fictional meme chat export with photos, replies and a poll"></a></td>
    <td width="50%" valign="top"><a href="docs/assets/html-account.png"><img src="docs/assets/html-account.png" width="390" alt="Fictional account export with Saved Messages and account sections"></a></td>
  </tr>
</table>

## Documentation

| Guide | Covers |
| --- | --- |
| [Archiving](docs/archiving.md) | Chat selection, media/date/size filters, custom names, resume, refresh, progress, JSON/HTML, offline rendering and statistics |
| [Backups](docs/backups.md) | Account sections, Saved Messages, one-time sync, recurring sync, cron jobs and logs |
| [Accounts and settings](docs/configuration.md) | Login, multiple terminals, proxies, storage, environment settings, transfer commands and build checks |
| [Transfers and tools](docs/README.md#transfers-and-tools) | Downloads, uploads, forwarding, chat tools, templates, session tools, extensions and shell completion |
| [Desktop export coverage](docs/desktop-compatibility.md) | Supported formats, fields and API limits |
| [Contributing](CONTRIBUTING.md) | Development workflow and privacy checks |

Run `./bin/ta --help` or `./bin/ta <command> --help` for command options.
