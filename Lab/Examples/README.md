# ![](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/core/winres/ico32.png) Zapretyan-Go \\=> Примеры (Порт [localfs](https://github.com/SHULKERPLAY/Zapretyan-Go/tree/main/extensions/localfs))
Это хранилище примеров реализации плагинов Zapretyan-Go с любым назначением, платформой и языком. Это лишь малая часть примеров.

> [!CAUTION] 
> **Проверяйте код, если вы копируете его для продакшена!**
> 
> **ПРИМЕРЫ СОЗДАНЫ ПРИ ПОМОЩИ GEMINI 3.5 FLASH С РУЧНОЙ КОРРЕКТИРОВКОЙ!**
>
> Это примеры, показывающие возможность работы на этих языках, а не полностью проверенный для продакшена продукт!

- [ Zapretyan-Go \\=\> Примеры (Порт localfs)](#-zapretyan-go--примеры-порт-localfs)
  - [Замечание для запуска плагинов](#замечание-для-запуска-плагинов)
  - [Порт на C](#порт-на-c)
    - [Исходный код](#исходный-код)
  - [Порт на C++ (C++17)](#порт-на-c-c17)
    - [Исходный код](#исходный-код-1)
  - [Порт на Python (3.10+)](#порт-на-python-310)
    - [Исходный код](#исходный-код-2)
  - [Порт на Node.js (Javascript // ESM)](#порт-на-nodejs-javascript--esm)
    - [Исходный код](#исходный-код-3)
  - [Порт на C# (.NET 6+ / C# 10+)](#порт-на-c-net-6--c-10)
    - [Исходный код](#исходный-код-4)
  - [Порт на Java (Java 11+)](#порт-на-java-java-11)
    - [Исходный код](#исходный-код-5)
  - [Порт на PHP (8.1+)](#порт-на-php-81)
    - [Исходный код](#исходный-код-6)
  - [Порт на Ruby (3.0+)](#порт-на-ruby-30)
    - [Исходный код](#исходный-код-7)
  - [Порт на Bash (4.0+ с `jq`)](#порт-на-bash-40-с-jq)
    - [Исходный код](#исходный-код-8)
  - [Порт на Rust (1.65+ с `serde` и `serde_json`)](#порт-на-rust-165-с-serde-и-serde_json)
    - [Исходный код + cargo.toml](#исходный-код--cargotoml)
  - [Порт на TypeScript (Deno)](#порт-на-typescript-deno)
    - [Исходный код](#исходный-код-9)
  - [Порт на PowerShell (Core / 7+)](#порт-на-powershell-core--7)
    - [Исходный код](#исходный-код-10)
  - [Порт на Lua (`dkjson` + `luafilesystem`)](#порт-на-lua-dkjson--luafilesystem)
    - [Исходный код](#исходный-код-11)

## Замечание для запуска плагинов
На момент `v2.1.0.0` запускать из ядра можно только:
- На Linux: Любые файлы, имеющие разрешение на исполнение (`+X`)
- На Windows: Файлы с расширениями `.bat`, `.sh`, `.exe`.

Если ваш плагин это НЕ скомпилированный исполняемый файл (Например он написан для NodeJS) или если вы обязательно должны передавать аргументы к исполняемому файлу, то вы не сможете запустить плагин напрямую. Для этого вы можете использовать вашу оболочку Bash или Windows CMD как "Прокси" для любых взаимодействий!

- В конфиге настройте запуск bash скрипта или иного для Windows
```toml
[[extension]]
name = "My Plugin"
desc = "Do something with data"
source = "https://lunarcreators.ru"
path = "./extensions/myplugin.sh"
enabled = true
```
- Запуск плагина на Node используя bash как прокси
```bash
#!/bin/bash
#./extensions/myplugin.sh

# Вызов вашего плагина
/usr/bin/node /path/myplugin/index.js
```
- Или если нужно передать аргумент компилированному приложению
```bash
#!/bin/bash
#./extensions/myplugin.sh

# Вызов вашего плагина
./myplugin -arg1 --arg2
```

## Порт на C
Для C использована библиотека `cJSON` (для парсинга/форматирования JSON). Взаимодействие с директориями реализовано через стандартный POSIX (`dirent.h`, `sys/stat.h`), который работает на Linux. Используется динамическое чтение строки через getline (или ручной буфер).

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/C/localfs.c)

## Порт на C++ (C++17)
Для C++ использована библиотека `nlohmann/json` и стандартная библиотека `<filesystem>` (доступна с C++17). Используется динамическое чтение строки через getline (или ручной буфер). Используется std::string с резервированием памяти, чтобы не аллоцировать её на каждой итерации.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/C%2B%2B/localfs.cpp)

## Порт на Python (3.10+)
В Python управление памятью для строк автоматизировано, однако для эффективного чтения используется итератор по `sys.stdin`, который считывает данные построчно по мере их поступления от ядра (аналог `Scanner` из Go). Для парсинга и форматирования используется встроенный модуль `json`.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Python/localfs.py)

## Порт на Node.js (Javascript // ESM)
Для точечного контроля за переносами строк и RAM используется встроенный модуль `readline`. Для логов в `stderr` используется `process.stderr.write` (так как стандартный `console.error` добавляет лишние префиксы или переводы строк в зависимости от среды).

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/NodeJS/localfs.js)

## Порт на C# (.NET 6+ / C# 10+)
Для работы с JSON используется современный и высокопроизводительный встроенный модуль `System.Text.Json`. Код написан с использованием Top-Level Statements (современный стиль C# без лишнего шаблонного кода). Используем `StreamReader.ReadLine` для чтения потока ввода.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/C%23/localfs.cs)

## Порт на Java (Java 11+)
Для работы с JSON в Java нет встроенного парсера "из коробки" (в стандартной библиотеке), поэтому используется самая популярная и производительная библиотека Jackson (`com.fasterxml.jackson.databind`). Используем `BufferedReader.readLine` для чтения потока ввода.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Java/Main.java)

## Порт на PHP (8.1+)
В PHP для вывода логов без лишних префиксов и автоматических переносов строк используется поток `fwrite(STDERR, ...)`. Для работы с JSON применяются стандартные функции `json_decode` и `json_encode`. Потоковое чтение реализовано через `fgets(STDIN)`.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/PHP/localfs.php)

## Порт на Ruby (3.0+)
В Ruby для записи в `stderr` используется метод warn или `STDERR.puts`. Метод `STDIN.each_line` эффективно стримит данные построчно. Работа с JSON построена на базе стандартной библиотеки `json`. Потоковое чтение реализовано через `STDIN.each_line`.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Ruby/localfs.rb)

## Порт на Bash (4.0+ с `jq`)
В Bash для парсинга и генерации JSON используется утилита `jq`, так как в самом командном интерпретаторе нет встроенных средств работы со сложными структурами данных. Вывод логов в `stderr` реализован через перенаправление `>&2`. Построчное чтение работает через цикл `read -r`, который обрабатывает поток ввода по мере поступления данных.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Bash/localfs.sh)

## Порт на Rust (1.65+ с `serde` и `serde_json`)
В Rust для обработки ввода используется `serde` и `serde_json`, а построчное чтение реализуется через `BufReader`, что обеспечивает максимальную производительность и низкое потребление RAM

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Rust/localfs.rs) + [cargo.toml](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Rust/cargo.toml)

## Порт на TypeScript (Deno)
Deno идеально подходит для таких задач благодаря нативной поддержке TypeScript и отличным API для работы с потоками памяти (`Deno.stdin`). Для парсинга и форматирования JSON используются стандартные встроенные средства `JSON.parse` и `JSON.stringify`.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/TypeScript/localfs.ts)

## Порт на PowerShell (Core / 7+)
В PowerShell вывод на `stdout` делается просто через базовый вывод объектов/строк, а для вывода в `stderr` без системных префиксов используется обходной путь через контекст `[Console]::Error.WriteLine()`. Парсинг JSON нативен благодаря `ConvertFrom-Json` и `ConvertTo-Json`.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Powershell/localfs.ps1)

## Порт на Lua (`dkjson` + `luafilesystem`)
Этот вариант полностью и максимально точно повторяет логику исходного кода, включая красивое форматирование (Pretty Print) JSON и точную сортировку по времени изменения файлов.

### [Исходный код](https://github.com/SHULKERPLAY/Zapretyan-Go/blob/main/Lab/Examples/Lua/localfs.lua)