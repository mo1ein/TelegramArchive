import os

APP_ID = int(os.environ.get("APP_ID"))
API_HASH = os.environ.get("API_HASH")

DOWNLOAD_MEDIA = {
    'audio': False,
    'video': False,
    'photo': True,
    'sticker': True,
    'document': True,
    'voice_message': True,
    'video_message': True
}
