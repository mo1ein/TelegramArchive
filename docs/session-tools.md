# Session backup and recovery

[Documentation index](README.md)

These commands manage saved logins and settings. For Telegram content backups, see [Backups](backups.md).

## Backup

Backup all namespace data to a file. Default: `<date>.backup.ta`.

```sh
ta backup
```

Or specify the output file:

```sh
ta backup -d /path/to/custom.backup.ta
```

## Recover

Recover your data from a ta backup file. Existing namespace data will be overwritten.

```sh
ta recover -f /path/to/custom.backup.ta
```

## Migrate

Migrate your data to another storage

See [Storage Flag](global-options.md#--storage) for storage option details.

Migrate current storage to file storage:
```sh
ta migrate --to type=file,path=/path/to/data.json
```

Migrate custom source storage to file storage:
```sh
ta migrate --storage type=bolt,path=/path/to/data-directory  --to type=file,path=/path/to/data.json
```
