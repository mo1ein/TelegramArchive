# Export Messages

[Documentation index](README.md)

Create a JSON download manifest for `ta download` or `ta forward`.
For a full chat backup with local media and checkpoints, use [archive](archiving.md).

### Chat references

`CHAT` can be a username (`example_channel` or `@example_channel`), a numeric ID,
a public link (`https://t.me/example_channel`) or a phone number.
Use `ta chat ls` to find IDs. Replace sample names and IDs with your own.

Empty CHAT means 'Saved Messages'

## All

Export all messages containing media to `ta-export.json`

```sh
ta chat export -c CHAT
```

## From Topic/Replies

Export media messages from specific topic:

Get Topic ID:

1. Message Link: `https://t.me/c/1234567890/100/123` (`100` is the topic ID)
2. `ta chat ls` command

```sh
ta chat export -c CHAT --topic 100
```

Export media messages from specific channel post replies:

```sh
ta chat export -c CHAT --reply 123
```

## Custom Destination

Export with specific output file path. Default: `ta-export.json`.

```sh
ta chat export -c CHAT -o /path/to/output.json
```

## Custom Type

### Time Range

Export with specific timestamp range. Default: `1970-01-01` - `NOW`

```sh
ta chat export -c CHAT -T time -i 1665700000,1665761624
```

`time` is also the default value of `-T` option, so you can omit it

```sh
ta chat export -c CHAT -i 1665700000,1665761624
```

### ID Range

Export with specific message id range. Default: `0` - `latest`

```sh
ta chat export -c CHAT -T id -i 100,500
```

### Last

Export last 100 media messages:

```sh
ta chat export -c CHAT -T last -i 100
```

## Filter

Please refer to [Filter Guide](expressions.md) for basic knowledge about filter.

List all available filter fields:

```sh
ta chat export -c CHAT -f -
```

Export last 10 zip files that `size > 5MiB` and `views > 200`:

```sh
ta chat export -c CHAT -T last -i 10 -f "Views>200 && Media.Name endsWith '.zip' && Media.Size > 5*1024*1024"
```

## With Content

Export with message content:

```sh
ta chat export -c CHAT --with-content
```

## Raw

Export Telegram MTProto raw message structure, which is useful for debugging.

```sh
ta chat export -c CHAT --raw
```

## Non-Media

Include text-only messages in the manifest:

```sh
ta chat export -c CHAT --all
```
