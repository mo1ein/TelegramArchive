# Changelog

## Unreleased

### Changed

- Require Go 1.26 or newer for the Bubble Tea v2 terminal interface.
- Open the interactive workspace when `ta` runs without arguments in a terminal;
  piped invocations and `ta --help` continue to display command help.
- **Breaking:** replace the Python application with the Go `ta` CLI. Build with
  Go 1.26 or newer and log in again when moving from a Python/Telethon session.
- Use command flags and optional exported `TA_*` settings for configuration.

### Added

- `ta tui`: a responsive Bubble Tea workspace covering public commands and their
  options, including installed extensions. Includes quick-start presets,
  command and option search, common/advanced forms, inline validation, command
  previews, path completion and keyboard help. Tasks stay in the TUI with
  retained results, live progress, input prompts and cancellation. Native account,
  chat and schedule browsers provide searchable lists, details and action
  buttons; offline statistics show summary cards. Mouse and keyboard navigation
  preserve list selection when opening follow-up workflows.
- Install ta with `go install github.com/mo1ein/TelegramArchive/cmd/ta@latest`
  or `make install` from a local checkout.
- Incremental JSON and offline HTML chat exports with local media, checkpoints,
  statistics, date/size/type filters and joined-dialog discovery.
- Separate account backups, including Saved Messages and selectable account data.
- Recurring foreground sync and managed system cron jobs with per-job logs and
  protection against overlapping runs of the same job.
- Archive and cron job completion summaries with message totals, media outcomes
  by type, active filters, finish times and elapsed time. Counts are also saved
  in structured reports and stay accurate across retries and skipped runs.
- Telegram Desktop export assets and formats; supported coverage and remaining
  differences are documented in `docs/desktop-compatibility.md`.

### Fixed

- Stream chat-browser pages into the TUI before loading finishes, with visible
  connection stages, bounded requests, proxy fallback support and partial
  results retained when loading is stopped. Avoid fetching every forum's topics
  before showing chats.
- Import native Telegram for macOS sessions, including the website-installed
  client, with read-only SQLCipher/Postbox readers, local passcode support and
  committed WAL snapshots. Preserve the native API profile and the Desktop
  default. `ta login --check` verifies data without saving a login or connecting.
- Recognize sandboxed Telegram Desktop data on macOS and report access errors
  with macOS permission guidance.
- Convert poll answer variants from gotd v0.145.1 so the CLI builds with the
  updated Telegram dependency.
- Continue backups after repeated Telegram timeouts for an optional thumbnail,
  retaining the original media and reporting the unavailable preview. Existing
  thumbnail warnings remain visible until a successful refresh replaces them.

### Removed

- Python entrypoints, dependencies and runtime configuration from the maintained
  source tree, along with the old Docker setup and `.env` example.

### Security

- Exclude local Telegram data, sessions, logs, legacy files and generated caches
  from version control. Test fixtures use synthetic Telegram identifiers.
