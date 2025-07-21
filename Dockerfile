FROM python:3.10-slim AS builder

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    libffi-dev \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .

RUN pip install --upgrade pip && pip install --prefix=/install -r requirements.txt


FROM python:3.10-slim AS final

WORKDIR /app

COPY --from=builder /install /usr/local
COPY . .

CMD ["python", "bot.py"]
