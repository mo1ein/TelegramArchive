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
You can choose to export and download each media type in `.env` file.
```
MEDIA_EXPORT_AUDIOS=false
MEDIA_EXPORT_VIDEOS=false
MEDIA_EXPORT_PHOTOS=true
MEDIA_EXPORT_STICKERS=false
MEDIA_EXPORT_ANIMATIONS=false
MEDIA_EXPORT_DOCUMENTS=false
MEDIA_EXPORT_VOICE_MESSAGES=false
MEDIA_EXPORT_VIDEO_MESSAGES=false
MEDIA_EXPORT_CONTACTS=false
```
And also each chat.
```
CHAT_EXPORT_CONTACTS=False
CHAT_EXPORT_BOT_CHATS=False
CHAT_EXPORT_PERSONAL_CHATS=True
CHAT_EXPORT_PUBLIC_CHANNELS=False
CHAT_EXPORT_PUBLIC_GROUPS=False
CHAT_EXPORT_PRIVATE_CHANNELS=False
CHAT_EXPORT_PRIVATE_GROUPS=False
```

### Export assholes chat
- [ ] Export telegram chat for each T (time: minutes) to backup asshole people chat who delete chats both-side.

### Export format
- [x] json
- [ ] html

## Contribute
Read [CONTRIBUTING.md](CONTRIBUTING.md).
