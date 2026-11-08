# Download

[Documentation index](README.md)

## From Links

### Message links

Use Telegram's Copy Link action. Supported examples include:

- `https://t.me/example_channel/123`
- `https://t.me/c/1234567890/123`
- `https://t.me/example_group/100/123`
- `https://t.me/c/1234567890/100/123`
- `https://t.me/example_channel/123?comment=456`
- `https://t.me/example_group/123?thread=100`

```sh
ta dl -u https://t.me/example_channel/123 -u https://t.me/example_channel/124
```

## From JSON

There are two ways to export the JSON you need:

### ta manifest

Create a media download manifest with `ta chat export`.

Please refer to [Export Messages](export-messages.md)

### Desktop Client

1. Choose the dialog you want to export, and click the three dots in the upper right corner, then
   click `Export Chat History`.
2. Uncheck all boxes(you don't need to download them now) and set `Size Limit` to minimum
3. Set Format to `JSON` and select the time period you need.
4. Export it! And `result.json` is what you need.

```sh
ta dl -f result1.json -f result2.json
```

## Combine Sources

```sh
ta dl \
-u https://t.me/example_channel/123 -u https://t.me/example_channel/124 \
-f result1.json -f result2.json
```

## Custom Destination

Download files to custom directory

```sh
ta dl -u https://t.me/example_channel/123 -d /path/to/dir
```

## Custom Parameters

Download with 8 threads per task, 4 concurrent tasks:

```sh
ta dl -u https://t.me/example_channel/123 -t 8 -l 4
```

## Descending Order

Download files in descending order(from newest to oldest)

Different order will affect resuming download

```sh
ta dl -f result.json --desc
```

## MIME Detection

If the file extension is not matched with the MIME type, ta will rename the file with the correct extension.

Side effect: like `.apk` file, it will be renamed to `.zip`.

```sh
ta dl -u https://t.me/example_channel/123 --rewrite-ext
```

## Album/Grouped Detection

Automatically detect if the message is an album/grouped message and download all of them.

```sh
ta dl -u https://t.me/example_channel/123 --group
```

## Auto Skip

Skip the same files(name and size) when downloading.

```sh
ta dl -u https://t.me/example_channel/123 --skip-same
```

## Takeout Session

Download files
with a [Telegram takeout session](https://core.telegram.org/api/takeout):

Takeout is intended for bulk exports. Telegram may require approval or a waiting period.

```sh
ta dl -u https://t.me/example_channel/123 --takeout
```

## Filters

Download files with extension filters:

The extension is only matched with the file name, not the MIME type. So it may not work as expected.

Whitelist and blacklist can not be used at the same time.

Whitelist: Only download files with `.jpg` `.png` extension

```sh
ta dl -u https://t.me/example_channel/123 -i jpg,png
```

Blacklist: Download all files except `.mp4` `.flv` extension

```sh
ta dl -u https://t.me/example_channel/123 -e mp4,flv
```

## Name Template

Download with custom file name template:

Please refer to [Template Guide](templates.md) for more details.

```sh
ta dl -u https://t.me/example_channel/123 \
--template "{{ .DialogID }}_{{ .MessageID }}_{{ .DownloadDate }}_{{ .FileName }}"
```

## Resume/Restart

Resume without UI interaction:

```sh
ta dl -u https://t.me/example_channel/123 --continue
```

Restart without UI interaction:

```sh
ta dl -u https://t.me/example_channel/123 --restart
```

## Serve

Expose the files as an HTTP server instead of downloading them with built-in downloader

The server listens on all network interfaces and has no authentication.
Use it only on a trusted network or behind access controls. The default port is `8080`.

This is useful when you want to download files with a download manager like `aria2`/`wget`/`axel`/`IDM`...

```sh
ta dl -u https://t.me/example_channel/123 --serve
```

With custom port:

```sh
ta dl -u https://t.me/example_channel/123 --serve --port 8081
```
