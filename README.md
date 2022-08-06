# Telegram-Archive
### Export telegram account chats
 - [x] All private chats
 - [x] Specefic chats (username or ID)
 - [x] export channels that restrict saving content
 - [ ] All contacts in your account

- #### All channels in your account
    - [x] public channels
    - [ ] private channels
- #### All groups in your account
    - [x] public groups
    - [ ] private groups

### Export media in each chat
You can choose to export and download each media type.
```
 MEDIA_EXPORT = {
    'audios': False,
    'videos': False,
    'photos': False,
    'stickers': False,
    'animations': True,
    'documents': False,
    'voice_messages': False,
    'video_messages': False,
    'contacts': True
}
```
And also each chat.
```
CHAT_EXPORT = {
    'contacts': False,
    'bot_chats': False,
    'personal_chats': True,
    'public_channels': False,
    'public_groups': False,
    'private_channels': False,
    'private_groups': False,
}
```

### Export assholes chat
- [ ] Export telegram chat for each T (time: minutes) to backup asshole people chat who delete chats both-side.

### Export format
- [x] json
- [ ] html

## Contribute
##### That's very enjoyable for me if you contribute. You can fix `TODO`'s or any problem, bug and tasks in `README`.
