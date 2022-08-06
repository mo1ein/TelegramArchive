from pyrogram import Client
from pyrogram.enums import ChatType
from config import CHAT_EXPORT


class Chat:
    def __init__(self, app: Client):
        self.app = app

    async def get_ids(self, private = True) -> list:
        dialog_ids = []
        async for dialog in self.app.get_dialogs():
            # TODO: private channel
            # but this is for all channels
            if CHAT_EXPORT['public_channels'] is True: 
                if dialog.chat.type == ChatType.CHANNEL:
                    dialog_ids.append(dialog.chat.id)
        
            # TODO: but this is for all groups
            # TODO: for SUPERGROUP?
            # TODO: private groups
            if CHAT_EXPORT['public_groups'] is True:
                if dialog.chat.type == ChatType.GROUP:
                    dialog_ids.append(dialog.chat.id)

            if CHAT_EXPORT['personal_chats'] is True: 
                if dialog.chat.type == ChatType.PRIVATE:
                    dialog_ids.append(dialog.chat.id)
            
            if CHAT_EXPORT['bot_chats'] is True: 
                if dialog.chat.type == ChatType.BOT:
                    dialog_ids.append(dialog.chat.id)

        return dialog_ids

