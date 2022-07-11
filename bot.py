
import os
import json
import time
from datetime import datetime
from pyrogram import Client
from pyrogram.enums import ChatType
from config import APP_ID, API_HASH, DOWNLOAD_MEDIA

app = Client(
    "my_bot",
    api_id=APP_ID,
    api_hash=API_HASH,
)

# TODO: list of users
chat_ids = 'mo_ein'


async def main():
    async with app:
        await app.send_message('me', 'ping!')
        chat = await app.get_chat(chat_ids)
        # print(chat)

        voice_num = 0
        photo_num = 0
        video_message_num = 0
        messages = []
        chat_data = {}

        if chat.type == ChatType.PRIVATE:
            user_info = await app.get_users(chat_ids)
            # print(user_info)
            chat_data['name'] = user_info.first_name
            chat_data['type'] = 'personal_chat'
            chat_data['id'] = user_info.id
        elif chat.type == ChatType.CHANNEL:
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_channel'
            # print(chat)

            # when using telegram api ids have -100 prefix
            # https://stackoverflow.com/questions/33858927/how-to-obtain-the-chat-id-of-a-private-telegram-channel
            if str(chat.id).startswith('-100'):
                chat_data['id'] = str(chat.id)[4::]

        chat_export_date = datetime.now().strftime("%Y-%m-%d")
        # TODO: use username instead of name
        chat_export_name = f'ChatExport_{chat_data["name"]}_{chat_export_date}'
        # TODO: when user want other path
        path = ''

        async for message in app.get_chat_history(chat_ids):
            # TODO: add initial message of channel
            # print(message)
            # print('clone!')
            msg_info = {}
            msg_info['id'] = message.id
            # TODO: type format should be str???
            msg_info['type'] = 'message'
            # TODO: correct date
            msg_info['date'] = message.date.strftime('%Y-%m-%dT%H:%M:%S')
            msg_info['date_unixtime'] = convert_to_unixtime(message.date)

            if chat.type != ChatType.CHANNEL:
                name = ''
                if message.from_user.first_name is not None:
                    name += message.from_user.first_name
                if message.from_user.last_name is not None:
                    if name != '':
                        name += f' {message.from_user.last_name}'
                    else:
                        name += message.from_user.last_name
                msg_info['from'] = name
                msg_info['from_id'] = f'user{message.from_user.id}'
            else:
                msg_info['from'] = chat.title
                if str(message.sender_chat.id).startswith('-100'):
                    msg_info['from_id'] = f'channel{str(message.sender_chat.id)[4::]}'

            if message.reply_to_message_id is not None:
                msg_info['reply_to_message_id'] = message.reply_to_message_id

            if message.forward_from_chat is not None:
                msg_info['forwarded_from'] = message.forward_from_chat.title
            elif message.forward_from is not None:
                msg_info['forwarded_from'] = message.forward_from.first_name

            if message.sticker is not None:
                if DOWNLOAD_MEDIA['sticker'] is True:
                    media_dir = f'{chat_export_name}/stickers'
                    os.makedirs(media_dir, exist_ok=True)
                    sticker_name = f'{media_dir}/{message.sticker.file_name}'
                    await app.download_media(
                        message.sticker.file_id,
                        sticker_name
                    )
                    thumbnail_name = f'{media_dir}/{message.sticker.file_name}_thumb.jpg'
                    await app.download_media(
                        message.sticker.thumbs[0].file_id,
                        thumbnail_name
                    )
                    msg_info['file'] = f'stickers/{message.sticker.file_name}'
                    msg_info['thumbnail'] = f'stickers/{message.sticker.file_name}_thumb.jpg'
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'sticker'
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.photo is not None:
                if DOWNLOAD_MEDIA['photo'] is True:
                    photo_num += 1
                    os.makedirs(f'{chat_export_name}/photos', exist_ok=True)
                    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
                    # TODO: what format for png??
                    photo_name = f'{chat_export_name}/photos/file_{photo_num}@{date}.jpg'
                    await app.download_media(
                        message.photo.file_id,
                        photo_name
                    )
                    msg_info['photo'] = f'photos/file_{photo_num}@{date}.jpg'
                else:
                    msg_info['photo'] = "(File not included. Change data exporting settings to download.)"
                msg_info['width'] = message.photo.width
                msg_info['height'] = message.photo.height
            elif message.video is not None:
                if DOWNLOAD_MEDIA['video'] is True:
                    media_dir = f'{chat_export_name}/video_files'
                    os.makedirs(
                        media_dir,
                        exist_ok=True
                    )
                    video_name = f'{media_dir}/{message.video.file_name}'
                    await app.download_media(
                        message.video.file_id,
                        video_name
                    )
                    thumbnail_name = f'{media_dir}/{message.video.file_name}_thumb.jpg'
                    await app.download_media(
                        message.video.thumbs[0].file_id,
                        thumbnail_name
                    )
                    msg_info['file'] = f'video_files/{message.video.file_name}'
                    msg_info['thumbnail'] = f'video_files/{message.video.file_name}_thumb.jpg'

                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'video_file'
                msg_info['mime_type'] = message.video.mime_type
                msg_info['duration_seconds'] = message.video.duration
                msg_info['width'] = message.video.width
                msg_info['height'] = message.video.height
            # TODO: media_type animation
            elif message.video_note is not None:
                if DOWNLOAD_MEDIA['video_message'] is True:
                    # TODO: video_message_num is not correct because we read messages last to first
                    video_message_num += 1
                    media_dir = f'{chat_export_name}/round_video_messages'
                    os.makedirs(
                        media_dir,
                        exist_ok=True
                    )
                    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
                    video_message_name = f'{media_dir}/file_{video_message_num}@{date}.mp4'
                    # TODO: if not downloaded??
                    await app.download_media(
                        message.video_note.file_id,
                        video_message_name
                    )
                    thumbnail_name = f'{media_dir}/file_{video_message_num}@{date}.mp4_thumb.jpg'
                    await app.download_media(
                        message.video_note.thumbs[0].file_id,
                        thumbnail_name
                    )
                    msg_info['file'] = f'round_video_messages/file_{video_message_num}@{date}.mp4'
                    msg_info['thumbnail'] = f'round_video_messages/file_{video_message_num}@{date}.mp4_thumb.jpg'
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'video_message'
                msg_info['mime_type'] = message.video_note.mime_type
                msg_info['duration_seconds'] = message.video_note.duration
            elif message.audio is not None:
                if DOWNLOAD_MEDIA['audio'] is True:
                    media_dir = f'{chat_export_name}/files'
                    os.makedirs(
                        media_dir,
                        exist_ok=True
                    )
                    audio_name = f'{media_dir}/{message.audio.file_name}'
                    await app.download_media(
                        message.audio.file_id,
                        audio_name
                    )
                    msg_info['file'] = f'files/{message.audio.file_name}'
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'audio_file'
                msg_info['performer'] = message.audio.performer
                msg_info['title'] = message.audio.title
                msg_info['mime_type'] = message.audio.mime_type
                msg_info['duration_seconds'] = message.audio.duration
            elif message.voice is not None:
                # TODO: voice_num is not correct because we read messages last to first
                voice_num += 1
                if DOWNLOAD_MEDIA['voice_message'] is True:
                    media_dir = f'{chat_export_name}/voice_messages'
                    os.makedirs(
                        media_dir,
                        exist_ok=True
                    )
                    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
                    voice_name = f'{media_dir}/audio_{voice_num}@{date}.ogg'
                    await app.download_media(
                        message.voice.file_id,
                        voice_name
                    )
                    msg_info['file'] = f'voice_messages/audio_{voice_num}@{date}.ogg'
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'voice_message'
                msg_info['mime_type'] = message.voice.mime_type
                msg_info['duration_seconds'] = message.voice.duration
            elif message.document is not None:
                if DOWNLOAD_MEDIA['document'] is True:
                    media_dir = f'{chat_export_name}/files'
                    os.makedirs(media_dir, exist_ok=True)
                    doc_name = f'{media_dir}/{message.document.file_name}'
                    await app.download_media(
                        message.document.file_id,
                        doc_name
                    )
                    if message.document.thumbs is not None:
                        thumbnail_name = f'{media_dir}/{message.document.file_name}_thumb.jpg'
                        await app.download_media(
                            message.document.thumbs[0].file_id,
                            thumbnail_name
                        )
                        msg_info['thumbnail'] = f'files/{message.document.file_name}_thumb.jpg'
                    msg_info['file'] = f'files/{message.document.file_name}'
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    # TODO: if have thumbnail??
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                # msg_info['media_type'] = 'document'
                msg_info['mime_type'] = message.document.mime_type

            # TODO: add hashtag
            # TODO: add mentions
            # TODO: add links, hrefs...

            if message.text is not None:
                msg_info['text'] = message.text
            else:
                msg_info['text'] = ""

            messages.append(msg_info)
            # for start first message in json
            messages.reverse()
            chat_data['messages'] = messages

        with open('output.json', mode='w') as f:
            json.dump(chat_data, f, indent=4, default=str)


def convert_to_unixtime(date: datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


app.run(main())
