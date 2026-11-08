# Global options

[Documentation index](README.md) · [Accounts and settings](configuration.md)

Global flags apply to the current command. Repeat them, or use exported environment settings.
Explicit flags take priority. ta does not load `.env` files.

## `-n` / `--ns`

Select a saved login namespace. The default is `default`.
A namespace holds one independent Telegram login; several namespaces may use the same account.

```sh
ta -n work login -T qr
ta -n work chat ls
```

Concurrent terminals need independent logins. See [multiple terminals](configuration.md).

## `--proxy`

Pass `protocol://username:password@host:port`:

```sh
ta --proxy socks5://127.0.0.1:1080 chat ls
ta --proxy http://127.0.0.1:8080 chat ls
ta --proxy https://127.0.0.1:8081 chat ls
```

`TA_PROXY` is the environment equivalent. `ALL_PROXY`, `HTTPS_PROXY` and `HTTP_PROXY` are fallbacks.

## `--storage`

Fresh installations use `type=bolt,path=~/.ta/data`. Existing session stores may be reused.
The flag accepts comma-separated `key=value` pairs:

```sh
ta --storage type=bolt,path=/path/to/login-data chat ls
```

| Driver | Storage | Concurrency |
| --- | --- | --- |
| `bolt` | Separate database files in a directory | Independent namespaces can run at the same time |
| `file` | One JSON file | Intended for inspection and debugging |
| `legacy` | One database file | Locks the whole store; migrate to `bolt` for concurrent namespaces |

Storage contains login credentials. It is separate from the archive's `--output` directory.
See [session backup and recovery](session-tools.md) to move saved logins.

## Transfer and connection flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-l`, `--limit` | `2` | Concurrent transfer tasks |
| `-t`, `--threads` | `4` | Transfer chunks per file |
| `--pool` | `8` | Connections per Telegram data center |
| `--delay` | `0s` | Pause between tasks |
| `--reconnect-timeout` | `5m` | Reconnection backoff limit; `0` removes this time limit |
| `--download-timeout` | `2m` | Inactivity limit for archive history reads and archive/account media transfers; `0` disables. Telegram flood waits extend the deadline. |
| `--ntp` | Empty | Optional time server; empty uses the system clock |
| `--debug` | `false` | Diagnostic logging |
| `--disable-progress-ps` | `false` | Hide CPU/memory statistics in the progress display |

Archive and account commands require transfer limits, threads and pool sizes from 1 to 64.
Higher concurrency does not guarantee higher throughput.

```sh
ta -t 8 -l 4 --pool 8 dl -u https://t.me/example_channel/123
ta --delay 5s dl -f ta-export.json
ta --reconnect-timeout 10m archive --chat example_channel --media all
ta --ntp pool.ntp.org chat ls
ta --disable-progress-ps dl -f ta-export.json
```

## Environment variables

| Variable | Flag |
| --- | --- |
| `TA_NS` | `--ns` |
| `TA_PROXY` | `--proxy` |
| `TA_STORAGE` | `--storage` |
| `TA_DEBUG` | `--debug` |
| `TA_THREADS` | `--threads` |
| `TA_LIMIT` | `--limit` |
| `TA_POOL` | `--pool` |
| `TA_DELAY` | `--delay` |
| `TA_NTP` | `--ntp` |
| `TA_RECONNECT_TIMEOUT` | `--reconnect-timeout` |
| `TA_DOWNLOAD_TIMEOUT` | `--download-timeout` |
| `TA_DISABLE_PROGRESS_PS` | `--disable-progress-ps` |
| `TA_TEMPLATE` | Download `--template` |

`TA_STORAGE` uses a JSON object, rather than the flag's comma-separated syntax:

```sh
export TA_NS=work
export TA_STORAGE='{"type":"bolt","path":"/path/to/login-data"}'
ta chat ls
```

Account settings and logs live under `~/.ta/`; logs use `~/.ta/log/`.
Cron settings and logs use `~/.ta/cron/`. Content archives default to `./data`.
