from pyrogram import Client
from pyrogram.enums import ChatType


class Private:
    def __init__(self, app: Client):
        self.app = app

    # TODO: and bots?? 
    async def get_privates_list(self) -> list:
        privates_id = []
        async for dialog in self.app.get_dialogs():
            if dialog.chat.type == ChatType.PRIVATE:
                privates_id.append(dialog.chat.id)
        return privates_id

