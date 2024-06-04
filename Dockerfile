FROM python:3.10
RUN mkdir /app
WORKDIR /app
COPY requirements.txt .
RUN pip3 install --no-cache-dir --upgrade -r ./requirements.txt
COPY bot.py .
COPY chats.py .
COPY config.py .
CMD ["python", "bot.py"]
