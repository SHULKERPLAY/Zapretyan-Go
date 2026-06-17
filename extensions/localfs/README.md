# ![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/core/winres/ico32.png) Zapretyan-Go \\=> localfs 1.0.1 (Официальный плагин)

## Назначение
Записывает каждое полученное от ядра событие типа `"type": "rkn"` в файл как читаемый JSON с `.txt` расширением

## Системные требования
- Версия ядра Zapretyan-Go не ниже `v2.1.0.0` с поддерживаемой версией JSON протокола `1`
- ~10МБ ОЗУ при работе

## Установка

- Исполняемый файл плагина рекомендуется помещать в папку с расширениями ядра: `./extensions`

- Добавьте конфигурацию плагина в ядро:

```toml
[[extension]]
name = "File Logger"
desc = "Write every event JSON into ./data/localfs_dir"
source = "https://github.com/SHULKERPLAY/Zapretyan-Go"
path = "./extensions/localfs"
enabled = true
save_last_entries = 100
localfs_dir = "localfs_logger"
```

## Конфигурация

- `save_last_entries` (По умолчанию: `100`)
  - Определяет какое количество последних `.txt` файлов должно быть сохранено в директории
- `localfs_dir` (По умолчанию: `localfs_logger`)
  - Определяет название папки в которую будут сохранятся записи. Директория будет находится внутри папки данных ядра
  ```toml
  # Для версии v2.1.0.0
    [core.data]
    data_dir = "./data"
  ```

## Список изменений
### v1.0.1
- Оптимизация использования ОЗУ путём назначения строгого размера буфера
### v1.0
- Первая сборка