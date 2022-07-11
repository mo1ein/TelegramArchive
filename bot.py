
import json
import time
import datetime
from pyrogram import Client
from pyrogram.enums import ChatType
from config import APP_ID, API_HASH

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
        messages = []
        chat_data = {}

        # TODO: choosed by user
        download_media = {
            'document': False,
             'audio': False, 
             'voice': False,
             'video': False, 
             'photo': True, 
             'video_message': True
        }
        dirs = ['stickers', 'voice_messages', 'video_files']
        path = ''
        voice_num = 0
        # folders: voice_messages, video_files, stickers 

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

            if message.forward_from_chat is not None:
                msg_info['forwarded_from'] = message.forward_from_chat.title
            elif message.forward_from is not None:
                msg_info['forwarded_from'] = message.forward_from.first_name

            if message.sticker is not None:
                if download_media['sticker'] is True:
                    await app.download_media(
                        message.sticker.file_id, 
                        message.sticker.file_name
                    )
                    thumbnail_name = f'{path}{message.sticker.file_name}_thumb.jpg'
                    await app.download_media(
                        message.sticker.thumbs.file_id, 
                        thumbnail_name
                    )
                    msg_info['file'] = path + message.sticker.file_name
                    msg_info['thumbnail'] = thumbnail_name
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'sticker'
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.photo is not None:
                if download_media['photo'] is True:
                    await app.download_media(
                        message.photo.file_id, 
                        message.photo.file_name
                    )
                    msg_info['photo'] = path + message.photo.file_name
                else:
                    msg_info['photo'] = "(File not included. Change data exporting settings to download.)"
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.video is not None:
                if download_media['video'] is True:
                    await app.download_media(
                        message.video.file_id, 
                        message.video.file_name
                    )
                    thumbnail_name = f'{path}{message.video.file_name}_thumb.jpg'
                    await app.download_media(
                        message.video.thumbs.file_id, 
                        thumbnail_name
                    )
                    msg_info['file'] = path + message.video.file_name
                    msg_info['thumbnail'] = thumbnail_name
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
                if download_media['video_message'] is True:
                    # TODO: if not downloaded??
                    await app.download_media(
                        message.video_note.file_id, 
                        message.video_note.file_name
                    )

                    thumbnail_name = f'{path}{message.video_note.file_name}_thumb.jpg'
                    await app.download_media(
                        message.video_note.thumbs.file_id, 
                        thumbnail_name
                    )
                    msg_info['file'] = path + message.video_note.file_name
                    msg_info['thumbnail'] = thumbnail_name
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'video_message'
                msg_info['mime_type'] = message.video_note.mime_type
                msg_info['duration_seconds'] = message.video_note.duration
            elif message.audio is not None:
                if download_media['audio'] is True:
                    await app.download_media(
                        message.audio.file_id, 
                        message.audio.file_name
                    )
                    msg_info['file'] = path + message.audio.file_name
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
                if download_media['voice'] is True:
                    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
                    voice_name = f'audio_{voice_num}@{date}.ogg'
                    await app.download_media(
                        message.voice.file_id, 
                        voice_name
                    )
                    msg_info['file'] = path + voice_name
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'voice_message'
                msg_info['mime_type'] = message.voice.mime_type
                msg_info['duration_seconds'] = message.voice.duration
            elif message.document is not None:
                if download_media['document'] is True:
                    await app.download_media(
                        message.document.file_id, 
                        message.document.file_name
                    )
                    thumbnail_name = f'{path}{message.document.file_name}_thumb.jpg'
                    await app.download_media(
                        message.document.thumbs.file_id, 
                        thumbnail_name
                    )
                    msg_info['file'] = path + message.document.file_name
                    msg_info['thumbnail'] = thumbnail_name
                else:
                    msg_info['file'] = "(File not included. Change data exporting settings to download.)"
                    msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
                msg_info['media_type'] = 'document'
                msg_info['mime_type'] = message.document.mime_type
            else:
                # there is no media in message!
                pass

            # TODO: reply_to_message
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


def convert_to_unixtime(date: datetime.datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


app.run(main())
