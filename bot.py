
import os
import json
import time
from datetime import datetime
from pyrogram import Client
from pyrogram.types import Message
from pyrogram.enums import ChatType, MessageEntityType
from config import APP_ID, API_HASH, DOWNLOAD_MEDIA

app = Client(
    "my_bot",
    api_id=APP_ID,
    api_hash=API_HASH,
)

# TODO: list of users
chat_ids = 'mo_ein'
# chat_ids = 'me'


async def main():
    async with app:
        await app.send_message('me', 'ping!')
        chat = await app.get_chat(chat_ids)
        # print(chat)

        voice_num = 0
        photo_num = 0
        video_message_num = 0
        username = ''
        messages = []
        chat_data = {}

        if chat.type == ChatType.PRIVATE:
            user_info = await app.get_users(chat_ids)
            username = user_info.username
            chat_data['name'] = user_info.first_name
            chat_data['type'] = 'personal_chat'
            chat_data['id'] = user_info.id
        elif chat.type == ChatType.CHANNEL:
            username = chat.username
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_channel'

            # when using telegram api ids have -100 prefix
            # https://stackoverflow.com/questions/33858927/how-to-obtain-the-chat-id-of-a-private-telegram-channel
            if str(chat.id).startswith('-100'):
                chat_data['id'] = str(chat.id)[4::]

        chat_export_date = datetime.now().strftime("%Y-%m-%d")
        chat_export_name = f'ChatExport_{username}_{chat_export_date}'
        # TODO: when user want other path
        path = ''

        async for message in app.get_chat_history(chat_ids):
            # TODO: add initial message of channel
            print(message)
            # print('clone!')
            msg_info = {}
            msg_info['id'] = message.id
            # TODO: type format should be str???
            msg_info['type'] = 'message'
            # TODO: correct date
            msg_info['date'] = message.date.strftime('%Y-%m-%dT%H:%M:%S')
            msg_info['date_unixtime'] = convert_to_unixtime(message.date)

            # set chat name
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

            # TODO: media_type animation??

            if message.sticker is not None:
                await get_sticker_data(message, msg_info, chat_export_name)
            elif message.photo is not None:
                photo_num += 1
                await get_photo_data(message, msg_info, chat_export_name, photo_num)
            elif message.video is not None:
                await get_video_data(message, msg_info, chat_export_name)
            elif message.video_note is not None:
                video_message_num += 1
                await get_video_note_data(
                    message,
                    msg_info,
                    chat_export_name,
                    video_message_num
                )
            elif message.audio is not None:
                await get_audio_data(message, msg_info, chat_export_name)
            elif message.voice is not None:
                # TODO: voice_num is not correct because we read messages last to first
                voice_num += 1
                await get_voice_data(message, msg_info, chat_export_name, voice_num)
            elif message.document is not None:
                await get_document_data(message, msg_info, chat_export_name)

            # TODO: add hashtag
            # TODO: add mentions
            # TODO: add links, hrefs...
            if message.text is not None:
                text = get_text_data(message, 'text')
                if text != []:
                    text.append(message.text)
                    msg_info['text'] = text
                else:
                    msg_info['text'] = message.text
            elif message.caption is not None:
                text = get_text_data(message, 'caption')
                if text != []:
                    text.append(message.caption)
                    msg_info['text'] = text
                else:
                    msg_info['text'] = message.caption
            else:
                msg_info['text'] = ''

            messages.append(msg_info)
            # for start first message in json
            messages.reverse()
            chat_data['messages'] = messages

        with open('output.json', mode='w') as f:
            json.dump(chat_data, f, indent=4, default=str)


async def get_sticker_data(message: Message, msg_info: dict, chat_export_name: str):
    if DOWNLOAD_MEDIA['sticker'] is True:
        media_dir = f'{chat_export_name}/stickers'
        os.makedirs(media_dir, exist_ok=True)
        sticker_name = f'{media_dir}/{message.sticker.file_name}'
        try:
            # TODO: None returned??
            await app.download_media(
                message.sticker.file_id,
                sticker_name
            )
            msg_info['file'] = f'stickers/{message.sticker.file_name}'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"

        thumbnail_name = f'{media_dir}/{message.sticker.file_name}_thumb.jpg'
        # TODO: if have thumb?
        try:
            # TODO: None returned??
            await app.download_media(
                message.sticker.thumbs[0].file_id,
                thumbnail_name
            )
            msg_info['thumbnail'] = f'stickers/{message.sticker.file_name}_thumb.jpg'
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"

    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
        msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    msg_info['media_type'] = 'sticker'
    msg_info['sticker_emoji'] = message.sticker.emoji
    msg_info['width'] = message.sticker.width
    msg_info['height'] = message.sticker.height


async def get_photo_data(
        message: Message,
        msg_info: dict,
        chat_export_name: str,
        photo_num: int
):
    if DOWNLOAD_MEDIA['photo'] is True:
        os.makedirs(f'{chat_export_name}/photos', exist_ok=True)
        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        # TODO: what format for png??
        photo_name = f'{chat_export_name}/photos/file_{photo_num}@{date}.jpg'
        try:
            # TODO: None returned??
            await app.download_media(
                message.photo.file_id,
                photo_name
            )
            msg_info['photo'] = f'photos/file_{photo_num}@{date}.jpg'
        except ValueError:
            print("Oops can't download media!")
            msg_info['photo'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['photo'] = "(File not included. Change data exporting settings to download.)"
    msg_info['width'] = message.photo.width
    msg_info['height'] = message.photo.height


async def get_video_data(message: Message, msg_info: dict, chat_export_name: str):
    if DOWNLOAD_MEDIA['video'] is True:
        media_dir = f'{chat_export_name}/video_files'
        os.makedirs(
            media_dir,
            exist_ok=True
        )
        video_name = f'{media_dir}/{message.video.file_name}'
        try:
            # TODO: None returned??
            await app.download_media(
                message.video.file_id,
                video_name
            )
            msg_info['file'] = f'video_files/{message.video.file_name}'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"

        thumbnail_name = f'{media_dir}/{message.video.file_name}_thumb.jpg'
        # TODO: if have thumb?
        try:
            # TODO: None returned??
            await app.download_media(
                message.video.thumbs[0].file_id,
                thumbnail_name
            )
            msg_info['thumbnail'] = f'video_files/{message.video.file_name}_thumb.jpg'
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
        msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    msg_info['media_type'] = 'video_file'
    msg_info['mime_type'] = message.video.mime_type
    msg_info['duration_seconds'] = message.video.duration
    msg_info['width'] = message.video.width
    msg_info['height'] = message.video.height


async def get_video_note_data(
    message: Message,
    msg_info: dict,
    chat_export_name: str,
    video_message_num: int
):
    if DOWNLOAD_MEDIA['video_message'] is True:
        # TODO: video_message_num is not correct because we read messages last to first
        media_dir = f'{chat_export_name}/round_video_messages'
        os.makedirs(
            media_dir,
            exist_ok=True
        )
        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        video_message_name = f'{media_dir}/file_{video_message_num}@{date}.mp4'
        # TODO: if not downloaded??
        try:
            await app.download_media(
                message.video_note.file_id,
                video_message_name
            )
            msg_info['file'] = f'round_video_messages/file_{video_message_num}@{date}.mp4'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"

        thumbnail_name = f'{media_dir}/file_{video_message_num}@{date}.mp4_thumb.jpg'
        # TODO: if have thumb?
        try:
            await app.download_media(
                message.video_note.thumbs[0].file_id,
                thumbnail_name
            )
            msg_info['thumbnail'] = f'round_video_messages/file_{video_message_num}@{date}.mp4_thumb.jpg'
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
        msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    msg_info['media_type'] = 'video_message'
    msg_info['mime_type'] = message.video_note.mime_type
    msg_info['duration_seconds'] = message.video_note.duration


async def get_audio_data(message: Message, msg_info: dict, chat_export_name: str):
    if DOWNLOAD_MEDIA['audio'] is True:
        media_dir = f'{chat_export_name}/files'
        os.makedirs(
            media_dir,
            exist_ok=True
        )
        audio_name = f'{media_dir}/{message.audio.file_name}'
        try:
            await app.download_media(
                message.audio.file_id,
                audio_name
            )
            msg_info['file'] = f'files/{message.audio.file_name}'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
    msg_info['media_type'] = 'audio_file'
    msg_info['performer'] = message.audio.performer
    msg_info['title'] = message.audio.title
    msg_info['mime_type'] = message.audio.mime_type
    msg_info['duration_seconds'] = message.audio.duration


async def get_voice_data(
        message: Message,
        msg_info: dict,
        chat_export_name: str,
        voice_num: int
):
    if DOWNLOAD_MEDIA['voice_message'] is True:
        media_dir = f'{chat_export_name}/voice_messages'
        os.makedirs(
            media_dir,
            exist_ok=True
        )
        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        voice_name = f'{media_dir}/audio_{voice_num}@{date}.ogg'
        try:
            await app.download_media(
                message.voice.file_id,
                voice_name
            )
            msg_info['file'] = f'voice_messages/audio_{voice_num}@{date}.ogg'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
    msg_info['media_type'] = 'voice_message'
    msg_info['mime_type'] = message.voice.mime_type
    msg_info['duration_seconds'] = message.voice.duration


async def get_document_data(message: Message, msg_info: dict, chat_export_name: str):
    if DOWNLOAD_MEDIA['document'] is True:
        media_dir = f'{chat_export_name}/files'
        os.makedirs(media_dir, exist_ok=True)
        doc_name = f'{media_dir}/{message.document.file_name}'
        try:
            await app.download_media(
                message.document.file_id,
                doc_name
            )
            msg_info['file'] = f'files/{message.document.file_name}'
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = "(File not included. Change data exporting settings to download.)"

        if message.document.thumbs is not None:
            thumbnail_name = f'{media_dir}/{message.document.file_name}_thumb.jpg'
            try:
                await app.download_media(
                    message.document.thumbs[0].file_id,
                    thumbnail_name
                )
                msg_info['thumbnail'] = f'files/{message.document.file_name}_thumb.jpg'
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    else:
        msg_info['file'] = "(File not included. Change data exporting settings to download.)"
        # TODO: if have thumbnail??
        msg_info['thumbnail'] = "(File not included. Change data exporting settings to download.)"
    msg_info['mime_type'] = message.document.mime_type

def get_text_data(message: Message, text_mode: str) -> list:
    text = []
    if text_mode == 'caption':
        if message.caption_entities is not None:
            entities = message.caption_entities
        else:
            return text
    elif text_mode == 'text':
        if message.text.entities is not None:
            entities = message.text.entities
        else:
            return text

    #TODO: bug. remove entitiy part from all message
    for e in entities:
        txt = {}
        if e.type == MessageEntityType.URL:
            txt['type'] = 'link'
        elif e.type == MessageEntityType.HASHTAG:
            txt['type'] = 'hashtag'
        elif e.type == MessageEntityType.CASHTAG:
            txt['type'] = 'cashtag'
        elif e.type == MessageEntityType.BOT_COMMAND:
            txt['type'] = 'bot_command'
        elif e.type == MessageEntityType.MENTION:
            txt['type'] = 'mention'
        elif e.type == MessageEntityType.EMAIL:
            txt['type'] = 'email'
        elif e.type == MessageEntityType.PHONE_NUMBER:
            txt['type'] = 'phone_number'
        elif e.type == MessageEntityType.BOLD:
            txt['type'] = 'bold'
        elif e.type == MessageEntityType.ITALIC:
            txt['type'] = 'italic'
        elif e.type == MessageEntityType.UNDERLINE:
            txt['type'] = 'underline'
        elif e.type == MessageEntityType.STRIKETHROUGH:
            txt['type'] = 'strikethrough'
        elif e.type == MessageEntityType.SPOILER:
            txt['type'] = 'spoiler'
        elif e.type == MessageEntityType.CODE:
            # TODO: other parts??
            txt['type'] = 'code'
        elif e.type == MessageEntityType.PRE:
            txt['type'] = 'pre'
            txt['language'] = ''
        elif e.type == MessageEntityType.BLOCKQUOTE:
            # TODO: other parts??
            txt['type'] = 'blockquote'
        elif e.type == MessageEntityType.TEXT_LINK:
            txt['type'] = 'text_link'
            txt['href'] = e.url
        elif e.type == MessageEntityType.TEXT_MENTION:
            # TODO: other parts??
            txt['type'] = 'text_mention'
        elif e.type == MessageEntityType.BANK_CARD:
            # TODO: other parts??
            txt['type'] = 'bank_card'
        # TODO: add other formats...
        else:
            # TODO: other parts??
            txt['type'] = 'unknown'

        if text_mode == 'text':
            txt['text'] = message.text[e.offset:e.length]
        else:
            txt['text'] = message.caption[e.offset:e.length]
        text.append(txt)
    return text


def convert_to_unixtime(date: datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


app.run(main())
