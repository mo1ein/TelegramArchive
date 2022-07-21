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
    'public_channels': True,
    'public_groups': True,
    'private_channels': True,
    'private_groups': True,
}
# TODO: add const sentences for print
