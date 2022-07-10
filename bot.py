
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

        if chat.type == ChatType.PRIVATE:
            user_info = await app.get_users(chat_ids)
            # print(user_info)
            chat_data['name'] = user_info.first_name
            chat_data['type'] = chat.type
            chat_data['id'] = user_info.id
        elif chat.type == ChatType.CHANNEL:
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_channel'
            # print(chat)
            # when using telegram api ids have -100 prefix
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
            msg_info['date'] = message.date
            msg_info['date_unixtime'] = convert_to_unixtime(message.date)

            if chat.type != ChatType.CHANNEL:
                name = ''
                if message.from_user.first_name is not None:
                    name += message.from_user.first_name
                if message.from_user.last_name is not None:
                    name += f' {message.from_user.last_name}'
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

            # TODO: add file locations
            msg_info['file'] = ''
            msg_info['thumbnail'] = ''

            if message.sticker is not None:
                msg_info['media_type'] = 'sticker'
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.video is not None:
                # thumbnail
                # file
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
            chat_data['messages'] = messages
            # TODO: reverse dic for start first message

        with open('output.json', mode='w') as f:
            json.dump(chat_data, f, indent=4, default=str)


def convert_to_unixtime(date: datetime.datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


app.run(main())
