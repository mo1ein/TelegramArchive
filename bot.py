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
        async for message in app.get_chat_history(chat_ids):
            # print(message)
            print('clone!')
            msg_info = {}
            msg_info['id'] = message.id
            # TODO: type format should be str???
            msg_info['type'] = 'message'
            msg_info['date'] = message.date
            msg_info['from'] = message.from_user.first_name
            # TODO:
            # msg_info['last_name'] = message.from_user.last_name
            msg_info['from_id'] = f'user{message.from_user.id}'
            msg_info['text'] = message.text
            if message.video is not None:
                msg_info['media_type'] = 'video_file'
                msg_info['mime_type'] = message.video.mime_type
            elif message.audio is not None:
                msg_info['media_type'] = 'audio_file'
                msg_info['mime_type'] = message.audio.mime_type
            elif message.sticker is not None:
                msg_info['media_type'] = 'sticker'
                # TODO: sticker emojie, width, height...
            elif message.voice is not None:
                msg_info['media_type'] = 'voice_message'
                msg_info['mime_type'] = message.audio.mime_type
                # TODO: duraction time
            elif message.document is not None:
                # TODO
                pass
            else:
                # TODO
                pass
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
