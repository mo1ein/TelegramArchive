import json
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
        # https://github.com/eternnoir/pyTelegramBotAPI/issues/204
        async for message in app.get_chat_history(chat_ids):
            # print(message)
            print('clone!')
            msg_info = {}
            msg_info['id'] = message.id
            # TODO: type format should be str???
            msg_info['type'] = 'message'
            # date_unixtime
            msg_info['date'] = message.date
            if message.from_user.last_name is not None:
                name = f'{message.from_user.first_name} {message.from_user.last_name}'
                msg_info['from'] = name
            else:
                # TODO: for just last_name??
                msg_info['from'] = message.from_user.first_name
            msg_info['from_id'] = f'user{message.from_user.id}'

            if message.video is not None:
                msg_info['media_type'] = 'video_file'
                msg_info['mime_type'] = message.video.mime_type
                msg_info['duration_seconds'] = message.video.duration
                msg_info['width'] = message.video.width
                msg_info['height'] = message.video.height
            elif message.audio is not None:
                # file
                # thumbnail
                msg_info['media_type'] = 'audio_file'
                # performer
                # title
                msg_info['mime_type'] = message.audio.mime_type
                msg_info['duration_seconds'] = message.audio.duration
            elif message.sticker is not None:
                msg_info['media_type'] = 'sticker'
                # thumbnail
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
            elif message.voice is not None:
                msg_info['media_type'] = 'voice_message'
                msg_info['mime_type'] = message.voice.mime_type
                msg_info['duration_seconds'] = message.voice.duration
            elif message.video_note is not None:
                msg_info['media_type'] = 'video_note'
                msg_info['mime_type'] = message.video_note.mime_type
                msg_info['duration_seconds'] = message.video_note.duration
            elif message.document is not None:
                msg_info['media_type'] = 'document'
                # file
                msg_info['mime_type'] = message.document.mime_type
            else:
                # TODO
                pass

            # TODO: if message.forwarded_from is not None:
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


app.run(main())
