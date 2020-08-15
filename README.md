# tobym

## Description

Simple telegram bot for personal usage.

## Build & Run

```
BOT_TOKEN=xxxx docker-compose up --build
```

## Delete WebHook

If bot wasn't shutdown gracefully:

```
curl -X POST -H 'Content-Type: application/json' "https://api.telegram.org/bot${BOT_TOKEN}/deleteWebhook"
```