# Changelog

## Unreleased

### Changed

- **Breaking:** replace the Python application with the Go `ta` CLI. Build with
  Go 1.25.8 or newer and log in again when moving from a Python/Telethon session.
- Use command flags and optional exported `TA_*` settings for configuration.

### Added

- Incremental JSON and offline HTML chat exports with local media, checkpoints,
  statistics, date/size/type filters and joined-dialog discovery.
- Separate account backups, including Saved Messages and selectable account data.
- Recurring foreground sync and managed system cron jobs with per-job logs and
  protection against overlapping runs of the same job.
- Telegram Desktop export assets and formats; supported coverage and remaining
  differences are documented in `docs/desktop-compatibility.md`.

### Removed

- Python entrypoints, dependencies and runtime configuration from the maintained
  source tree, along with the old Docker setup and `.env` example.

### Security

- Exclude local Telegram data, sessions, logs, legacy files and generated caches
  from version control. Test fixtures use synthetic Telegram identifiers.
