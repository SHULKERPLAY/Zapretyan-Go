# ![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/core/winres/ico32.png) Zapretyan-Go \\=> Daily Statistiscs 1.0.1 (Официальный плагин)

## Назначение
Считает количество блокировок/разблокировок в событиях `"type": "rkn"` во временном файле и записывает актуальные данные каждый день. Может записывать один JSON с изменениями текущего дня и историческую `.csv` таблицу по дням

### Форма сохраняемого JSON
```json
{"todayban":"0","todayunban":"0","totalban":"1448696 (+1304 за сутки)","rawtotalban":"1448696","todayipban":"0","todayipunban":"0","totalipban":"806078 (+726 за сутки)","rawtotalipban":"806078"}
```

### Форма таблицы CSV с историей
```xlsx
Date;Banned Domains;Unbanned Domains;Total banned Domains;Banned IPs;Unbanned IPs;Total banned IPs
14.06.2026;1978;242;1448696;0;0;806078
15.06.2026;0;0;1448696;0;0;806078
16.06.2026;0;0;1448696;0;0;806078
``` 

## Системные требования
- Версия ядра Zapretyan-Go не ниже `v2.1.0.0` с поддерживаемой версией JSON протокола `1`
- ~10МБ ОЗУ при работе

## Установка
- Исполняемый файл плагина рекомендуется помещать в папку с расширениями ядра: `./extensions`
- Добавьте конфигурацию плагина в ядро:

```toml
[[extension]]
name = "Daily Statistics"
desc = "Write daily statistics to today's JSON and historical CSV"
source = "https://github.com/SHULKERPLAY/Zapretyan-Go"
path = "./extensions/dailystat"
enabled = true
latestjson = false
json_file = "./statistics/latest.json"
analytics = true
csv_file = "./statistics/analyticsV2.csv"
day_start = 0
    [extension.locale]
    for24hrs = "за 24 часа"
```

## Конфигурация

- `latestjson` (По умолчанию: `false`)
  - Включает сохранение и перезапись JSON файла с данными последнего дня
- `json_file` (По умолчанию: `"./statistics/latest.json"`)
  - Относительный от папки данных ядра или полный путь к JSON файлу последнего дня 
- `analytics` (По умолчанию: `true`)
  - Включает запись истории по дням в CSV таблицу
- `csv_file` (По умолчанию: `"./statistics/analyticsV2.csv"`)
  - Относительный от папки данных ядра или полный путь к CSV таблице с историей
- `day_start` (По умолчанию: `0`, Минимум: `0`, Максимум: `23`)
  - Час системного времени в 24-часовом формате по наступлении которого будет производиться ротация файлов и счётчик начнёт новый день
- `locale.for24hrs` (По умолчанию: `"за 24 часа"`)
  - Надпись, которая будет идти в разнице между днями в JSON файле
    - `"totalban":"1448696 (+1304 за 24 часа)"`


## Список изменений
### v1.0.1
- Исправлена ошибка, когда при указании абсолютного пути в конфигурации, этот путь всё равно склеивался с основной директорией ядра
- Исправлен формат вывода текста разницы, так как текст в скобках должен быть обёрнут с помощью \`( )\`
### v1.0
- Первая сборка