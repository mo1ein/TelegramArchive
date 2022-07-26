import os

API_ID = int(os.environ.get("API_ID"))
API_HASH = os.environ.get("API_HASH")

# TODO: set 'files' instead of 'documents'??
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

CHAT_EXPORT = {
    'contacts': False,
    'personal_chats': True,
    'public_channels': False,
    'public_groups': False,
    'private_channels': False,
    'private_groups': False,
}

FILE_NOT_FOUND = '(File not included. Change data exporting settings to download.)'
