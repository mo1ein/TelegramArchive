# Forward

[Documentation index](README.md)

Forward messages with automatic fallback and message routing

One-liner to forward messages from `https://t.me/telegram/193` to `Saved Messages`:

```sh
ta forward --from https://t.me/telegram/193
```

## Custom Source

### Message links

Use Telegram's Copy Link action. Supported examples include:

- `https://t.me/example_channel/123`
- `https://t.me/c/1234567890/123`
- `https://t.me/example_group/100/123`
- `https://t.me/c/1234567890/100/123`
- `https://t.me/example_channel/123?comment=456`
- `https://t.me/example_group/123?thread=100`

You can forward messages from links and [exported JSON files](download.md#from-json):

```sh
ta forward \
    --from https://t.me/telegram/193 \
    --from https://t.me/telegram/195 \
    --from ta-export.json \
    --from ta-export2.json
```

## Custom Destination

### Chat references

`CHAT` can be a username (`example_channel` or `@example_channel`), a numeric ID,
a public link (`https://t.me/example_channel`) or a phone number.
Use `ta chat ls` to find IDs. Replace sample names and IDs with your own.

### Specific Chat

Forward to specific one chat:

```sh
ta forward --from ta-export.json --to CHAT
```

### Message Routing

Forward to different chats by message router which is based on [expression](expressions.md).

List all available fields:

```sh
ta forward --from ta-export.json --to -
```

Forward to `CHAT1` if message contains `foo`, otherwise forward to `Saved Messages`:

You must return a **string** or **struct** as the target CHAT, and empty string means forward to `Saved Messages`.

```sh
ta forward --from ta-export.json \
    --to 'Message.Message contains "foo" ? "CHAT1" : ""'
```

Forward to `CHAT1` if message contains `foo`, otherwise forward to reply to message/topic `4` in `CHAT2`:

```sh
ta forward --from ta-export.json \
--to 'Message.Message contains "foo" ? "CHAT1" : { Peer: "CHAT2", Thread: 4 }'
```

Pass a file name if the expression is complex:

### router.txt

Write your expression like `switch`:
```javascript
Message.Message contains "foo" ? "CHAT1" :
From.ID == 123456 ? "CHAT2" :
Message.Views > 30 ? { Peer: "CHAT3", Thread: 101 } :
""
```

```sh
ta forward --from ta-export.json --to router.txt
```

## Mode

Forward messages with automatic fallback strategy.

Available modes:
- `direct` (default)
- `clone`

### Direct

Prefer to use official forward API.

If the chat or message is not allowed to use official forward API, it will be automatically downgraded to `clone` mode.

```sh
ta forward --from ta-export.json --mode direct
```

### Clone

Forward messages by copying them, which doesn't have forwarded header.

Some message content can't be copied, such as poll, invoice, etc. They will be ignored.

```sh
ta forward --from ta-export.json --mode clone
```

## Edit

Edit the message before forwarding based on [expression](expressions.md).

- You must pass the first message of grouped photos to edit the caption.
- You can pass any message of grouped documents to edit the corresponding comment.

You can reference relevant fields from the original message in the expression.

List all available fields:
```sh
ta forward --from ta-export.json --edit -
```

Append `Test Forwarded Message` to the original message:
```sh
ta forward --from ta-export.json --edit 'Message.Message + " Test Forwarded Message"'
```

Write styled message with [HTML](https://core.telegram.org/bots/api#html-style):
```sh
ta forward --from ta-export.json --edit \
    'Message.Message + `<b>Bold</b> <a href="https://example.com">Link</a>`'
```

Pass a file name if the expression is complex:

### edit.txt

```javascript
repeat(Message.Message, 2) + `
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
` + From.VisibleName
```

```sh
ta forward --from ta-export.json --edit edit.txt
```

## Dry Run

Print the progress without actually sending messages, which is useful for message routing debugging.

```sh
ta forward --from ta-export.json --dry-run
```

## Silent

Send messages without notification.

```sh
ta forward --from ta-export.json --silent
```

## No Grouped Detection

By default, ta will detect grouped messages and forward them as an album.

You can disable this behavior by `--single` to forward it as a single message.

```sh
ta forward --from ta-export.json --single
```

## Descending Order

Forward messages in descending order for each source.

```sh
ta forward --from ta-export.json --desc
```
