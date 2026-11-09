# Archiving

[Back to README](../README.md)

Use `archive` for chat backups, `render` to rebuild local output, and `stats` for offline analysis.
For account data and scheduled syncs, see [Backups](backups.md).

## Export all joined channels

```sh
ta archive --all --types channels --media all --format both
```

This exports every joined channel, including private channels and archived dialogs.
The default output directory is `./data`. An export looks like this:

```text
data/
  export_results.html                # open this to browse all chat exports
  lists/chats.html
  ChatExport_channel_123_example_channel/
    result.json                      # messages, oldest first
    messages.html                    # more pages: messages2.html, ...
    archive-metadata.json            # usernames and extra Telegram metadata
    statistics.json                  # message, sender, media and activity counts
    archive.db                       # checkpoint; keep this for resume
    photos/
    files/
```

New folder names include the chat ID and username, when available.
Later runs reuse the folder even if its title, username or custom label changes.
User chats, basic groups and channels have separate ID spaces.
Checkpoints verify the logged-in account. Use a separate output root for each account.

## Archive chats and media

`archive` defaults to Saved Messages (`me`), JSON, photos and `--resume=true`.
Choose chats by displayed name, username, Telegram URL or ID.
Repeat `--chat` or separate values with commas.

```sh
# Usernames and Saved Messages.
ta archive --chat me,telegram --media all

# People and private groups work by their displayed chat name.
ta archive --chat "mammad" --media all
ta archive --chat "Book Club" --media all
# An @ prefix explicitly selects a username instead of a displayed name.
ta archive --chat "@mammad" --media all
# Use ta chat ls to find chat IDs.
ta archive --chat=-1001234567890 --media none

# Custom display name and folder label for one chat.
ta archive --chat telegram --export-name "My reference archive"

# All matching chats, including private groups and channels.
ta archive --all --types personal,bots,groups,channels
ta archive --all --types contacts,private_channels

# PDF/ZIP files from every joined channel, with date and size limits.
ta archive --all --types channels --media documents --extensions pdf,zip \
  --since 2026-01-01 --until 2026-02-01 --min-file-size 1024 \
  --max-file-size 104857600 --format html

# MIME filtering; omit messages without a matching file.
ta archive --all --types channels --media all --mime 'video/*' \
  --only-media --format both

# Photos/videos in a date range, with JSON split into 50 MiB parts.
ta archive --chat me --since 2026-01-01 --until 2026-02-01 \
  --media photos,videos --max-file-size 104857600 --part-size 52428800

# Proxy and saved login namespace.
ta --proxy socks5://127.0.0.1:1080 -n personal archive --chat me
```

Name matching ignores case and extra spaces and searches main and archived chats,
including personal conversations. Bare names first match the full displayed chat
name, then fall back to usernames if no displayed name matches. Prefix a username
with `@`, or use a Telegram URL, to select it explicitly.
When several chats share a displayed name, ta lists their IDs and asks you to
choose an ID or `@username`. `ta chat ls` lists names and IDs.
`--export-name` changes the display name and folder label for one chat.
The original username stays in `archive-metadata.json`.
This option does not anonymize the backup.
Unsafe filename characters become underscores.

| Selection | Options |
| --- | --- |
| Chat types | `personal`, `bots`, `groups`, `channels`, `contacts`, `public_channels`, `private_channels`, `public_groups`, `private_groups` |
| Media types | `photos`, `videos`, `audios`, `stickers`, `animations`, `documents`, `voice_messages`, `video_messages`, `contacts`; also `all` or `none` |
| File filters | `--extensions pdf,zip` and `--mime 'video/*'` |
| File sizes | `--min-file-size` and `--max-file-size`, in bytes; bounds are inclusive and max `0` means unlimited |
| Dates | `--since` includes the start; `--until` excludes the end. Use `YYYY-MM-DD` or RFC3339. Date-only values use UTC. |
| Message filtering | `--only-media` keeps only messages with a matching downloadable file |
| Destination | `--output /path/to/backups` |

Media, extension, MIME and size filters limit downloads.
Message metadata stays unless `--only-media` removes the message.
File filters select all media categories unless you also pass `--media`.
Thumbnails follow their parent file's filters. Selecting `stickers` also downloads custom emoji.
Contacts can include vCards; locations and polls retain their metadata.
Skipped files keep their metadata and an explanation.

`--all` searches main and archived dialogs. It excludes Saved Messages.
Use `account` for Saved Messages and account contacts; explicit `--chat me` also works.
`--types contacts` selects chats with Telegram contacts.

`--include-migrated` defaults to true and includes a supergroup's original basic-group history.
That history gets a separate linked folder with its original IDs.
`--include-left --all --takeout` also looks for left channels.
Telegram may restrict these to your own messages; the export records that limit.

A failed chat does not stop the others. With resume enabled, a stalled transfer
returns early so ta can recreate the connection before continuing other chats.
`run-report.json` lists results and discovery warnings.
The command returns a nonzero exit status if any chat fails.

## Resume, refresh and speed

Rerun the same command to continue an interrupted export or fetch new messages.
Keep the output directory and filters the same.
ta checks complete files by byte size and reuses them. Partial files restart.
ta saves a page's checkpoint after its selected downloads finish.

| Mode | Behavior |
| --- | --- |
| `--resume=true` (default) | Resume completed pages and append new messages |
| `--refresh` | Also scan older history for edits, reactions and changed attachments |
| `--resume=false` | Rebuild the checkpoint from current Telegram history; reuse matching completed media |

Incremental and refresh runs keep messages already archived, even after Telegram deletes them.
An interrupted refresh continues from its last completed page.
For a fresh snapshot, use `--resume=false`. Keep older snapshots in a separate output directory.

```sh
ta archive --all --types channels --media all --format both --refresh
ta archive --chat me --resume=false --media photos,documents
```

Changing media, date, size, file filters or takeout mode requires a rebuild or a new output directory.
Changing the output format or HTML page size reuses existing history.
Published files remain available until their replacements are ready.

With resume enabled, `archive` retries dropped connections up to five times.
Each fresh session fetches new file references and reuses completed work.
History pages and media requests that stop responding also trigger this recovery
after two minutes without data or a server response. This includes connecting to
a media data center.
Successful responses and downloaded chunks reset the inactivity timer; Telegram's
explicit flood waits extend it. Set `--download-timeout 5m` for a longer limit or
`--download-timeout 0` to disable it. The timer does not limit the total duration of
a healthy download or history scan; each history page gets a fresh timer.
A running cron process uses the version it started with;
restart a stalled run after upgrading to enable this behavior.
Ctrl+C, authorization errors and local file errors stop the run.
`--resume=false` disables these session retries.

Telegram can return repeated timeouts for a document thumbnail while its video
or document remains downloadable. After three consecutive server timeout replies,
ta marks that thumbnail as unavailable and continues the backup. Original videos,
documents and photos still require successful downloads. Cancellation and local
file errors still stop the run.

Unavailable thumbnails appear as placeholders in JSON/HTML, with details in
`archive-metadata.json` and a warning in `run-report.json`. Progress excludes them
from the remaining download total and reports their count at completion.
Use `--refresh` to retry these thumbnails later.

ta reads history in pages of 100 messages. Downloads run in parallel:

| Setting | Default | Controls |
| --- | --- | --- |
| `-l`, `--limit` | `2` | Concurrent message downloads |
| `-t`, `--threads` | `4` | Parallel chunks per file |
| `--pool` | `8` | Connections per Telegram data center |

```sh
ta -l 4 -t 4 --pool 8 archive --chat me --media all
```

Before downloading, ta scans the pending history across all selected chats to
calculate the total download size. Completed local files and excluded media do
not count toward the remaining work. This preparation adds a history scan before
downloads start; it does not download media twice. A large first backup can spend
several minutes here. During this scan, progress shows the current chat, messages
scanned, files found and pending bytes found:

```text
Scanning Fadi
[................................]     --  1.0 GiB found
Elapsed 3m0s · 23500 messages scanned · 900 files found
```

The byte count grows as more pending files are found. Download percentage, speed
and ETA appear after the scan completes and the total size is known. Incremental
backups scan new or unfinished history; `--refresh` also revisits older messages.

One progress bar covers the entire selected chat backup. It shows a current
download's filename, downloaded/total bytes, speed, elapsed time and ETA:

```text
123_document.pdf
[############....................]  38.0%  114 MiB / 300 MiB
Elapsed 28s · ETA 46s · 4.1 MiB/s
```

With parallel downloads, the filename is the most recently started file that is
still downloading. The bar reaches 100% after the archive output is written.
Messages arriving after preparation are left for the next sync. If Telegram does
not report a file size, progress uses completed files and omits the total byte
size. Cron and redirected output receive periodic plain text updates.

Each chat ends with a `Finished archive` summary. It includes the finish time,
elapsed time, active filters, total archived messages and new messages added.
Media results are grouped by type, with thumbnails listed separately:

```text
Type        Downloaded  Reused  Filtered  Failed  Unavailable  Canceled
Photos      12          30      2         0       0            0
Videos      3           8       0         0       0            0
Thumbnails  8           20      0         0       1            0
```

These are counts for the current invocation. Downloaded means a complete,
verified file was saved. Reused means a matching local file was checked and kept.
Filtered means a visited file was excluded by the selected media, date, size,
extension or MIME filters. Excluded history is not scanned just to count it.
Failed and canceled transfers are listed separately. Retries and repeated file
references do not inflate counts, and files downloaded before a reconnect remain
counted as downloaded.

Unchanged history is retained without recounting every old media file. An
incremental run can therefore report the full archived message total alongside
zero new downloads. Stored unavailable thumbnails keep the result marked
`completed with warnings` until a refresh successfully replaces them.
The structured counts are also saved in `run-report.json` and account reports.

History stays on disk; output streams in date order.
Memory holds a history page, transfer buffers and statistics maps.
Account photo, story and contact lists keep their metadata in memory.
ta handles Telegram flood waits and reconnects.
Speed depends on Telegram, your account and your connection.
See [global options](global-options.md#transfer-and-connection-flags) for transfer settings.

Add `--takeout` to use Telegram's bulk export session for history and files.
Telegram may require approval or a waiting period. ta reports that delay.
Chat archives use normal history by default.

## JSON, HTML and offline rendering

Choose `--format json`, `--format html` or `--format both`.
JSON follows Telegram Desktop's chat schema. Extra fields live in `archive-metadata.json`.
Display dates use local time; Unix timestamps retain the instant. Activity statistics use UTC.

HTML uses Telegram Desktop v7.4.1's bundled CSS, JavaScript and icons.
It supports RTL text, nested formatting, forwards, replies, spoilers, polls, contacts and service messages.
Media previews, attachment cards, date separators and pagination work offline.
Open `export_results.html` for all chats, or a chat's `messages.html`.
`index.html` remains an alias for the chat export index. Chats share the root assets.
Animated TGS stickers and custom emoji can be downloaded; the reader does not play their animation.

| Output option | Behavior |
| --- | --- |
| `--html-page-size` | Messages per HTML page; default `1000` |
| `--part-size` | Byte budget per JSON part; `0` writes one `result.json` |

Split files use `result_partN.json`. Each part includes chat metadata.
A message larger than the part budget gets its own part.

Rebuild JSON or HTML from a local checkpoint, without contacting Telegram:

```sh
ta render data/ChatExport_channel_123 --format html --html-page-size 500
ta render data/ChatExport_channel_123 --format both
ta render data/AccountExport_123 --format both
```

`render` also updates older checkpoints to the current presentation and JSON schema.
Unchanged syncs reuse existing output. Missing or modified output files are rebuilt.
After messages change, sync rebuilds affected HTML pages and updates their statistics;
unchanged pages are reused. JSON and HTML share one message-decoding pass when both
are requested. Desktop-compatible JSON and the metadata sidecar remain complete
files, so they are rewritten when the archive changes. The first sync after upgrading
an older checkpoint rebuilds its rendering cache and metadata index once.
Secret chats are unavailable through this cloud-history client.
See [Desktop export coverage](desktop-compatibility.md) for supported fields and remaining differences.
Live export parity with Telegram Desktop has not been verified.

## Statistics

```sh
ta stats data/ChatExport_user_123/result.json
ta stats data/ChatExport_user_123/result_part*.json > combined-statistics.json
```

`stats` reads Desktop chat JSON, ta exports and legacy Python exports offline.
It streams multiple files or parts. Pass each part once; records are not deduplicated.
Statistics cover messages, service events, words, Unicode characters, replies, forwards and edits.
They also include first/last dates, activity by sender/day/hour, media counts and referenced file bytes.
File-byte totals include media skipped by download filters.

To add a statistic, update `Stats.Observe` and its fields in
[`internal/archive/stats.go`](../internal/archive/stats.go). Exports and offline analysis use the same code.
