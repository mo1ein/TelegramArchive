# Troubleshooting

[Documentation index](README.md)

## Telegram login on macOS

See [macOS login setup](macos.md) for the native website client's data directory,
Full Disk Access for Terminal or a coding agent, masked local passcode prompts,
and read-only verification before import. Its
[troubleshooting table](macos.md#troubleshooting) covers permission errors,
key-unlock failures, unsupported authorization archives and pasted shell prompts.

## Telegram stops updating while ta is running

If Telegram hangs or stops showing new messages while `ta` runs, check how you
set up the saved login. Plain `ta login` imports the app's authorization key
from Telegram Desktop or Telegram for macOS. When both run, each opens its own
connection using that key. Even a single `ta` command can conflict with the app,
and Telegram can invalidate the shared key.
See [Telegram's session rules](https://core.telegram.org/api/errors#406-not-acceptable).

To keep the Telegram app open while using `ta`, give `ta` a separate login for
the same account. Finish any `ta` command using the namespace, then replace its
imported login with a QR login:

```sh
ta -n default login -T qr
```

If you use a different namespace or custom session storage, select the same
namespace and storage you normally use. On your phone, open **Telegram →
Settings → Devices → Link Desktop Device** and scan the QR code. Fully quit and
reopen the desktop app so it reconnects, then run `ta` as usual. For future
logins, use `-T qr` or `-T code` to create a separate authorization.

To keep the imported login, fully quit the Telegram app before running an online
`ta` command and reopen it when the command finishes. Local commands such as
`ta render` and `ta stats` can run with the app open.

## Connection timeouts or EOF

If **Browse your chats** stays on a loading screen, use a current build. The
browser reports its connection stage and shows each chat page as it arrives.
Connection setup times out after 30 seconds. On failure, Activity shows the
reason and **Connection settings** opens the proxy field. If some chats have
arrived, **Stop loading** keeps them and enables their action buttons.

A successful `ping telegram.org` tests ICMP reachability to that address. It does
not test Telegram's API connection: ta uses MTProto to a data-centre endpoint
over TCP. Website traffic, ICMP and API traffic can have different routes or
filtering. See Telegram's [transport](https://core.telegram.org/mtproto/transports)
and [data-centre](https://core.telegram.org/api/datacenter) documentation. A direct
timeout that succeeds through a proxy points to a problem with the direct route;
ping alone does not identify where the problem is.

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
