# Upload

[Documentation index](README.md)

## Upload Files

Upload specified files and directories to `Saved Messages`:

```sh
ta up -p /path/to/file -p /path/to/dir
```

## Custom Destination

Upload to custom chat.

### Chat references

`CHAT` can be a username (`example_channel` or `@example_channel`), a numeric ID,
a public link (`https://t.me/example_channel`) or a phone number.
Use `ta chat ls` to find IDs. Replace sample names and IDs with your own.

### Specific Chat

Upload to specific one chat:

```sh
ta up -p /path/to/file -c CHAT
```

Upload to specific topic in a forum chat:

```sh
ta up -p /path/to/file -c CHAT --topic 100
```

### Message Routing

Upload to different chats by message router which is based on [expression](expressions.md).

The `--to` flag is conflicted with the `-c/--chat` and `--topic` flags. You can only use one of them.

List all available fields:

```sh
ta up -p /path/to/file --to -
```

Upload to `CHAT1` if MIME contains `video`, otherwise upload to `Saved Messages`:

You must return a **string** or **struct** as the target CHAT, and empty string means upload to `Saved Messages`.

```sh
ta up -p /path/to/file \
--to 'MIME contains "video" ? "CHAT1" : ""'
```

Upload to `CHAT1` if MIME contains `video`, otherwise upload to reply to message/topic `4` in `CHAT2`:

```sh
ta up -p /path/to/file \
--to 'MIME contains "video" ? "CHAT1" : { Peer: "CHAT2", Thread: 4 }'
```

Pass a file name if the expression is complex:

### router.txt

Write your expression like `switch`:

```javascript
MIME contains "video" ? "CHAT1" :
FileExt contains ".mp3" ? "CHAT2" :
FileName contains "chat3" ? {Peer: "CHAT3", Thread: 101} :
""
```

```sh
ta up -p /path/to/file --to router.txt
```

## Custom Parameters

Upload with 8 threads per task, 4 concurrent tasks:

```sh
ta up -p /path/to/file -t 8 -l 4
```

## Custom Caption

Custom caption is based on [expression](expressions.md).

List all available fields:

```sh
ta up -p /path/to/file --caption -
```

Custom simple caption:
```sh
ta up -p /path/to/file --caption 'FileName + " - uploaded by ta"'
```

Write styled message with [HTML](https://core.telegram.org/bots/api#html-style):
```sh
ta up -p /path/to/file --caption  \
'FileName + `<b>Bold</b> <a href="https://example.com">Link</a>`'
```

Pass a file name if the expression is complex:

### caption.txt

```javascript
repeat(FileName, 2) + `
<a href="https://www.google.com">Google</a>
<a href="https://www.bing.com">Bing</a>
<b>bold</b>
<i>italic</i>
<code>code</code>
<tg-spoiler>spoiler</tg-spoiler>
<pre><code class="language-go">
package main

import "fmt"

func main() {
    fmt.Println("hello world")
}
</code></pre>
` + MIME
```

```sh
ta up -p /path/to/file --caption caption.txt
```

## Filters

Upload files with extension filters:

The extension is only matched with the file name, not the MIME type. So it may not work as expected.

Whitelist and blacklist can not be used at the same time.

Whitelist: Only upload files with `.jpg` `.png` extension

```sh
ta up -p /path/to/file -p /path/to/dir -i jpg,png
```

Blacklist: Upload all files except `.mp4` `.flv` extension

```sh
ta up -p /path/to/file -p /path/to/dir -e mp4 -e flv
```

## Delete Local

Delete the uploaded file after uploading successfully:

```sh
ta up -p /path/to/file --rm
```

## Photo

Upload images as photos instead of documents:

```sh
ta up -p /path/to/file --photo
```
