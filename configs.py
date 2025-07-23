import os
import json
from dotenv import load_dotenv

# Load .env in local dev (has no effect in Docker unless .env is mounted/copied)
load_dotenv()

def str_to_bool(val):
    return str(val).lower() in ('true', '1', 't', 'yes')

API_ID = int(os.environ.get("API_ID"))
API_HASH = os.environ.get("API_HASH")

CHAT_IDS = json.loads(os.environ.get("CHAT_IDS", "[]"))

# todo: check if this path exist? or not and go to failed and panic
DOWNLOAD_PATH = os.environ.get("DOWNLOAD_PATH")

MEDIA_EXPORT = {
    'audios': str_to_bool(os.environ.get("MEDIA_EXPORT_AUDIOS")),
    'videos': str_to_bool(os.environ.get("MEDIA_EXPORT_VIDEOS")),
    'photos': str_to_bool(os.environ.get("MEDIA_EXPORT_PHOTOS")),
    'stickers': str_to_bool(os.environ.get("MEDIA_EXPORT_STICKERS")),
    'animations': str_to_bool(os.environ.get("MEDIA_EXPORT_ANIMATIONS")),
    'documents': str_to_bool(os.environ.get("MEDIA_EXPORT_DOCUMENTS")),
    'voice_messages': str_to_bool(os.environ.get("MEDIA_EXPORT_VOICE_MESSAGES")),
    'video_messages': str_to_bool(os.environ.get("MEDIA_EXPORT_VIDEO_MESSAGES")),
    'contacts': str_to_bool(os.environ.get("MEDIA_EXPORT_CONTACTS")),
}

CHAT_EXPORT = {
    'contacts': str_to_bool(os.environ.get("CHAT_EXPORT_CONTACTS")),
    'bot_chats': str_to_bool(os.environ.get("CHAT_EXPORT_BOT_CHATS")),
    'personal_chats': str_to_bool(os.environ.get("CHAT_EXPORT_PERSONAL_CHATS")),
    'public_channels': str_to_bool(os.environ.get("CHAT_EXPORT_PUBLIC_CHANNELS")),
    'public_groups': str_to_bool(os.environ.get("CHAT_EXPORT_PUBLIC_GROUPS")),
    'private_channels': str_to_bool(os.environ.get("CHAT_EXPORT_PRIVATE_CHANNELS")),
    'private_groups': str_to_bool(os.environ.get("CHAT_EXPORT_PRIVATE_GROUPS")),
}

page_size = os.environ.get('JSON_FILE_PAGE_SIZE', '')
JSON_FILE_PAGE_SIZE = None if page_size.lower() in ('', 'none') else int(page_size)

FILE_NOT_FOUND = '(File not included. Change data exporting settings to download.)'
