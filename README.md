# ta (Telegram Archive)

ta is a Go CLI that exports Telegram chats
as **JSON or offline HTML with local media**, plus statistics and scheduled backups.
Chat/group/channel exports and account backups are separate commands.
Inspired by [tdl](https://github.com/iyear/tdl), ta uses its transfer and session components.

## Quick start

Building from source requires Go 1.25.8 or newer. Run the compiled binary directly.

```sh
make build
./bin/ta login -T qr                  # QR login; -T code uses a phone/code
./bin/ta archive --all --types channels --media all --format both
```

Joined-channel exports include public/private channels in both the main and archived
dialog folders. New exports go to `data/ChatExport_channel_<id>_<username>/`
(the username suffix is omitted when the chat has no username):

```text
result.json          # JSON chat metadata and messages, oldest first
messages.html        # offline reader; messages2.html, etc. for further pages
statistics.json      # counts, senders, days/hours, media and text statistics
archive.db           # local checkpoint and ordered messages; keep for resume
photos/              # relative paths referenced by result.json
files/
```

Chat directories include stable peer types and IDs. Existing directories, including
older ID-only names, are reused when a title, username, or custom label changes,
so sync continues the same archive. User, basic group and channel IDs have separate directories.
Checkpoints also verify the logged-in account before resuming.
Use a separate `--output` directory for each account; even a full rebuild rejects
another account's checkpoint so its media cache cannot be reused accidentally.

`ta login`, `ta chat ls`, `ta download`, `ta upload`, `ta forward`, and other transfer
commands are available. `ta chat export` generates a download manifest;
**use `archive` for a chat backup**. Run `./bin/ta --help` for the full CLI.

## Select chats and media

```sh
# Private group names, usernames, Telegram URLs, or IDs; repeat --chat or comma-separate.
./bin/ta archive --chat me,telegram --media all
./bin/ta archive --chat "Book Club" --media all
./bin/ta archive --chat=-1001234567890 --media none
./bin/ta archive --chat telegram --export-name "My reference archive"

# All matching dialogs, including private groups/channels you can access.
./bin/ta archive --all --types personal,bots,groups,channels
./bin/ta archive --all --types contacts,private_channels

# Export only PDF/ZIP documents from every joined channel, with size/date limits.
./bin/ta archive --all --types channels --media documents --extensions pdf,zip \
  --since 2026-01-01 --until 2026-02-01 --min-file-size 1024 \
  --max-file-size 104857600 --format html

# MIME patterns are also supported. --only-media removes nonmatching messages.
./bin/ta archive --all --types channels --media all --mime 'video/*' \
  --only-media --format both

# UTC dates: since is inclusive; until is exclusive.
./bin/ta archive --chat me --since 2026-01-01 --until 2026-02-01 \
  --media photos,videos --max-file-size 104857600 --part-size 52428800

# ta proxy settings apply; ALL_PROXY, HTTPS_PROXY and HTTP_PROXY are fallbacks.
./bin/ta --proxy socks5://127.0.0.1:1080 -n personal archive --chat me
```

Chat types: `personal`, `bots`, `groups`, `channels`, `contacts`,
`public_channels`, `private_channels`, `public_groups`, `private_groups`.
`contacts` selects chats with Telegram contacts. Account contacts and Saved Messages
are handled by the separate `account` command. `--all` does not include Saved Messages;
explicit `--chat me` remains available for compatibility.
Private-group names are matched case-insensitively against joined dialogs in the
main and archived folders. If names are ambiguous, select by numeric ID. The
existing chat list command shows dialog names and IDs. Each chat export stores
its Telegram username when available. New export folders include that username
by default; `--export-name` sets the display name and replaces the username suffix
with a custom folder label. Unsafe filename characters are replaced with underscores.
The Telegram username stays in the JSON metadata.

The `ta accounts` command lists saved namespaces with account IDs and usernames.
Choose an account with `ta -n work login -T qr` or `ta -n work archive ...`.
After a successful login, the CLI prints the account ID and username.

To run commands in multiple terminals, give each concurrent terminal its own
namespace. You can log in to the **same Telegram account** in all of them. For
example, keep your current export running in the first terminal, then run:

```bash
# Second terminal: choose an unused namespace and log in once.
./bin/ta --ns terminal2 login -T qr
./bin/ta --ns terminal2 archive --chat "Another group" --output data
```

Keep using `--ns terminal2` in that terminal. `-T code` also creates an independent
login. Each active command needs an independent login: copying a session database
or importing the same Desktop session shares the authorization key. Telegram can
invalidate that key when it is used by concurrent main connections
([Telegram's session rules](https://core.telegram.org/api/errors#406-not-acceptable)).
The session lock protects the login and reports which namespace is busy. Complete
or stop its running command before reusing that namespace. Exports of different
chats may share an output root; simultaneous exports of the same chat need
separate `--output` directories. `sync` locks its entire output root.

Old `legacy` session storage locks every namespace. When all commands have
finished, migrate it once with `./bin/ta migrate --to "type=bolt,path=$HOME/.ta/data"`.
Default storage selection will then use that directory. If you set `--storage` or
`TA_STORAGE`, update it to `type=bolt,path=...` for the migrated directory.

`--include-migrated` (default true) also exports the original basic-group history
of migrated supergroups, under a separate linked peer directory so overlapping
message IDs cannot overwrite each other. `--include-migrated=false` skips these
metadata lookups. `--include-left --all --takeout` optionally includes left channels;
Telegram may allow only your own messages, which is marked in the export.

A failed chat does not stop other chats. `run-report.json` records all outcomes and
discovery warnings. HTML runs write Desktop's `export_results.html` and
`lists/chats.html`; `index.html` remains an alias for the export index.
Any failure returns a nonzero exit status after the other chats finish.

Media types: `photos`, `videos`, `audios`, `stickers`, `animations`,
`documents`, `voice_messages`, `video_messages`, `contacts`; also `all` or `none`.
Shared contacts include contact information and optional vCards. Location and poll
metadata is exported without downloading files. Skipped files retain metadata and
Desktop-style explanatory placeholders. Media filenames are sanitized and include
message/media IDs to avoid collisions and stale files after attachment edits.
Selecting `stickers` also downloads custom emoji referenced in text, with shared
transfers deduplicated within a page.

Extensions and MIME filters combine with media categories and inclusive file-size
limits. They filter downloads while retaining message metadata by default.
Supplying `--extensions` or `--mime` selects all media categories unless you also
pass `--media` explicitly; the file filters then narrow the downloads.
`--only-media` includes only messages containing a matching downloadable attachment.
Thumbnails follow the parent attachment's filters.

`--part-size` is a byte budget. Each `result_partN.json` includes the chat metadata;
a single message larger than the budget occupies its own part. Exported dates
use local time, as Desktop does; Unix timestamps preserve the instant. Activity
statistics use UTC. `result.json` follows Desktop's chat schema, including its
formatted text arrays, service messages, file placeholders and numeric IDs.
Extra metadata (chat usernames, restricted-history scope, raw service/media data)
is kept in `archive-metadata.json` and checkpoints. UTF-16 offsets stay in the
checkpoint for nested HTML formatting, rather than appearing in Desktop's JSON
entities. See [the compatibility matrix](docs/desktop-compatibility.md). Secret
chats are not available through this cloud-history client.

## Resume and performance

Rerun the same command to continue after interruption or append newer messages.
Each completed page is committed only after its selected downloads succeed.
Existing complete media is checked by byte size and reused; incomplete files are
never published as completed media. A failed file stops the page with a visible
error, and retrying fetches fresh message/file references. Partial file chunks are
restarted; completed files and history pages are reused.

With the default `--resume=true`, a dropped Telegram connection (EOF, connection
reset, or network timeout) triggers up to five fresh session attempts with capped
exponential backoff. Each attempt fetches fresh file references and reuses completed
pages and media. Ctrl+C, authorization errors and local file errors stop the run.
The archive announces automatic recovery on startup. An interrupted session is
retried and cannot be reported as a successful export.
`--resume=false` disables these session retries so a reconnect cannot restart a
requested full rebuild and discard its progress.

Incremental runs preserve previously archived messages, including messages later
deleted on Telegram. `--refresh` scans existing history again to update older edits,
reactions and attachments while retaining archived deletions. It resumes an
interrupted refresh from its last completed page:

```sh
./bin/ta archive --all --types channels --media all --format both --refresh
```

For a fresh snapshot containing only current Telegram history, rebuild the checkpoint:

```sh
./bin/ta archive --chat me --resume=false --media photos,documents
```

Use `--resume=false` when changing media types, extensions/MIME filters, date limits, file size limits, or
takeout mode. It rebuilds the checkpoint and JSON from current Telegram history,
while reusing matching completed media. Output format and HTML page size can change
without rebuilding history. Previous published files stay available
until a replacement is written. Keep a separate output directory if you want to
retain an older snapshot.

History is read 100 messages at a time. Downloads run concurrently across messages
(`-l/--limit`, default 2), with parallel chunks per file (`-t/--threads`, default 4)
and DC connection pooling (`--pool`, default 8). A live progress bar
shows transferred bytes, processed messages, and active media downloads. Example:

```sh
./bin/ta -l 4 -t 4 --pool 8 archive --chat me --media all
```

Message history lives on disk in bbolt, an existing dependency; output is streamed
in chronological order. RAM holds a page of messages, download buffers and
statistics maps (distinct senders/days), rather than the full history.
Account profile-photo/story lists and contact lists currently retain their metadata
in memory; their media downloads use the same bounded transfer pipeline.
ta handles Telegram flood waits and reconnection. Throughput still depends
on Telegram limits, account and network; local benchmarks do not establish live
download speed. 

For Telegram's official bulk export session, add `--takeout`. This wraps history
and file requests and paginates Telegram's message ranges. Telegram can require
approval or a waiting period; the CLI reports that delay explicitly. Normal history
is the default so a first export can start without takeout approval.

## HTML and offline rendering

Choose `--format json`, `--format html` or `--format both`. HTML uses Telegram
Desktop v7.4.1's original export CSS, JavaScript and icons, bundled locally.
The layout includes Desktop's header, sender initials, date separators, joined
messages, attachment cards, sections and pagination. Reply navigation and spoiler
reveals work offline. It renders Unicode/RTL text, formatting, forwards, polls,
contacts, service events and media links. Chat readers share the root assets.
`--html-page-size` defaults to 1,000 messages to keep individual pages responsive.
Open the export's `export_results.html` or a chat's `messages.html` directly in a browser.
Animated TGS custom emoji/stickers remain downloadable assets; their animation is
not played by the static reader.

To change formats without reconnecting to Telegram:

```sh
./bin/ta render data/ChatExport_channel_123 --format html --html-page-size 500
./bin/ta render data/ChatExport_channel_123 --format both
./bin/ta render data/AccountExport_123 --format both
```

This also upgrades older checkpoints to the Desktop presentation and JSON schema
without downloading their media again. Resumed archives automatically invalidate
old presentation caches once; unchanged runs continue to reuse current outputs.

## Separate account backup

Account data goes to `data/AccountExport_<account ID>/`. It is independent of
`archive`, and **Saved Messages belong here** alongside the other account sections.

```sh
# All account sections, including Saved Messages and their media.
./bin/ta account --include all --media all --format both

# Select only the sections you want.
./bin/ta account --include saved_messages,contacts,profile_photos,stories \
  --media all --format html

# Account filters apply to profile photos, stories and Saved Messages media.
./bin/ta account --include saved_messages --media documents --extensions pdf \
  --since 2026-01-01 --max-file-size 104857600 --format both
```

Sections: `personal_info`, `contacts`, `frequent_contacts`, `profile_photos`,
`stories`, `saved_messages`, `sessions`, `web_sessions`, `other_data`.
Desktop's `result.json` / `export_results.html` summarize them; `account.json` and
`account.html` remain aliases. Profile pictures and stories use Desktop's arrays;
Saved Messages appear under `chats.list` in this separate account export, with
correct paths to their media. Media and Saved Messages also have separate
subdirectories. `account-report.json` records section failures. A failed
section leaves previous successful section files intact and does not stop others.
Authorization hashes/session credentials are not included in session metadata.

Account export defaults to `--takeout` for the full saved-phone contact list and
Telegram's `other_data.json`. Telegram may require approval or a wait. With
`--takeout=false`, contacts contain registered Telegram contacts only, and
`other_data` reports that takeout is required. No account backup is started by a
normal chat export.

## Automatic incremental backups

To install a backup in your **system cron** using only ta commands:

```sh
# Install the job, archive immediately, then let cron launch future syncs.
./bin/ta cron add my-channel --chat "channel_username" --every 6h \
  --media all --format both --output ./data --run-now

# All joined chats and separate account data instead.
./bin/ta cron add everything --everything --every 24h --format both --run-now

./bin/ta cron list
./bin/ta cron logs my-channel
./bin/ta cron run my-channel
./bin/ta cron remove my-channel
```

This installs or updates that named job in your user crontab, preserving other
cron entries. `--every 6h` means `0 */6 * * *`: 00:00, 06:00, 12:00 and 18:00 in
cron's timezone. `--run-now` performs the first backup immediately; omit it to
install and exit immediately. Ta does not need to stay open between cron runs.
An overlapping trigger of the same job is skipped while its previous run finishes.
Each trigger runs one incremental sync and exits; failures retry at the next tick.

Current login namespace, session-storage path, proxy settings, working directory
and absolute output/binary paths are captured automatically. Job settings and logs
are private under `~/.ta/cron/`; proxy credentials are not placed in crontab.
Keep the binary and its saved working directory in place. Re-run `cron add` with
the same name to update settings. Removing a job keeps its logs and backup data.
To back up several channels on one account, use one job with `--chat first,second`
or `--all --types channels`. Distinct simultaneous jobs need independent saved
logins and output roots, as normal concurrent ta commands do.

This requires a working system cron service and the `crontab` command (Linux/macOS).
Supported intervals are whole minutes dividing an hour, whole hours dividing a
day, and 24h. For arbitrary intervals such as 90m, use the foreground scheduler:

Use one recurring sync job for the data you select:

```sh
# All joined chats/groups/channels, media, and separate account data every 6 hours.
./bin/ta sync --everything --every 6h --format both

# Only joined channels, with file filters.
./bin/ta sync --all --types channels --extensions pdf,mp4 \
  --max-file-size 104857600 --every 1h --format both

# Chats plus selected account sections; Saved Messages stay with account data.
./bin/ta sync --all --media all --account-include contacts,saved_messages \
  --every 6h --format both

# Account sections only.
./bin/ta sync --chats=false --account-include contacts,profile_photos \
  --media all --every 24h --format both

# Omit --every to perform just one cycle.
./bin/ta sync --everything --format both
```

The first run downloads the selected history. Later runs append new messages to
existing checkpoints, reuse completed media, and discover newly joined chats.
Unchanged chats reuse their existing JSON/HTML/statistics after checking the
checkpoint revision, presentation settings and output files. Missing or modified
output files are rebuilt. Chat histories and Saved Messages retain locally archived
messages deleted from Telegram. Contacts, sessions and account asset indexes are
refreshed as current metadata; their previous media files remain on disk.

Account backup is optional (`--account` or `--account-include`), except when
`--everything` explicitly selects it. It always uses its separate account directory.
Explicit media/file/date/size filters still apply with `--everything`. A bare `sync`
selects all chats matching `--types`; use `--chat` for explicit peers.

Each cycle runs chats and account sections sequentially. A failed chat phase does
not prevent the account phase. `backup-report.json` records the latest cycle and
component outcomes; existing per-chat/account reports provide the details. One
output-root lock prevents overlapping `sync` jobs, and the Telegram session lock
is released between phases and while waiting. Failed cycles retry at the next
interval. Ctrl+C/SIGTERM cancels the current cycle; restarting retries immediately
from checkpoints. Missed intervals are skipped rather than queued.

Incremental mode fetches new messages. Add `--refresh` to scan older history for
edits/reactions/attachments too; this costs more API requests but reuses complete
media. Recurring sync always resumes by default and rejects `--resume=false`.
To change checkpoint filter settings,
use a new output directory or perform a one-time rebuild without `--every`.

`--runs 3` limits recurring cycles; `--job-timeout 2h` bounds an entire cycle;
`--stop-on-error` stops after the first failed cycle. Global settings such as
`-n personal`, `--proxy` and `--storage` are forwarded to both phases. Account
sections retain their normal takeout default; explicit `--takeout=false` applies
to both phases and leaves takeout-only account sections unavailable.

For unattended use on Linux/macOS, run the native process in the background:

```sh
nohup ./bin/ta sync --everything --every 6h --format both --output ./data \
  > backup.log 2>&1 &
```

Use your OS service manager if automatic restart after reboot is required.
Cron can invoke a one-time `ta sync`, but an additional timer is unnecessary when
using `--every`. Existing `ta schedule --target archive|account|sync --every 6h --
[target flags]` remains supported. `ta backup` creates a session/settings backup
with a `.backup.ta` filename; Telegram content backups use `archive`, `account` or `sync`.

## Statistics and extending them

```sh
./bin/ta stats data/ChatExport_user_123/result.json
./bin/ta stats data/ChatExport_user_123/result_part*.json > combined-statistics.json
```

This command works offline with per-chat Desktop JSON, this project's JSON and
legacy Python exports. It accepts multiple parts/files and streams messages.
Pass each part once: statistics aggregate the supplied records without deduplication.

Statistics include message/service counts, words, Unicode characters, replies,
forwards, edits, first/last dates, per-sender/day/hour activity, media counts and
referenced media bytes (including files excluded from download).

To add a statistic, add a field and update `Stats.Observe` in
[`internal/archive/stats.go`](internal/archive/stats.go). Both live exports and
offline analysis call it. No plugin registry or new storage layer is needed.

## Command settings

Choose chats, media, filters and destinations with command flags. Global settings
also accept exported `TA_PROXY`, `TA_THREADS`, `TA_LIMIT`, `TA_POOL`, `TA_NS` and
`TA_RECONNECT_TIMEOUT` environment variables. Explicit flags take priority.

```sh
TA_LIMIT=4 TA_THREADS=4 ./bin/ta archive --chat telegram --media all --output ./data
```

Resume is enabled by default. Use `--output /path/to/backups` to choose where
archives and media are stored; the default is `./data`. Keep the same flags and
output directory when resuming. Old Python configuration variables are ignored.

Fresh installations store sessions and logs under `~/.ta/`. Existing session
stores are reused for compatibility. Log in using `ta login` or its Desktop session
import. Download sizes are checked and checkpoints are saved after each completed page.

## Build and verify

```sh
make build
make test
make race
make vet
make bench
```

Tests cover JSON parts, Unicode entities, chat/media selection, download concurrency,
resume/incremental behavior, cancellation, size checks, failure recovery, takeout
wrappers/ranges, all/archived dialog discovery, migration history, nested formatting,
HTML escaping/pagination, account metadata separation, scheduling, refresh and
offline statistics. The HTML reader is also checked in an isolated browser.
Live Telegram export comparisons require your login.
The transfer core is pinned to v0.20.4 with the compatible gotd/td v0.140.0 API
(Telegram layer 222). Update these dependencies together to keep inherited
commands compatible. `ta version` shows the binary's Telegram API layer.
