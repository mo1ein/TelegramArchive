
import os
import json
import time
from datetime import datetime
from pyrogram import Client
from pyrogram.types import Message
from pyrogram.enums import ChatType, MessageEntityType
from config import API_ID, API_HASH, MEDIA_EXPORT, CHAT_EXPORT, FILE_NOT_FOUND

app = Client(
    "my_bot",
    api_id=API_ID,
    api_hash=API_HASH,
)

# TODO: list of users
# if you want to get "saved messages", use "me" or "myself"
# TODO: ids with number must be int fix that
chat_ids = 'mo_ein'


async def main():
    # with should delete??
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
        read_messages = True

        if CHAT_EXPORT['contacts'] is True:
            read_messages = False
            # list
            contacts = await get_contact_data()
            # TODO:
            chat_data['about'] = ''

            tmp = {}
            # TODO:
            tmp['about'] = ''
            tmp['list'] = contacts
            chat_data['contacts'] = tmp
            # TODO:
            chat_data['frequent_contacts'] = {}
        elif (chat.type == ChatType.PRIVATE
                and CHAT_EXPORT['personal_chats'] is True):
            user_info = await app.get_users(chat_ids)
            username = user_info.username
            chat_data['name'] = user_info.first_name
            chat_data['type'] = 'personal_chat'
            chat_data['id'] = user_info.id
        elif (chat.type == ChatType.CHANNEL
                and CHAT_EXPORT['public_channels'] is True):
            username = chat.username
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_channel'
            # when using telegram api ids have -100 prefix
            # https://stackoverflow.com/questions/33858927/how-to-obtain-the-chat-id-of-a-private-telegram-channel
            if str(chat.id).startswith('-100'):
                chat_data['id'] = str(chat.id)[4::]
            else:
                chat_data['id'] = chat.id
        elif (chat.type == ChatType.GROUP
                and CHAT_EXPORT['public_groups'] is True):
            username = chat.username
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_group'
            if str(chat.id).startswith('-100'):
                chat_data['id'] = str(chat.id)[4::]
            else:
                chat_data['id'] = chat.id
        elif (chat.type == ChatType.SUPERGROUP
                and CHAT_EXPORT['public_groups'] is True):
            username = chat.username
            chat_data['name'] = chat.title
            chat_data['type'] = 'public_supergroup'
            if str(chat.id).startswith('-100'):
                chat_data['id'] = str(chat.id)[4::]
            else:
                chat_data['id'] = chat.id
        # TODO: private SUPERGROUP and GROUP
        else:
            # bot
            pass

        if read_messages:
            # TODO: add exception for private channels
            # read messages from first to last
            all_messages = [m async for m in app.get_chat_history(chat_ids)]
            all_messages.reverse()
            for message in all_messages:
                # TODO: add initial message of channel
                # print(message)
                # print('clone!')
                msg_info = {}
                msg_info['id'] = message.id
                msg_info['type'] = 'message'
                msg_info['date'] = message.date.strftime('%Y-%m-%dT%H:%M:%S')
                msg_info['date_unixtime'] = convert_to_unixtime(message.date)

                # set chat name
                # this part is so shitty fix THIS
                if chat.type == ChatType.PRIVATE:
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
                    # TODO: is this correct for groups?
                    if chat.type == ChatType.CHANNEL:
                        if str(message.sender_chat.id).startswith('-100'):
                            msg_info['from_id'] = f'channel{str(message.sender_chat.id)[4::]}'
                        else:
                            msg_info['from_id'] = f'channel{str(message.sender_chat.id)}'
                    else:
                        pass
                        # print(message)
                        # TODO: use from user...

                if message.reply_to_message_id is not None:
                    msg_info['reply_to_message_id'] = message.reply_to_message_id

                if message.forward_from_chat is not None:
                    msg_info['forwarded_from'] = message.forward_from_chat.title
                elif message.forward_from is not None:
                    msg_info['forwarded_from'] = message.forward_from.first_name

                # TODO: type service. actor...

                if message.sticker is not None:
                    names = generate_file_name(
                        message,
                        'sticker',
                        username
                    )
                    await get_sticker_data(message, msg_info, names)
                elif message.animation is not None:
                    names = generate_file_name(message, 'animation', username)
                    await get_animation_data(
                        message,
                        msg_info,
                        names
                    )
                elif message.photo is not None:
                    photo_num += 1
                    names = generate_file_name(
                        message,
                        'photo',
                        username,
                        photo_num
                    )
                    await get_photo_data(
                        message,
                        msg_info,
                        names
                    )
                elif message.video is not None:
                    names = generate_file_name(message, 'video', username)
                    await get_video_data(message, msg_info, names)
                elif message.video_note is not None:
                    video_message_num += 1
                    names = generate_file_name(
                        message,
                        'video_note',
                        username,
                        video_message_num
                    )
                    await get_video_note_data(
                        message,
                        msg_info,
                        names
                    )
                elif message.audio is not None:
                    names = generate_file_name(message, 'audio', username)
                    await get_audio_data(message, msg_info, names)
                elif message.voice is not None:
                    voice_num += 1
                    names = generate_file_name(
                        message,
                        'voice',
                        username,
                        voice_num
                    )
                    await get_voice_data(
                        message,
                        msg_info,
                        names
                    )
                elif message.document is not None:
                    names = generate_file_name(message, 'document', username)
                    await get_document_data(
                        message,
                        msg_info,
                        names
                    )

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
                chat_data['messages'] = messages

        with open('output.json', mode='w') as f:
            json.dump(chat_data, f, indent=4, default=str)


async def get_sticker_data(
    message: Message,
    msg_info: dict,
    names: tuple
) -> None:
    if MEDIA_EXPORT['stickers'] is True:
        sticker_path, thumb_path, sticker_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.sticker.file_id,
                sticker_path
            )
            msg_info['file'] = sticker_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.sticker.thumbs is not None:
            try:
                # TODO: None returned??
                await app.download_media(
                    message.sticker.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'sticker'
    msg_info['sticker_emoji'] = message.sticker.emoji
    msg_info['width'] = message.sticker.width
    msg_info['height'] = message.sticker.height


async def get_animation_data(
    message: Message,
    msg_info: dict,
    names: tuple
) -> None:
    if MEDIA_EXPORT['animations'] is True:
        animation_path, thumb_path, animation_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.animation.file_id,
                animation_path
            )
            msg_info['file'] = animation_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.animation.thumbs is not None:
            try:
                await app.download_media(
                    message.animation.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND

    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'animation'
    msg_info['mime_type'] = message.animation.mime_type
    msg_info['width'] = message.animation.width
    msg_info['height'] = message.animation.height


async def get_photo_data(
    message: Message,
    msg_info: dict,
    names: tuple
):
    if MEDIA_EXPORT['photos'] is True:
        photo_path, photo_relative_path = names
        try:
            await app.download_media(
                message.photo.file_id,
                photo_path
            )
            msg_info['photo'] = photo_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['photo'] = FILE_NOT_FOUND
    else:
        msg_info['photo'] = FILE_NOT_FOUND
    msg_info['width'] = message.photo.width
    msg_info['height'] = message.photo.height


async def get_video_data(
    message: Message,
    msg_info: dict,
    names: tuple
) -> None:
    if MEDIA_EXPORT['videos'] is True:
        video_path, thumb_path, video_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.video.file_id,
                video_path
            )
            msg_info['file'] = video_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.video.thumbs is not None:
            try:
                await app.download_media(
                    message.video.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'video_file'
    msg_info['mime_type'] = message.video.mime_type
    msg_info['duration_seconds'] = message.video.duration
    msg_info['width'] = message.video.width
    msg_info['height'] = message.video.height


async def get_video_note_data(
    message: Message,
    msg_info: dict,
    names: tuple
):
    if MEDIA_EXPORT['video_messages'] is True:
        vnote_path, thumb_path, vnote_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.video_note.file_id,
                vnote_path
            )
            msg_info['file'] = vnote_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.video_note.thumbs is not None:
            try:
                await app.download_media(
                    message.video_note.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'video_message'
    msg_info['mime_type'] = message.video_note.mime_type
    msg_info['duration_seconds'] = message.video_note.duration


async def get_audio_data(
    message: Message,
    msg_info: dict,
    names: tuple
) -> None:
    if MEDIA_EXPORT['audios'] is True:
        audio_path, thumb_path, audio_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.audio.file_id,
                audio_path
            )
            msg_info['file'] = audio_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.audio.thumbs is not None:
            try:
                await app.download_media(
                    message.audio.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'audio_file'
    msg_info['performer'] = message.audio.performer
    msg_info['title'] = message.audio.title
    msg_info['mime_type'] = message.audio.mime_type
    msg_info['duration_seconds'] = message.audio.duration


async def get_voice_data(
        message: Message,
        msg_info: dict,
        names: tuple
) -> None:
    if MEDIA_EXPORT['voice_messages'] is True:
        voice_path, voice_relative_path = names
        try:
            await app.download_media(
                message.voice.file_id,
                voice_path
            )
            msg_info['file'] = voice_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
    msg_info['media_type'] = 'voice_message'
    msg_info['mime_type'] = message.voice.mime_type
    msg_info['duration_seconds'] = message.voice.duration


async def get_document_data(
    message: Message,
    msg_info: dict,
    names: tuple
) -> None:
    if MEDIA_EXPORT['documents'] is True:
        doc_path, thumb_path, doc_relative_path, thumb_relative_path = names
        try:
            await app.download_media(
                message.document.file_id,
                doc_path
            )
            msg_info['file'] = doc_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['file'] = FILE_NOT_FOUND

        if message.document.thumbs is not None:
            try:
                await app.download_media(
                    message.document.thumbs[0].file_id,
                    thumb_path
                )
                msg_info['thumbnail'] = thumb_relative_path
            except ValueError:
                print("Oops can't download media!")
                msg_info['thumbnail'] = FILE_NOT_FOUND
        else:
            msg_info['thumbnail'] = FILE_NOT_FOUND
    else:
        msg_info['file'] = FILE_NOT_FOUND
        msg_info['thumbnail'] = FILE_NOT_FOUND
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

    # TODO: bug. remove entitiy part from all message
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


# TODO: fix List[str]
async def get_contact_data() -> list:
    contacts = await app.get_contacts()
    all_contacts = []
    for c in contacts:
        user = {}
        # TODO: add date and date_unixtime
        if c.first_name is not None:
            user['first_name'] = c.first_name
        else:
            user['first_name'] = ''

        if c.last_name is not None:
            user['last_name'] = c.last_name
        else:
            user['last_name'] = ''

        if c.phone_number is not None:
            user['phone_number'] = c.phone_number
        else:
            user['phone_number'] = ''

        all_contacts.append(user)
    # TODO: convert to vcf
    return all_contacts


def generate_file_name(
    message: Message,
    file_type: str,
    username: str,
    media_num: int = None,
) -> tuple:

    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''

    if file_type == 'photo':
        media_dir = f'{chat_export_name}/photos'
        os.makedirs(media_dir, exist_ok=True)

        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        # TODO: what format for png??
        photo_name = f'file_{media_num}@{date}.jpg'
        photo_path = f'{chat_export_name}/photos/{photo_name}'
        photo_relative_path = f'photos/{photo_name}'

        result = (
            photo_path,
            photo_relative_path,
        )
        return result

    elif file_type == 'video':
        media_dir = f'{chat_export_name}/video_files'
        os.makedirs(media_dir, exist_ok=True)
        if message.video.file_name is not None:
            video_name = message.video.file_name
            video_path = f'{media_dir}/{video_name}'
        else:
            # TODO: set name
            video_name = 'random name'
            video_path = 'random name'

        if message.video.thumbs is not None:
            thumb_name = f'{video_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        video_relative_path = f'video_files/{video_name}'
        thumb_relative_path = f'video_files/{thumb_name}'

        result = (
            video_path,
            thumb_path,
            video_relative_path,
            thumb_relative_path
        )
        return result

    elif file_type == 'voice':
        media_dir = f'{chat_export_name}/voice_messages'
        os.makedirs(media_dir, exist_ok=True)

        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        voice_name = f'audio_{media_num}@{date}.ogg'
        voice_path = f'{media_dir}/{voice_name}'
        voice_relative_path = f'voice_messages/{voice_name}'

        result = voice_path, voice_relative_path
        return result

    elif file_type == 'video_note':
        media_dir = f'{chat_export_name}/round_video_messages'
        os.makedirs(media_dir, exist_ok=True)

        # TODO: by default don't have name
        date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
        vnote_name = f'file_{media_num}@{date}.mp4'
        vnote_path = f'{media_dir}/{vnote_name}'

        if message.video_note.thumbs is not None:
            thumb_name = f'{vnote_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        vnote_relative_path = f'round_video_messages/{vnote_name}'
        thumb_relative_path = f'round_video_messages/{thumb_name}'

        result = (
            vnote_path,
            thumb_path,
            vnote_relative_path,
            thumb_relative_path
        )
        return result

    elif file_type == 'sticker':
        media_dir = f'{chat_export_name}/stickers'
        os.makedirs(media_dir, exist_ok=True)

        if message.sticker.file_name is not None:
            sticker_name = message.sticker.file_name
            sticker_path = f'{media_dir}/{sticker_name}'
        else:
            # TODO: set name
            sticker_name = 'random name'
            sticker_path = 'random name'

        if message.sticker.thumbs is not None:
            thumb_name = f'{sticker_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        sticker_relative_path = f'stickers/{sticker_name}'
        thumb_relative_path = f'stickers/{thumb_name}'

        result = (
            sticker_path,
            thumb_path,
            sticker_relative_path,
            thumb_relative_path
        )
        return result

    elif file_type == 'animation':
        media_dir = f'{chat_export_name}/video_files'
        os.makedirs(media_dir, exist_ok=True)
        if message.animation.file_name is not None:
            animation_name = message.animation.file_name
            animation_path = f'{media_dir}/{animation_name}'
        else:
            # TODO: set name
            animation_name = 'random name'
            animation_path = 'random name'

        if message.animation.thumbs is not None:
            thumb_name = f'{animation_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        animation_relative_path = f'video_files/{animation_name}'
        thumb_relative_path = f'video_files/{thumb_name}'

        result = (
            animation_path,
            thumb_path,
            animation_relative_path,
            thumb_relative_path
        )
        return result

    elif file_type == 'audio':
        media_dir = f'{chat_export_name}/files'
        os.makedirs(media_dir, exist_ok=True)

        if message.audio.file_name is not None:
            audio_name = message.audio.file_name
            audio_path = f'{media_dir}/{audio_name}'
        else:
            # TODO: set name
            audio_name = 'random name'
            audio_path = 'random name'

        if message.audio.thumbs is not None:
            thumb_name = f'{audio_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        audio_relative_path = f'files/{audio_name}'
        thumb_relative_path = f'files/{thumb_name}'

        result = audio_path, thumb_path, audio_relative_path, thumb_relative_path
        return result

    else:
        # document
        media_dir = f'{chat_export_name}/files'
        os.makedirs(media_dir, exist_ok=True)
        if message.document.file_name is not None:
            doc_name = message.document.file_name
            doc_path = f'{media_dir}/{doc_name}'
        else:
            # TODO: set name
            doc_name = 'random name'
            doc_path = 'random name'
            pass

        if message.document.thumbs is not None:
            thumb_name = f'{doc_name}_thumb.jpg'
            thumb_path = f'{media_dir}/{thumb_name}'
        else:
            thumb_name = None
            thumb_path = None

        doc_relative_path = f'files/{doc_name}'
        thumb_relative_path = f'files/{thumb_name}'

        result = doc_path, thumb_path, doc_relative_path, thumb_relative_path
        return result


def convert_to_unixtime(date: datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


app.run(main())
