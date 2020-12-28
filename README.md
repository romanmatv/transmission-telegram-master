# transmission-telegram

##  Docker Alternate Installation Route

### docker-compose Example

```
version: '3'
services:
  telegram-transmission-bot:
    container_name: telegram-transmission-bot
    restart: on-failure
    image: machsix/transmission-telegram:latest
    environment:
        - TELEGRAM_TRANSMISSION_BOT=378883659:foobar
        - TELEGRAM_USERNAME=foobar
        - TRANSMISSION_URL=http://localhost:9091/transmission/rpc
        - USERNAME=foo
        - PASS=bar
        - DOWNLOAD_DIR=/var/lib/transmission-daemon/Downloads
    entrypoint: bash
    command: ['-c', '/transmission-telegram -token=$${TELEGRAM_TRANSMISSION_BOT} -master=$${TELEGRAM_USERNAME} -url=$${TRANSMISSION_URL} -username=$${USERNAME} -password=$${PASS} -private -dir=$${DOWNLOAD_DIR}']
```
