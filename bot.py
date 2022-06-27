from pyrogram import Client
from config import APP_ID, API_HASH

app = Client(
    "my_bot",
    api_id=APP_ID,
    api_hash=API_HASH,
)

async def main():
    async with app:
        await app.send_message('me', 'ping!')
        async for message in app.get_chat_history('mo_ein'):
            print(message)
app.run(main())
