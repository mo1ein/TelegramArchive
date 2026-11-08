# Troubleshooting

[Documentation index](README.md)

## Connection timeouts or EOF

Check your connection and proxy. These are examples; replace the proxy address with your own:

```sh
ta --proxy socks5://127.0.0.1:1080 archive --chat example_channel --media all
ta --reconnect-timeout 10m archive --chat example_channel --media all
```

With resume enabled, archive retries connection failures up to five times.
Rerunning the same command reuses completed files and checkpoints.
See [resume and recovery](archiving.md#resume-refresh-and-speed).

## Session database timeout

Another command is using the selected namespace. Finish it, or create an independent login:

```sh
ta --ns terminal2 login -T qr
ta --ns terminal2 archive --chat example_channel --output ./data-terminal2
```

The same Telegram account can have independent logins.
Copying a session or importing the same Desktop session shares its authorization key.
See [accounts and settings](configuration.md) for concurrent terminals and legacy storage.

## Incorrect clock or message IDs

Check the system clock when Telegram reports `msg_id too high`.
The optional `--ntp` setting can use a time server:

```sh
ta --ntp pool.ntp.org chat ls
```

## Takeout approval

Telegram may require confirmation or a waiting period for bulk exports.
ta reports the delay. Account exports use takeout by default.
See [account backups](backups.md#separate-account-backup) for the effect of `--takeout=false`.

## Errors after an update

Run `ta version` and check the Telegram API layer.
Keep the transfer and protocol dependencies compatible; see [Desktop coverage](desktop-compatibility.md).
For diagnostic logs, use `--debug`. Redact usernames, chat IDs, proxy credentials and session data before sharing logs.
Logs live under `~/.ta/log/`; cron output is available through `ta cron logs NAME`.
