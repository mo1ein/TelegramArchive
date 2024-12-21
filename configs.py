import os
import json

from dotenv import dotenv_values

env = dotenv_values(".env")


API_ID = int(env.get("API_ID"))
API_HASH = env.get("API_HASH")

CHAT_IDS = env.get("CHAT_IDS")
CHAT_IDS = json.loads(CHAT_IDS)

# TODO: set 'files' instead of 'documents'??
MEDIA_EXPORT = {
    'audios': env.get("MEDIA_EXPORT_AUDIOS").lower() in ('true', '1', 't'),
    'videos': env.get("MEDIA_EXPORT_VIDEOS").lower() in ('true', '1', 't'),
    'photos': env.get("MEDIA_EXPORT_PHOTOS").lower() in ('true', '1', 't'),
    'stickers': env.get("MEDIA_EXPORT_STICKERS").lower() in ('true', '1', 't'),
    'animations': env.get("MEDIA_EXPORT_ANIMATIONS").lower() in ('true', '1', 't'),
    'documents': env.get("MEDIA_EXPORT_DOCUMENTS").lower() in ('true', '1', 't'),
    'voice_messages': env.get("MEDIA_EXPORT_VOICE_MESSAGES").lower() in ('true', '1', 't'),
    'video_messages': env.get("MEDIA_EXPORT_VIDEO_MESSAGES").lower() in ('true', '1', 't'),
    'contacts': env.get("MEDIA_EXPORT_CONTACTS").lower() in ('true', '1', 't'),
}

CHAT_EXPORT = {
    'contacts': env.get("CHAT_EXPORT_CONTACTS").lower() in ('true', '1', 't'),
    'bot_chats': env.get("CHAT_EXPORT_BOT_CHATS").lower() in ('true', '1', 't'),
    'personal_chats': env.get("CHAT_EXPORT_PERSONAL_CHATS").lower() in ('true', '1', 't'),
    'public_channels': env.get("CHAT_EXPORT_PUBLIC_CHANNELS").lower() in ('true', '1', 't'),
    'public_groups': env.get("CHAT_EXPORT_PUBLIC_GROUPS").lower() in ('true', '1', 't'),
    'private_channels': env.get("CHAT_EXPORT_PRIVATE_CHANNELS").lower() in ('true', '1', 't'),
    'private_groups': env.get("CHAT_EXPORT_PRIVATE_GROUPS").lower() in ('true', '1', 't'),
}

# Negative value means no splitting
MAX_FILE_SIZE = int(env.get("MAX_FILE_SIZE"))

FILE_NOT_FOUND = '(File not included. Change data exporting settings to download.)'

