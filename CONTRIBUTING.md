# Contributing

ta is a Go CLI. Building requires Go 1.25.8 or newer.

Create a short-lived branch from `main` and keep each commit focused on one
change. Describe the problem, resulting behavior and verification in your pull
request. Discuss larger features in an issue before implementing them.

## Build and verify

```sh
make build
make test
make race
make vet
git diff --check
```

Format changed Go files with `gofmt`. Run `make bench` when changing archive
throughput or memory use, and report the workload and results. Keep transfer-core
and Telegram protocol dependencies compatible; their pinned versions are in
`go.mod` and [the compatibility notes](docs/desktop-compatibility.md).

Tests should use synthetic chats and fake Telegram boundaries. Live-account
tests require a separate login and should never run automatically in the test
suite. See [the compatibility notes](docs/desktop-compatibility.md) before
claiming Telegram Desktop export parity.

## Protect private data

Use fictional channel names, usernames, IDs, messages and media in tests and
documentation. Redact account details, chat names, export paths, proxy credentials
and invitation links from bug reports and screenshots.

Never commit Telegram sessions, authorization keys, account backups, downloaded
messages or media. `.gitignore` excludes local runtime data and legacy files,
but review the staged paths and diff before every commit:

```sh
git status --short
git diff --cached --stat
git diff --cached
```

Public upstream names and license notices are attribution and must be retained.
The bundled Desktop icons under `internal/archive/desktop_assets/images/` are
source assets and must be included in Git.
Their license notices are in [licenses/](licenses/).

## Existing users

Document command or output changes in `CHANGELOG.md` and update the README when
behavior changes. Keep the existing Git history; the Python implementation
remains accessible in earlier commits.
