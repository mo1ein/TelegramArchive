# Filters, captions and routing

[Documentation index](README.md)

Chat filters, upload captions and message routers use the Expr language.
Download filename templates use [Go templates](templates.md), with different syntax.

## Discover fields

Pass `-` to the relevant option to list its available fields:

```sh
ta chat ls -f -
ta chat export -c CHAT -f -
ta up -p /path/to/file --caption -
ta up -p /path/to/file --to -
ta forward --from ta-export.json --edit -
ta forward --from ta-export.json --to -
```

Fields differ between commands. A chat filter can use `Type`, `VisibleName` and `Topics`.
An upload caption can use `FileName`, `FileExt` and `MIME`.

## Filters

A filter returns true for records to keep:

```sh
ta chat ls -f 'Type contains "channel" && VisibleName contains "Telegram"'
ta chat export -c CHAT -f 'Media.Name endsWith ".zip" && Media.Size > 5*1024*1024'
```

Use `&&`, `||` and parentheses to combine conditions.
Strings support operations such as `contains` and `endsWith`.

## Captions and destinations

A caption or edit expression returns a string.
A destination expression returns a chat reference or `{Peer: "CHAT", Thread: 100}`.
An empty destination string selects Saved Messages:

```sh
ta up -p /path/to/file --caption 'FileName + " - uploaded by ta"'
ta up -p /path/to/file --to 'MIME contains "video" ? "CHAT1" : ""'
ta forward --from ta-export.json --to 'Message.Views > 100 ? "CHAT1" : {Peer: "CHAT2", Thread: 100}'
```

For longer expressions, put the expression in a text file and pass its path.
See [uploads](upload.md) and [forwarding](forward.md) for HTML formatting and file examples.
The [Expr language reference](https://expr-lang.org/docs/language-definition) documents the full syntax.
