import logging
import os
import json
import sys
import time
from datetime import datetime
import uuid

from tqdm_loggable.auto import tqdm
from tqdm_loggable.tqdm_logging import tqdm_logging
from pyrogram import Client
from pyrogram.types import Message
from pyrogram.enums import ChatType, MessageEntityType
from configs import API_ID, API_HASH, MEDIA_EXPORT, CHAT_IDS, FILE_NOT_FOUND, JSON_FILE_PAGE_SIZE, DOWNLOAD_PATH
from chats import ChatExporter

logger = logging.getLogger(__name__)


class Archive:
    def __init__(self, chat_ids=None) -> None:
        if chat_ids is None:
            chat_ids = []
        self.chat_ids = chat_ids
        self.voice_num = 0
        self.photo_num = 0
        self.contact_num = 0
        self.video_message_num = 0
        self.username = ''
        self.messages = []
        self.chat_data = {}

    def fill_chat_data(self, chat: ChatExporter) -> None:
        self.username = chat.username
        match chat.type:
            case ChatType.PRIVATE:
                # TODO: lastname
                self.chat_data['name'] = chat.first_name
                self.chat_data['type'] = 'personal_chat'
                self.chat_data['id'] = chat.id
            case ChatType.CHANNEL:
                self.chat_data['name'] = chat.title
                self.chat_data['type'] = 'public_channel'
                # when using telegram api ids have -100 prefix
                # https://stackoverflow.com/questions/33858927/how-to-obtain-the-chat-id-of-a-private-telegram-channel
                self.chat_data['id'] = str(chat.id)[4::] if str(chat.id).startswith('-100') else chat.id
            case ChatType.GROUP:
                self.chat_data['name'] = chat.title
                self.chat_data['type'] = 'public_group'
                self.chat_data['id'] = str(chat.id)[4::] if str(chat.id).startswith('-100') else chat.id
            case ChatType.SUPERGROUP:
                self.chat_data['name'] = chat.title
                self.chat_data['type'] = 'public_supergroup'
                self.chat_data['id'] = str(chat.id)[4::] if str(chat.id).startswith('-100') else chat.id
            # TODO: private SUPERGROUP and GROUP
            case _:
                # bot? other chat types
                pass

    async def process_message(self, chat, message, msg_info: dict, pbar: tqdm) -> None:
        # TODO: move msg_info filling to other function
        msg_info['id'] = message.id
        msg_info['type'] = 'message'
        msg_info['date'] = message.date.strftime('%Y-%m-%dT%H:%M:%S')
        msg_info['date_unixtime'] = convert_to_unixtime(message.date)

        # set chat name
        # this part is so shitty fix THIS
        if chat.type == ChatType.PRIVATE:
            full_name = ''
            if message.from_user.first_name is not None:
                full_name += message.from_user.first_name
            if message.from_user.last_name is not None:
                full_name += f' {message.from_user.last_name}'
            msg_info['from'] = full_name
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
                # TODO: use from user...

        if message.reply_to_message_id is not None:
            msg_info['reply_to_message_id'] = message.reply_to_message_id

        if message.forward_from_chat is not None:
            msg_info['forwarded_from'] = message.forward_from_chat.title
        elif message.forward_from is not None:
            msg_info['forwarded_from'] = message.forward_from.first_name

        # TODO: type service. actor...

        if message.sticker is not None:
            if MEDIA_EXPORT['stickers'] is True:
                names = get_sticker_name(
                    message,
                    self.username
                )
                await get_sticker_data(message, msg_info, names, pbar)
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
                msg_info['media_type'] = 'sticker'
                msg_info['sticker_emoji'] = message.sticker.emoji
                msg_info['width'] = message.sticker.width
                msg_info['height'] = message.sticker.height
        elif message.animation is not None:
            if MEDIA_EXPORT['animations'] is True:
                names = get_animation_name(message, self.username)
                await get_animation_data(message, msg_info, names, pbar)
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
                msg_info['media_type'] = 'animation'
                msg_info['mime_type'] = message.animation.mime_type
                msg_info['width'] = message.animation.width
        elif message.photo is not None:
            if MEDIA_EXPORT['photos'] is True:
                self.photo_num += 1
                names = get_photo_name(
                    message,
                    self.username,
                    self.photo_num
                )
                await get_photo_data(message, msg_info, names, pbar)
            else:
                msg_info['photo'] = FILE_NOT_FOUND
                msg_info['width'] = message.photo.width
                msg_info['height'] = message.photo.height
        elif message.video is not None:
            if MEDIA_EXPORT['videos'] is True:
                names = get_video_name(message, self.username)
                await get_video_data(message, msg_info, names, pbar)
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
                msg_info['media_type'] = 'video_file'
                msg_info['mime_type'] = message.video.mime_type
                msg_info['duration_seconds'] = message.video.duration
                msg_info['width'] = message.video.width
        elif message.video_note is not None:
            if MEDIA_EXPORT['video_messages'] is True:
                self.video_message_num += 1
                names = get_video_name(
                    message,
                    self.username,
                    self.video_message_num
                )
                await get_video_note_data(
                    message,
                    msg_info,
                    names,
                    pbar
                )
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
                msg_info['media_type'] = 'video_message'
                msg_info['mime_type'] = message.video_note.mime_type
                msg_info['duration_seconds'] = message.video_note.duration
        elif message.audio is not None:
            if MEDIA_EXPORT['audios'] is True:
                names = get_audio_name(message, self.username)
                await get_audio_data(message, msg_info, names, pbar)
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
                msg_info['media_type'] = 'audio_file'
                msg_info['performer'] = message.audio.performer
                msg_info['title'] = message.audio.title
                msg_info['mime_type'] = message.audio.mime_type
        elif message.voice is not None:
            if MEDIA_EXPORT['voice_messages'] is True:
                self.voice_num += 1
                names = get_voice_name(
                    message,
                    self.username,
                    self.voice_num
                )
                await get_voice_data(
                    message,
                    msg_info,
                    names,
                    pbar
                )
            else:
                msg_info['file'] = FILE_NOT_FOUND
        elif message.document is not None:
            if MEDIA_EXPORT['documents'] is True:
                names = get_document_name(message, self.username)
                await get_document_data(
                    message,
                    msg_info,
                    names,
                    pbar
                )
            else:
                msg_info['file'] = FILE_NOT_FOUND
                msg_info['thumbnail'] = FILE_NOT_FOUND
        elif message.contact is not None:
            self.contact_num += 1
            names = get_contact_name(
                self.username,
                self.contact_num
            )
            get_contact_data(
                message,
                msg_info,
                names,
                pbar
            )
        elif message.location is not None:
            msg_info['location_information'] = {
                'latitude': message.location.latitude,
                'longitude': message.location.longitude
            }

        if message.text is not None:
            text = get_text_data(message, 'text')
            if text:
                text.append(message.text)
                msg_info['text'] = text
            else:
                msg_info['text'] = message.text
        elif message.caption is not None:
            caption = get_text_data(message, 'caption')
            if caption:
                caption.append(message.caption)
                msg_info['caption'] = caption
            else:
                msg_info['caption'] = message.caption
        else:
            msg_info['text'] = ''
        self.messages.append(msg_info)
        self.chat_data['messages'] = self.messages


def generate_json_name(username: str, path: str = '') -> str:
    # TODO: add path when user want other path
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    json_name = f'{chat_export_name}/result.json'
    os.makedirs(chat_export_name, exist_ok=True)
    return json_name


async def get_sticker_data(
        message: Message,
        msg_info: dict,
        names: tuple,
        pbar: tqdm
) -> None:
    sticker_path, thumb_path, sticker_relative_path, thumb_relative_path = names
    file_name = os.path.basename(sticker_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.sticker.file_id, sticker_path)
        msg_info['file'] = sticker_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.sticker.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.sticker.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
        msg_info['thumbnail'] = FILE_NOT_FOUND

    msg_info['media_type'] = 'sticker'
    msg_info['sticker_emoji'] = message.sticker.emoji
    msg_info['width'] = message.sticker.width
    msg_info['height'] = message.sticker.height

async def get_animation_data(
        message: Message,
        msg_info: dict,
        names: tuple,
        pbar: tqdm
) -> None:
    animation_path, thumb_path, animation_relative_path, thumb_relative_path = names
    file_name = os.path.basename(animation_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.animation.file_id, animation_path)
        msg_info['file'] = animation_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.animation.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.animation.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
        msg_info['thumbnail'] = FILE_NOT_FOUND

    msg_info['media_type'] = 'animation'
    msg_info['mime_type'] = message.animation.mime_type
    msg_info['width'] = message.animation.width
    msg_info['height'] = message.animation.height

async def get_photo_data(
        message: Message,
        msg_info: dict,
        names: tuple,
        pbar: tqdm
):
    photo_path, photo_relative_path = names
    file_name = os.path.basename(photo_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.photo.file_id, photo_path)
        msg_info['photo'] = photo_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['photo'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    msg_info['width'] = message.photo.width
    msg_info['height'] = message.photo.height


async def get_video_data(
        message: Message,
        msg_info: dict,
        names: tuple,
        pbar: tqdm
) -> None:
    video_path, thumb_path, video_relative_path, thumb_relative_path = names
    file_name = os.path.basename(video_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.video.file_id, video_path)
        msg_info['file'] = video_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.video.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.video.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
        msg_info['thumbnail'] = FILE_NOT_FOUND

    msg_info['media_type'] = 'video_file'
    msg_info['mime_type'] = message.video.mime_type
    msg_info['duration_seconds'] = message.video.duration
    msg_info['width'] = message.video.width
    msg_info['height'] = message.video.height


async def get_video_note_data(
    message: Message,
    msg_info: dict,
    names: tuple,
    pbar: tqdm
):
    vnote_path, thumb_path, vnote_relative_path, thumb_relative_path = names
    file_name = os.path.basename(vnote_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.video_note.file_id, vnote_path)
        msg_info['file'] = vnote_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.video_note.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.video_note.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
        msg_info['thumbnail'] = FILE_NOT_FOUND

    msg_info['media_type'] = 'video_message'
    msg_info['mime_type'] = message.video_note.mime_type
    msg_info['duration_seconds'] = message.video_note.duration

async def get_audio_data(
    message: Message,
    msg_info: dict,
    names: tuple,
    pbar: tqdm
) -> None:
    audio_path, thumb_path, audio_relative_path, thumb_relative_path = names
    file_name = os.path.basename(audio_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.audio.file_id, audio_path)
        msg_info['file'] = audio_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.audio.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.audio.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
        msg_info['thumbnail'] = FILE_NOT_FOUND

    msg_info['media_type'] = 'audio_file'
    msg_info['performer'] = message.audio.performer
    msg_info['title'] = message.audio.title
    msg_info['mime_type'] = message.audio.mime_type
    msg_info['duration_seconds'] = message.audio.duration


async def get_voice_data(
    message: Message,
    msg_info: dict,
    names: tuple,
    pbar: tqdm
) -> None:
    voice_path, voice_relative_path = names
    file_name = os.path.basename(voice_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.voice.file_id, voice_path)
        msg_info['file'] = voice_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    msg_info['media_type'] = 'voice_message'
    msg_info['mime_type'] = message.voice.mime_type
    msg_info['duration_seconds'] = message.voice.duration

async def get_document_data(
        message: Message,
        msg_info: dict,
        names: tuple,
        pbar: tqdm
) -> None:
    doc_path, thumb_path, doc_relative_path, thumb_relative_path = names
    file_name = os.path.basename(doc_path)
    pbar.set_postfix(file=file_name)
    try:
        await app.download_media(message.document.file_id, doc_path)
        msg_info['file'] = doc_relative_path
    except ValueError:
        print("Oops can't download media!")
        msg_info['file'] = FILE_NOT_FOUND
    finally:
        pbar.set_postfix(file=None)

    if message.document.thumbs is not None:
        thumb_file_name = os.path.basename(thumb_path)
        pbar.set_postfix(file=thumb_file_name)
        try:
            await app.download_media(message.document.thumbs[0].file_id, thumb_path)
            msg_info['thumbnail'] = thumb_relative_path
        except ValueError:
            print("Oops can't download media!")
            msg_info['thumbnail'] = FILE_NOT_FOUND
        finally:
            pbar.set_postfix(file=None)
    else:
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

    for e in entities:
        txt = {}
        match e.type:
            case MessageEntityType.URL:
                txt['type'] = 'link'
            case MessageEntityType.HASHTAG:
                txt['type'] = 'hashtag'
            case MessageEntityType.CASHTAG:
                txt['type'] = 'cashtag'
            case MessageEntityType.BOT_COMMAND:
                txt['type'] = 'bot_command'
            case MessageEntityType.MENTION:
                txt['type'] = 'mention'
            case MessageEntityType.EMAIL:
                txt['type'] = 'email'
            case MessageEntityType.PHONE_NUMBER:
                txt['type'] = 'phone_number'
            case MessageEntityType.BOLD:
                txt['type'] = 'bold'
            case MessageEntityType.ITALIC:
                txt['type'] = 'italic'
            case MessageEntityType.UNDERLINE:
                txt['type'] = 'underline'
            case MessageEntityType.STRIKETHROUGH:
                txt['type'] = 'strikethrough'
            case MessageEntityType.SPOILER:
                txt['type'] = 'spoiler'
            case MessageEntityType.CODE:
                txt['type'] = 'code'
            case MessageEntityType.PRE:
                txt['type'] = 'pre'
                txt['language'] = ''
            case MessageEntityType.BLOCKQUOTE:
                txt['type'] = 'blockquote'
            case MessageEntityType.TEXT_LINK:
                txt['type'] = 'text_link'
                txt['href'] = e.url
            case MessageEntityType.TEXT_MENTION:
                txt['type'] = 'text_mention'
            case MessageEntityType.BANK_CARD:
                txt['type'] = 'bank_card'
            case MessageEntityType.CUSTOM_EMOJI:
                txt['type'] = 'custom_emoji'
            case _:
                txt['type'] = 'unknown'

        if text_mode == 'text':
            txt['text'] = message.text[e.offset:e.offset + e.length]
        else:
            txt['text'] = message.caption[e.offset:e.offset + e.length]
        text.append(txt)
    return text


# TODO: fix better typing
def get_contact_data(
        message: Message,
        msg_info: dict,
        names: tuple
) -> list:
    contact_data = {'phone_number': message.contact.phone_number}
    contact_data['fist_name'] = message.contact.first_name if message.contact.first_name is not None else ''
    contact_data['last_name'] = message.contact.last_name if message.contact.last_name is not None else ''
    msg_info['contact_information'] = contact_data

    if MEDIA_EXPORT['contacts'] is True:
        vcard_path, vcard_relative_path = names
        msg_info['contact_vcard'] = vcard_relative_path

        # convert to vcard
        vcard = (
            'BEGIN:VCARD\n'
            'VERSION:3.0\n'
            f'FN;CHARSET=UTF-8:{message.contact.first_name} {message.contact.last_name}\n'
            f'N;CHARSET=UTF-8:{message.contact.last_name};{message.contact.first_name};;;\n'
            f'TEL;TYPE=CELL:{message.contact.phone_number}\n'
            'END:VCARD\n'
        )
        with open(vcard_path, 'w') as f:
            f.write(vcard)
    else:
        msg_info['contact_vcard'] = FILE_NOT_FOUND


def get_photo_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
    media_dir = f'{chat_export_name}/photos'
    os.makedirs(media_dir, exist_ok=True)

    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
    # TODO: what format for png??
    photo_name = f'file_{media_num}@{date}.jpg'
    photo_path = f'{chat_export_name}/photos/{photo_name}'
    photo_relative_path = f'photos/{photo_name}'

    result = photo_path, photo_relative_path
    return result


def get_video_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
    media_dir = f'{chat_export_name}/video_files'
    os.makedirs(media_dir, exist_ok=True)

    # TODO: gen better way random str
    video_name = str(uuid.uuid4()) if message.video.file_name is None else message.video.file_name
    video_path = f'{media_dir}/{video_name}'

    if message.video.thumbs is not None:
        thumb_name = f'{video_name}_thumb.jpg'
        thumb_path = f'{media_dir}/{thumb_name}'
    else:
        thumb_name = None
        thumb_path = None

    video_relative_path = f'video_files/{video_name}'
    thumb_relative_path = None if thumb_name is None else f'video_files/{thumb_name}'

    result = (
        video_path,
        thumb_path,
        video_relative_path,
        thumb_relative_path
    )
    return result


def get_voice_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''

    media_dir = f'{chat_export_name}/voice_messages'
    os.makedirs(media_dir, exist_ok=True)

    date = message.date.strftime('%d-%m-%Y_%H-%M-%S')
    voice_name = f'audio_{media_num}@{date}.ogg'
    voice_path = f'{media_dir}/{voice_name}'
    voice_relative_path = f'voice_messages/{voice_name}'

    result = voice_path, voice_relative_path
    return result


def get_video_note_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
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
    thumb_relative_path = None if thumb_name is None else f'round_video_messages/{thumb_name}'

    result = (
        vnote_path,
        thumb_path,
        vnote_relative_path,
        thumb_relative_path
    )
    return result


def get_sticker_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
    media_dir = f'{chat_export_name}/stickers'
    os.makedirs(media_dir, exist_ok=True)
    # TODO: gen better way random str
    sticker_name = str(uuid.uuid4()) if message.sticker.file_name is None else message.sticker.file_name
    sticker_path = f'{media_dir}/{sticker_name}'

    if message.sticker.thumbs is not None:
        thumb_name = f'{sticker_name}_thumb.jpg'
        thumb_path = f'{media_dir}/{thumb_name}'
    else:
        thumb_name = None
        thumb_path = None

    sticker_relative_path = f'stickers/{sticker_name}'
    thumb_relative_path = None if thumb_name is None else f'stickers/{thumb_name}'

    result = (
        sticker_path,
        thumb_path,
        sticker_relative_path,
        thumb_relative_path
    )
    return result


def get_animation_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
    media_dir = f'{chat_export_name}/video_files'
    os.makedirs(media_dir, exist_ok=True)

    # TODO: gen better way random str
    animation_name = str(uuid.uuid4()) if message.animation.file_name is None else message.animation.file_name
    animation_path = f'{media_dir}/{animation_name}'

    if message.animation.thumbs is not None:
        thumb_name = f'{animation_name}_thumb.jpg'
        thumb_path = f'{media_dir}/{thumb_name}'
    else:
        thumb_name = None
        thumb_path = None

    animation_relative_path = f'video_files/{animation_name}'
    thumb_relative_path = None if thumb_name is None else f'video_files/{thumb_name}'
    result = (
        animation_path,
        thumb_path,
        animation_relative_path,
        thumb_relative_path
    )
    return result


def get_audio_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''

    media_dir = f'{chat_export_name}/files'
    os.makedirs(media_dir, exist_ok=True)

    # TODO: gen better way random str
    audio_name = str(uuid.uuid4()) if message.audio.file_name is None else message.audio.file_name
    audio_path = f'{media_dir}/{audio_name}'

    if message.audio.thumbs is not None:
        thumb_name = f'{audio_name}_thumb.jpg'
        thumb_path = f'{media_dir}/{thumb_name}'
    else:
        thumb_name = None
        thumb_path = None

    audio_relative_path = f'files/{audio_name}'
    thumb_relative_path = None if thumb_name is None else f'files/{thumb_name}'

    result = audio_path, thumb_path, audio_relative_path, thumb_relative_path
    return result


def get_document_name(
        message: Message,
        username: str,
        media_num: int = None
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''

    media_dir = f'{chat_export_name}/files'
    os.makedirs(media_dir, exist_ok=True)

    # TODO: gen better way random str
    doc_name = str(uuid.uuid4()) if message.document.file_name is None else message.document.file_name
    doc_path = f'{media_dir}/{doc_name}'

    if message.document.thumbs is not None:
        thumb_name = f'{doc_name}_thumb.jpg'
        thumb_path = f'{media_dir}/{thumb_name}'
    else:
        thumb_name = None
        thumb_path = None

    doc_relative_path = f'files/{doc_name}'
    thumb_relative_path = None if thumb_name is None else f'files/{thumb_name}'

    result = doc_path, thumb_path, doc_relative_path, thumb_relative_path
    return result


def get_contact_name(
        username: str,
        media_num: int,
) -> tuple:
    chat_export_date = datetime.now().strftime("%Y-%m-%d")
    chat_export_name = f'{DOWNLOAD_PATH}/ChatExport_{username}_{chat_export_date}'
    # TODO: when user want other path
    path = ''
    media_dir = f'{chat_export_name}/contacts'
    os.makedirs(media_dir, exist_ok=True)
    contact_name = f'contact_{media_num}.vcf'
    contact_path = f'{media_dir}/{contact_name}'
    contact_relative_path = f'contacts/{contact_name}'
    return contact_path, contact_relative_path


def convert_to_unixtime(date: datetime):
    # telegram date format: "2022-07-10 08:49:23"
    unix_time = int(time.mktime(date.timetuple()))
    return unix_time


def to_html():
    pass


def split_json_file(data: dict, output_path: str, page_size: int = JSON_FILE_PAGE_SIZE):
    messages = data["messages"]
    page = 1
    current_data = {"messages": []}

    for msg in messages:
        current_data["messages"].append(msg)
        serialized_data = json.dumps(current_data, default=str)
        if len(serialized_data.encode('utf-8')) > page_size:
            # Remove the last message that made it too big
            current_data["messages"].pop()
            # Save current part
            with open(output_path.replace("result.json", f"result_part{page}.json"), "w") as outf:
                json.dump(current_data, outf, indent=4, default=str)
            # Start new part with the removed message
            page += 1
            current_data = {"messages": [msg]}

    # Save last part
    with open(output_path.replace("result.json", f"result_part{page}.json"), "w") as outf:
        json.dump(current_data, outf, indent=4, default=str)


async def main():
    fmt = f"%(message)s"
    logging.basicConfig(level=logging.INFO, format=fmt)

    # Mute other libraries (like Telethon, HTTPX, etc.)
    logging.getLogger("telethon").setLevel(logging.WARNING)
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("pyrogram").setLevel(logging.WARNING)

    # Set the rate how often we update logs
    # Defaults to 10 seconds - optional
    # tqdm_logging.set_log_rate(datetime.timedelta(seconds=1))

    async with app:
        print("\033[32mStarting...\033[0m")

        if not CHAT_IDS:
            all_dialogs_id = await ChatExporter(app).get_ids()
            CHAT_IDS.extend(all_dialogs_id)

        archive = Archive(CHAT_IDS)

        for cid in CHAT_IDS:
            # when use telegram api, channels id have -100 prefix
            if type(cid) == int and not str(cid).startswith('-100'):
                new_id = int(f'-100{cid}')
                CHAT_IDS[CHAT_IDS.index(cid)] = new_id

            chat = await app.get_chat(cid)
            archive.fill_chat_data(chat)

            # TODO: add exception for private channels
            # read messages from first to last
            all_messages = [m async for m in app.get_chat_history(cid)]
            all_messages.reverse()
            # Wrap the message processing with tqdm to show progress
            pbar = tqdm(all_messages, desc=f"Processing messages for @{chat.username or chat.title or chat.id}", unit="message")
            for message in pbar:
                # TODO: add initial message of channel
                msg_info = {}
                await archive.process_message(chat, message, msg_info, pbar)

            json_name = generate_json_name(username=archive.username)
            if not JSON_FILE_PAGE_SIZE:
                with open(json_name, mode='w') as f:
                    json.dump(archive.chat_data, f, indent=4, default=str)
            else:
                split_json_file(archive.chat_data, json_name, JSON_FILE_PAGE_SIZE)


app = Client(
    "my_bot",
    api_id=API_ID,
    api_hash=API_HASH,
)

app.run(main())
