# transmission-telegram
Переписанная версия основанная на https://github.com/machsix/transmission-telegram
Добавлена возможность закачки в отдельные папки

## Конфигурация
-rootdir = Путь до корневой папки с подпапками (Например: /volume1/Share)
-downloaddirs = Имя подпапок без пробелов через запятую (Например: Movies,Series,Music)
-defdir = Имя подпапки для загрузки по умолчанию (Например: Downloads)

## Поведение
Команда add изменена на прием только одной ссылки на торрент

Подпись при загрузке торрент файла распознается как имя желаемой подпапки

Проверка имени подпапки не учитывает регистр. Если папка не указана или не распознана, то торрент закачивается в defdir

## TODO
* Добавить локализацию
* Добавить настройку позволяющую загружать не в defdir, а в указанную в сообщении
* Добавить настройку позволяющую учитывать регистр при проверки имен подпапок
* ? Wrte me

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
