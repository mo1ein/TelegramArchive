
import json
import time
import datetime
from pyrogram import Client
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
        messages = []
        async for message in app.get_chat_history(chat_ids):
            # print(message)
            print('clone!')
            msg_info = {}
            msg_info['id'] = message.id
            # TODO: type format should be str???
            msg_info['type'] = 'message'
            msg_info['date'] = message.date
            msg_info['date_unixtime'] = convert_to_unixtime(message.date)
            # TODO: add date_unixtime
            if message.from_user.last_name is not None:
                name = f'{message.from_user.first_name} {message.from_user.last_name}'
                msg_info['from'] = name
            else:
                # TODO: for just last_name??
                msg_info['from'] = message.from_user.first_name
            msg_info['from_id'] = f'user{message.from_user.id}'

            # TODO: add file locations
            msg_info['file'] = ''
            msg_info['thumbnail'] = ''

            if message.forward_from_chat is not None:
                msg_info['forwarded_from'] = message.forward_from_chat.title
            elif message.forward_from is not None:
                msg_info['forwarded_from'] = message.forward_from.first_name
            # file
            # thumbnail
            if message.sticker is not None:
                msg_info['media_type'] = 'sticker'
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.video is not None:
                # thumbnail
                msg_info['media_type'] = 'video_file'
                msg_info['mime_type'] = message.video.mime_type
                msg_info['duration_seconds'] = message.video.duration
                msg_info['width'] = message.video.width
                msg_info['height'] = message.video.height
            elif message.video_note is not None:
                msg_info['media_type'] = 'video_note'
                msg_info['mime_type'] = message.video_note.mime_type
                msg_info['duration_seconds'] = message.video_note.duration
            elif message.audio is not None:
                msg_info['media_type'] = 'audio_file'
                msg_info['performer'] = message.audio.performer
                msg_info['title'] = message.audio.title
                msg_info['mime_type'] = message.audio.mime_type
                msg_info['duration_seconds'] = message.audio.duration
            elif message.voice is not None:
                msg_info['media_type'] = 'voice_message'
                msg_info['mime_type'] = message.voice.mime_type
                msg_info['duration_seconds'] = message.voice.duration
            elif message.document is not None:
                msg_info['media_type'] = 'document'
                msg_info['mime_type'] = message.document.mime_type
            else:
                # TODO
                pass

            # TODO: add hashtag and mention in text
            if message.text is not None:
                msg_info['text'] = message.text
            else:
                msg_info['text'] = ""
            messages.append(msg_info)

            # TODO: find username or chat_id info instead of this
            '''
            username = message.from_user.username
            if username != chat_ids or user_id != chat_ids:
                # TODO: find your info first before loop
                your_id = message.from_user.id
            '''
            # TODO: media, video ...
        with open('output.json', mode='w') as f:
            json.dump(messages, f, indent=4, default=str)


def convert_to_unixtime(date: datetime.datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


app.run(main())
