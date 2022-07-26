from pyrogram import Client
from pyrogram.enums import ChatType


class Channel:
    def __init__(self, app: Client):
        self.app = app

    async def get_channels_list(self, private = True) -> list:
        channels_id = []
        async for dialog in self.app.get_dialogs():
            # TODO: private
            if dialog.chat.type == ChatType.CHANNEL:
                channels_id.append(dialog.chat.id)
        return channels_id

