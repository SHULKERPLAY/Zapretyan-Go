# ![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/core/winres/ico32.png) Zapretyan-Go \\=> Discord Sender 1.0.0 (Официальный плагин)

## Назначение
Повторяет назначение оригинальной SHULKERPLAY/Zapretyan, переосмысляя подход к выводу списков в сообщениях. В отличие от оригинала, отсылает каждое новое изменение сразу при их наличии, а не раз в день.

Отсылает детальные сообщения о новых банах/разбанах доменов/IP адрес, а также ежедневную статистику в настроенные чаты Discord с помощью вашего бота!

## Системные требования
- Версия ядра Zapretyan-Go не ниже `v2.1.0.0` с поддерживаемой версией JSON протокола `1`
- От 10МБ ОЗУ при работе (+~13МБ на каждые ~100000 записей диффа в одном событии)
- Плагин `Daily Statistics` версии не ниже `v1.0.1`

## Установка
- Исполняемый файл плагина рекомендуется помещать в папку с расширениями ядра: `./extensions`
- Добавьте конфигурацию плагина в ядро:

```toml
[[extension]]
name = "Discord Sender"
desc = "Send new changes as embeds using your Discord-Bot"
source = "https://github.com/SHULKERPLAY/Zapretyan-Go"
path = "./extensions/discord-sender"
enabled = false
BOT_TOKEN = ""

[extension.sender]
isban = true
isunban = true
isbanip = true
isunbanip = true
istotal = true

[extension.data]
mmdb_update = true
mmdb_lang = "ru"
mmdbasn_path = "./discord-sender/dbip-asn.mmdb"
mmdbcountry_path = "./discord-sender/dbip-country.mmdb"
total_json_path = "./statistics/latest.json"

[extension.channels]
bancid = ""
unbancid = ""
banipcid = ""
unbanipcid = ""
totalcid = ""

[extension.embed]
iconurl = "https://lunarcreators.ru/wp-content/uploads/2025/11/discordiconmini.webp"
embed_author_name = "Запретян-Go <3"
embed_author_url = "https://discord.com/discovery/applications/907372459144147035"
# Embed colors (HEX e.g. 0fafff)
banclr = "#ff5e5e"
unbanclr = "#5e87ff"
banipclr = "#ffa45e"
unbanipclr = "#5effac"
totalclr = "#ffff7d"

[extension.locale] # Override output messages text
banned = "Добавлено записей в реестр"
cat_casino = "Зеркала казино и букмекеры"
cat_film = "Пиратские кино и сериалы"
domains = "Домены"
embed_footer = "🩵 С любовью, @shulkerplay"
ips = "IP Адреса"
newbanned = "В реестр Роскомнадзора добавлены"
newunbanned = "Из реестра Роскомнадзора удалены"
stats_date = "Статистика за" # ...26/06/2026
today_banned = "Сегодня заблокировано"
today_unbanned = "Сегодня разблокировано"
top_country = "Топ стран"
top_isp = "Топ провайдеров"
total_banned = "Всего заблокировано"
unbanned = "Удалено записей из реестра"
```

## Конфигурация
- `BOT_TOKEN` (Обязательное поле (`"T0kEn"`))
  - Токен вашего Discord бота с [портала разработчика](https://discord.com/developers/applications)
  
### `[extension.sender]` - Раздел с переключателями каналов отправки
- `isban` (По умолчанию: `true`)
  - Включает отправку новых заблокированых доменов в `bancid`

![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/extensions/discord-sender/ban.webp) 

- `isunban` (По умолчанию: `true`)
  - Включает отправку новых разблокированых доменов в `unbancid`

![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/extensions/discord-sender/unban.webp) 

- `isbanip` (По умолчанию: `true`)
  - Включает отправку новых заблокированых IP адресов в `banipcid`

![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/extensions/discord-sender/banip.webp) 

- `isunbanip` (По умолчанию: `true`)
  - Включает отправку новых разблокированых IP адресов в `unbanipcid`

![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/extensions/discord-sender/unbanip.webp) 

- `istotal` (По умолчанию: `true`)
  - Включает отправку ежедневной статистики в `totalcid`. Требует валидный путь к JSON, создаваемый плагином [`Daily Statistics`](https://github.com/SHULKERPLAY/Zapretyan-Go/tree/main/extensions/dailystat)

![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/extensions/discord-sender/total.webp) 

### `[extension.data]` - Раздел с параметрами источников данных
- `mmdb_update` (По умолчанию: `true`)
  - Проверяет наличие и дату обновления файлов баз данных GeoLite2. На основе этих баз данных работает отправка `isbanip` и `isunbanip`. Если при открытии GeoLite сервиса произойдёт ошибка (По ошибке чтения или отсутствию базы), эти рассылки будут отключены. Если база данных отсутствует или обновлялась более 45 дней назад, они будут обновлены автоматически с сервиса `db-ip.com`. Отключать этот параметр стоит только если у вас уже есть другой софт, обновляющий базы GeoLite, или если вы вовсе не собираетесь рассылать изменения в IP адресах. В этом случае укажите полный путь до собственных баз и отключите автообновления.
- `mmdb_lang` (По умолчанию: `"ru"`)
  - Язык для обработки запроса внутри базы GeoLite2. От него зависит язык названий стран в выводе сообщений. Базы `db-ip.com` поддерживают языки: `"de"`, `"en"`, `"es"`, `"fa"`, `"fr"`, `"ja"`, `"ko"`, `"pt-BR"`, `"ru"`, `"zh-CN"`. Но если у вас более дорогая база данных, вы можете указать любой языковой код, который зашит в базу данных которую используете вы.
- `mmdbasn_path` (По умолчанию: `"./discord-sender/dbip-asn.mmdb"`)
  - Относительный или полный путь к файлу БД GeoLite ASN. Он будет сохранён по этому пути если файл отсутствует и включён параметр `mmdb_update`.
- `mmdbcountry_path` (По умолчанию: `"./discord-sender/dbip-country.mmdb"`)
  - Относительный или полный путь к файлу БД GeoLite Country. Он будет сохранён по этому пути если файл отсутствует и включён параметр `mmdb_update`.
- `total_json_path` (По умолчанию: `"./statistics/latest.json"`)
  - Относительный или полный путь к файлу с JSON статистикой текущего дня плагина [`Daily Statistics`](https://github.com/SHULKERPLAY/Zapretyan-Go/tree/main/extensions/dailystat). Плагин создаст файл-маркер во время последней отправки ежедневной статистики. Если JSON файл будет новее чем файл-маркер, то новая статистика будет отослана в `totalcid`, а маркер - обновлён. Путь по умолчанию содержит относительный путь для плагина [`Daily Statistics`](https://github.com/SHULKERPLAY/Zapretyan-Go/tree/main/extensions/dailystat) актуальный для версии `v1.0.1`

### `[extension.channels]` - Раздел с ID каналов для отправки
- `bancid` (Обязательное поле `SnowflakeID`)
  - ID канала для отправки заблокированых доменов при включенном `bancid`
- `unbancid` (Обязательное поле `SnowflakeID`)
  - ID канала для отправки разблокированых доменов при включенном `unbancid`
- `banipcid` (Обязательное поле `SnowflakeID`)
  - ID канала для отправки заблокированых IP адресов при включенном `banipcid`
- `unbanipcid` (Обязательное поле `SnowflakeID`)
  - ID канала для отправки разблокированых IP адресов при включенном `unbanipcid`
- `totalcid` (Обязательное поле `SnowflakeID`)
  - ID канала для отправки ежедневной статистики при включенном `totalcid`.

### `[extension.embed]` - Раздел с ID каналов для отправки
- `iconurl` (По умолчанию: `"https://lunarcreators.ru/wp-content/uploads/2025/11/discordiconmini.webp"`)
  - URL до иконки, которая будет отбражаться возле поля автора в Embed
- `embed_author_name` (По умолчанию: `"Запретян-Go <3"`)
  - Имя, которое будет отбражаться в поле автора в Embed
- `iconurl` (По умолчанию: `"https://discord.com/discovery/applications/907372459144147035"`)
  - URL на который будет переходить пользователь, нажав на имя автора в Embed
- `banclr` (По умолчанию: `"#ff5e5e"`)
  - Цвет полосы Embed сообщения при выводе заблокированых доменов
- `unbanclr` (По умолчанию: `"#5e87ff"`)
  - Цвет полосы Embed сообщения при выводе разблокированых доменов
- `banipclr` (По умолчанию: `"#ffa45e"`)
  - Цвет полосы Embed сообщения при выводе заблокированых IP адресов
- `unbanipclr` (По умолчанию: `"#5effac"`)
  - Цвет полосы Embed сообщения при выводе разблокированых IP адресов
- `totalclr` (По умолчанию: `"#ffff7d"`)
  - Цвет полосы Embed сообщения при выводе ежедневной статистики

### `[extension.locale]` - Раздел для изменения выводов в сообщениях

> [!TIP]
> Если вы не хотите менять вывод, вы можете удалить весь раздел, или оставить только `embed_footer`, или оставить всё как есть

- `banned` (По умолчанию: `"Добавлено записей в реестр"`)
- `cat_casino` (По умолчанию: `"Зеркала казино и букмекеры"`)
- `cat_film` (По умолчанию: `"Пиратские кино и сериалы"`)
- `domains` (По умолчанию: `"Домены"`)
- `embed_footer` (По умолчанию: `"🩵 С любовью, @shulkerplay"`)
- `ips` (По умолчанию: `"IP Адреса"`)
- `newbanned` (По умолчанию: `"В реестр Роскомнадзора добавлены"`)
- `newunbanned` (По умолчанию: `"Из реестра Роскомнадзора удалены"`)
- `stats_date` (По умолчанию: `"Статистика за"`)
- `today_banned` (По умолчанию: `"Сегодня заблокировано"`)
- `today_unbanned` (По умолчанию: `"Сегодня разблокировано"`)
- `top_country` (По умолчанию: `"Топ стран"`)
- `top_isp` (По умолчанию: `"Топ провайдеров"`)
- `total_banned` (По умолчанию: `"Всего заблокировано"`)
- `unbanned` (По умолчанию: `"Удалено записей из реестра"`)

## Список изменений
### v1.0.1
- Исправлена ошибка, мешавшая валидации плагина на Linux системах. Плагин не мог корректно закрыть `stdin`, поэтому сканнер не позволял плагину завершить работу. Теперь работа сканнера закрывается через новый канал управления.
- Обновлена логика сканнера
- Добавлено обрезание пробелов на вводе в сканнер
### v1.0
- Первая сборка