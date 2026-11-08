# Export Members

[Documentation index](README.md)

Export chat members/subscribers, admins, bots, etc.

Chat administrator permission is required for some types of members(kicked, banned, etc.).

### Chat references

`CHAT` can be a username (`example_channel` or `@example_channel`), a numeric ID,
a public link (`https://t.me/example_channel`) or a phone number.
Use `ta chat ls` to find IDs. Replace sample names and IDs with your own.

## All

Export all users to `ta-users.json`

```sh
ta chat users -c CHAT
```

## Custom Destination

Export with specified file path

```sh
ta chat users -c CHAT -o /path/to/export.json
```

## Raw

Export Telegram MTProto raw user structure, which is useful for debugging.

```sh
ta chat users -c CHAT --raw
```
