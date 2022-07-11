import os

APP_ID = int(os.environ.get("APP_ID"))
API_HASH = os.environ.get("API_HASH")

DOWNLOAD_MEDIA = {
    'audio': False,
    'video': True,
    'photo': True,
    'sticker': False,
    'document': False,
    'voice_message': False,
    'video_message': False
}
