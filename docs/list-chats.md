# List Chats

[Documentation index](README.md)

## List all chats

```sh
ta chat ls
```

## JSON Output

```sh
ta chat ls -o json
```

## Filter

Please refer to [Filter Guide](expressions.md) for basic knowledge about filter.

List all available filter fields:

```sh
ta chat ls -f -
```

List channels that VisibleName contains "Telegram":

```sh
ta chat ls -f "Type contains 'channel' && VisibleName contains 'Telegram'"
```

List groups that have topics:

```sh
ta chat ls -f "len(Topics)>0"
```
