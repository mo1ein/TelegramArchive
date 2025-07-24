from pyrogram import Client
from pyrogram.enums import ChatType
from configs import CHAT_EXPORT

class ChatExporter:
	"""
	Export chat IDs from a Pyrogram Client based on configured chat-type flags.
	"""

	def __init__(self, app: Client) -> None:
		self.app: Client = app
		# Map each ChatType to its corresponding export-flag in the config
		self._export_map: dict[ChatType, bool] = {
			ChatType.CHANNEL: CHAT_EXPORT.get("channels", False),
			ChatType.SUPERGROUP: CHAT_EXPORT.get("super_group", False),
			ChatType.GROUP: CHAT_EXPORT.get("group", False),
			ChatType.PRIVATE: CHAT_EXPORT.get("personal", False),
			ChatType.BOT: CHAT_EXPORT.get("bot", False),
		}

	async def get_ids(self) -> list[int]:
		"""
		Asynchronously iterate through all dialogs and return a list of
		chat IDs whose ChatType is enabled in CHAT_EXPORT.
		"""
		dialog_ids: list[int] = []

		async for dialog in self.app.get_dialogs():
			chat = dialog.chat
			should_export: bool = self._export_map.get(chat.type, False)
			print(
				f"Dialog: id={chat.id}, "
				f"title={getattr(chat, "title", None) or getattr(chat, "first_name", None)}, "
				f"type={chat.type.name}, export={should_export}"
			)
			if should_export:
				dialog_ids.append(chat.id)

		return dialog_ids
