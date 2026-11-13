# Account and scheduled backups

[Back to README](../README.md)

Use `account` for account data, `sync` for incremental backups, and `cron` for scheduled runs.
Chat selection and file filters are covered in [Archiving](archiving.md).

## Separate account backup

`account` writes to `data/AccountExport_<account ID>/`.
Saved Messages belong here alongside the other account sections.
A normal chat export does not start an account backup.

```sh
# All account sections and their media.
ta account --include all --media all --format both

# Selected sections.
ta account --include saved_messages,contacts,profile_photos,stories \
  --media all --format html

# Filters also apply to Saved Messages, profile photos and stories.
ta account --include saved_messages --media documents --extensions pdf \
  --since 2026-01-01 --max-file-size 104857600 --format both
```

Sections: `personal_info`, `contacts`, `frequent_contacts`, `profile_photos`,
`stories`, `saved_messages`, `sessions`, `web_sessions`, `other_data`.

The account's `result.json` and `export_results.html` summarize its sections.
`account.json` and `account.html` remain aliases.
Saved Messages appear under `chats.list`, with their files in `saved_messages/`.
Profile pictures and stories use Desktop's array formats and local media folders.
`account-report.json` records section failures. Other sections continue after a failure.
Previous successful section files remain available.
Session metadata excludes authorization hashes and session credentials.

Account backups use takeout by default for saved phone contacts and `other_data.json`.
With `--takeout=false`, contacts include registered Telegram contacts only.
`other_data` requires takeout. Telegram may ask you to approve the export or wait.

## Scheduled backups

### System cron

Install a real cron job using ta commands:

```sh
# Back up now, then sync every six hours.
ta cron add my-channel --chat "channel_username" --every 6h \
  --media all --format both --output ./data --run-now --at-startup

# All chats, media and separate account data, once a day.
ta cron add everything --everything --every 24h --format both --run-now

ta cron list
ta cron logs my-channel
ta cron run my-channel
ta cron remove my-channel
```

`cron add` installs or updates a named job in your user crontab. Other cron entries stay intact.
Each trigger runs one incremental `sync` and exits. You can close ta between runs.
`--run-now` starts the first backup after installation; omit it to install and exit.
If another backup is using the same output folder, `--run-now` and `ta cron run NAME`
wait for it to finish. Ctrl+C cancels waiting and keeps the installed cron job.
Automatic cron runs skip a busy output folder, and triggers are also skipped while
the same job is running. Failed jobs retry at the next scheduled time.

Cron only runs at scheduled times while the computer is on. It does not replay
backups missed while the computer was off or asleep. Add `--at-startup` to also
run one incremental backup when the system starts, keeping the regular schedule.
Startup runs wait for a shared output folder so each job gets a turn. They use
the saved login and settings, so no terminal needs to be open. Add or update the
same job without `--at-startup` to remove its startup trigger.

The last line of a completed job starts with `Finished job "NAME"`. It reports
the result, downloaded/reused/filtered/failed/unavailable file totals, finish time
and elapsed time. A table above it breaks down media types, and the archive
summary gives the total messages stored and the new messages added this run.
`completed with warnings` keeps unavailable thumbnails visible even on later
incremental runs. A skipped job says `skipped` and does not claim a completed
backup or reuse counts from a previous run. A failed job retains the work and
reports the partial results available.
If a child cannot provide a valid report, the result or counts are marked
unavailable rather than claiming success or inventing zero counts.

Inspect `backup-report.json` for the structured totals and component outcomes.
For live results, use `ta cron logs NAME` or `tail -f ~/.ta/cron/NAME.log`.

`--every 6h` means 00:00, 06:00, 12:00 and 18:00 in cron's timezone.
Use whole minutes that divide an hour, or whole hours that divide a day.
Examples are `15m`, `1h`, `6h` and `24h`. Use foreground sync for intervals such as `90m`.
This requires a working cron service and the `crontab` command on Linux/macOS.

Settings and logs stay private under `~/.ta/cron/`. Proxy credentials stay out of crontab.
ta saves the login namespace, session-storage path, proxy, working directory and absolute paths.
Keep the binary and working directory in place. Removing a job keeps its archives and logs.
For several channels on one account, use one job with `--chat first,second` or `--all --types channels`.
Concurrent jobs need separate saved logins and output roots.

#### Cron jobs for different accounts

Use a separate saved login namespace (`-n`) for each Telegram account.
Each cron job saves its selected namespace, so jobs for different accounts can run independently.

```sh
# List existing accounts and namespaces.
ta accounts

# Log in once per account; scan each QR with the matching Telegram account.
ta -n account1 login -T qr
ta -n account2 login -T qr

# Schedule one chat from each account with different job names and output roots.
ta -n account1 cron add account1-chat --chat "first_channel" --every 6h \
  --media all --format both --output "$HOME/backups/account1"

ta -n account2 cron add account2-chat --chat "second_channel" --every 6h \
  --media all --format both --output "$HOME/backups/account2"

ta cron list
```

Replace `first_channel` and `second_channel` with the desired chat usernames, URLs or IDs.
If an account is already saved under `default`, use `-n default` for its job and skip that login step.
Job names are shared across namespaces: reusing a name updates the existing job.
Keep a separate output root for each account. Add `--run-now` to also start a backup immediately.

### Foreground sync

`sync` runs once unless you add `--every`. With an interval, keep the process running.
It downloads the selected history first, then fetches new messages.
With `--all`, each cycle also discovers newly joined chats.

```sh
# Everything, every six hours.
ta sync --everything --every 6h --format both

# Joined channels, with file filters.
ta sync --all --types channels --extensions pdf,mp4 \
  --max-file-size 104857600 --every 1h --format both

# Chats plus selected account sections.
ta sync --all --media all --account-include contacts,saved_messages \
  --every 6h --format both

# Account sections only.
ta sync --chats=false --account-include contacts,profile_photos \
  --media all --every 24h --format both

# One backup cycle.
ta sync --everything --format both
```

A bare `sync` selects all chats matching `--types`.
Add `--account` or `--account-include` for account data.
`--everything` selects all chats, account sections and media. Explicit filters still apply.
Account data keeps its separate directory and takeout default.
Explicit `--takeout=false` applies to both chat and account exports.

Chat histories and Saved Messages keep archived deletions.
Contacts, sessions and asset indexes refresh as current metadata; previous media files stay on disk.
Unchanged chats reuse their JSON, HTML and statistics.

Each cycle runs chats first, then account sections. A failed chat phase does not prevent the account phase.
`backup-report.json` records the latest cycle. Per-chat and account reports provide the details.
An output-root lock prevents overlapping syncs, even with separate login namespaces.
One-time syncs wait for a busy output folder; recurring syncs skip the busy cycle
by default. `--skip-busy` makes a one-time sync skip too; `--skip-busy=false`
makes recurring runs wait. `--job-timeout` also limits time spent waiting.
The `.backup.lock` file remains between runs; its presence does not mean a backup
is active. The operating system releases the lock when the process exits.
The login is released between phases and while waiting.
Failed cycles retry at the next interval. Missed intervals are skipped.
Ctrl+C or SIGTERM cancels the current cycle; restarting continues from checkpoints.

Use `--refresh` to update older edits and reactions too.
Recurring sync requires resume and rejects `--resume=false`.
Change filters through a one-time rebuild or a new output directory.
`--runs 3` limits cycles. `--job-timeout 2h` limits each cycle.
`--stop-on-error` stops after a failed cycle.

To run it in the background:

```sh
nohup ta sync --everything --every 6h --format both --output ./data \
  > backup.log 2>&1 &
```

A service manager can restart foreground sync after reboot.
`ta schedule --target archive|account|sync --every 6h -- [target flags]` also remains available.
