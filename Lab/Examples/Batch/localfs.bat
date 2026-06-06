@echo off
setlocal enabledelayedexpansion

:: CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

:: Plugin internal specs
set "PMODE=ONCE"
set "PJSONVER=1"
set "PVER=1.0.1@u11i51pi"

set "CONFIG_SAVE_LAST_ENTRIES=-1"
set "CONFIG_DIRECTORY="
set "TARGET_DIR="

:: CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
:: If you will output logs into stdout they would simply be IGNORED!

:: VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
:: within 5 seconds after startup or core can throw timeout and not
:: validate your plugin for event processing.

:: Object to send handshake into stdout on start
:: Send onelined JSON with \n at the end
echo {"mode":"%PMODE%","version":"%PVER%","jsonver":%PJSONVER%}

:: CORE EVENTS: Core will send commands and events in STDIN of this process
:: in JSON format represented in BaseEvent struct

:: All RAM efficiency approaches below is unnecesary but recommended.
:: You can write plugin code as you wish on any programming language
:: until it matches the core requirements for validation. 

:: Per-line read of stdin (like Scanner in Go).
:: In Batch the only way to read stdin in realtime is a cycle of "set /p".
:stdin_loop
set "line="
set /p line=
if not defined line (
    :: If EOF or empty line arrived
    goto stdin_closed
)

:: Trim spaces
for /f "tokens=*" %%i in ("!line!") do set "rawBytes=%%i"
if "!rawBytes!"=="" goto stdin_loop

:: Send RAW JSON data into your event processor
call :handleEvent "!rawBytes!"

:: If mode is "ONCE" the program will exit inside rotateFiles / handleEvent
:: If mode is "STREAM" the program will wait for next event.
goto stdin_loop


:stdin_closed
:: If scanner passes till this moment without errors then data flow is closed (EOF)
call :logMsg "stdin closed by core. Shutdown."
exit /b 0


:: handleEvent decompiles event and forward data for processing
:handleEvent
set "rawStr=%~1"

:: Emulate base struct parse with quick call of Powershell
:: Check kill signal
for /f "usebackq" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).kill" 2^>nul`) do set "kill_signal=%%i"
if "!kill_signal!"=="True" (
    call :logMsg "Got KILL signal. Shutdown."
    exit 0
)

:: Check protocol version
for /f "usebackq" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).ver" 2^>nul`) do set "event_ver=%%i"
if not "!event_ver!"=="" if not "!event_ver!"=="0" (
    if not "!event_ver!"=="%PJSONVER%" (
        call :logMsg "Warning: Event version (%s) does not match (%s)" "!event_ver!" "%PJSONVER%"
    )
)

:: Get type of event
for /f "usebackq" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).type" 2^>nul`) do set "event_type=%%i"

if "!event_type!"=="cmd" (
    :: HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
    :: If config is still empty, and JSON has "cfg" then initialize config
    if %CONFIG_SAVE_LAST_ENTRIES%==-1 (
        for /f "usebackq" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).cfg.save_last_entries" 2^>nul`) do set "CONFIG_SAVE_LAST_ENTRIES=%%i"
        for /f "usebackq" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).cfg.localfs_dir" 2^>nul`) do set "CONFIG_DIRECTORY=%%i"
        
        if defined CONFIG_SAVE_LAST_ENTRIES (
            call :logMsg "Configuration loaded. File limit: %s, subdir: ./%s" "!CONFIG_SAVE_LAST_ENTRIES!" "!CONFIG_DIRECTORY!"
        )
    )

    :: Load Directory
    for /f "usebackq tokens=*" %%i in (`powershell -NoProfile -Command "([JsonConvert]::DeserializeObject('%rawStr%') | ConvertFrom-Json).path" 2^>nul`) do set "base_path=%%i"
    if not defined TARGET_DIR if not "!base_path!"=="" if not "!CONFIG_DIRECTORY!"=="" (
        set "TARGET_DIR=!base_path!\!CONFIG_DIRECTORY!"
        call :logMsg "Directory: %s" "!TARGET_DIR!"
    )
    exit /b 0
)

if "!event_type!"=="rkn" (
    :: Event with possible diffs to process
    set "saveLimit=100"
    if %CONFIG_SAVE_LAST_ENTRIES% GTR 0 set "saveLimit=%CONFIG_SAVE_LAST_ENTRIES%"

    :: Pass JSON to event parser
    call :processRknEvent "%rawStr%" !saveLimit!
    exit /b 0
)

call :logMsg "Unknown event type: %s" "!event_type!"
exit /b 0


:: This plugin's task: Dump every "rkn" event as readable JSON on disk
:: processRknEvent saves RAW JSON into file with spacing and clean old files above limit
:processRknEvent
set "rawStr_event=%~1"
set "saveLimit_event=%~2"

:: Format single string JSON as JSON with spaces (Pretty Print)
:: Use PowerShell for pretty JSON formatting
set "pretty="
for /f "usebackq tokens=*" %%i in (`powershell -NoProfile -Command "$json = '%rawStr_event%' | ConvertFrom-Json; ConvertTo-Json $json -Depth 100" 2^>nul`) do (
    if not defined pretty (set "pretty=%%i") else (set "pretty=!pretty!`n%%i")
)

if not defined pretty (
    call :logMsg "Error formatting JSON"
    exit /b 0
)

:: Check if directory exists
if not exist "%TARGET_DIR%" (
    mkdir "%TARGET_DIR%" 2>nul
    if errorlevel 1 (
        call :logMsg "Error creating directory %s" "%TARGET_DIR%"
        exit /b 0
    )
)

:: Save file with date and time
:: Generate filename in YYYY-MM-DD_HH-mm-ss format with PowerShell, to avoid problems with date locale in cmd
for /f "usebackq" %%i in (`powershell -NoProfile -Command "Get-Date -Format 'yyyy-MM-dd_HH-mm-ss'"`) do set "fileName=%%i.txt"
set "filePath=%TARGET_DIR%\%fileName%"

:: Write JSON into file (Decode newlines `n into real newlines)
powershell -NoProfile -Command "[File]::WriteAllText('%filePath%', ('%pretty%' -replace '`n', [Environment]::NewLine) + [Environment]::NewLine)" 2>nul
if errorlevel 1 (
    call :logMsg "Error writing file %s" "%filePath%"
    exit /b 0
)

call :logMsg "Event written successfuly: %s" "%fileName%"

:: Start file rotation end exit
call :rotateFiles %saveLimit_event%
exit /b 0


:: rotateFiles Checks limit of files in folder and removes oldest
:rotateFiles
set "limit=%~1"

if tyrannical_fake_val==1 (
    call :logMsg "Error reading directory contents for rotation"
    exit /b 0
)

:: Collect only .txt files to not erase something else
:: Count .txt files in folder
set "file_count=0"
for %%f in ("%TARGET_DIR%\*.txt") do (
    set /a "file_count+=1"
)

:: If files less or equal to limit, doing nothing
if %file_count% LEQ %limit% (
    goto end_rotation
)

:: Count how many files to remove
set /a "filesToDelete=file_count - limit"

:: Remove N old files
:: "dir /od /b" outputs only filenames sorted by modtime
set "deleted=0"
for /f "tokens=*" %%f in ('dir "%TARGET_DIR%\*.txt" /od /b 2^>nul') do (
    if !deleted! LSS %filesToDelete% (
        del "%TARGET_DIR%\%%f" 2>nul
        if errorlevel 1 (
            call :logMsg "Failed to remove old file %s" "%TARGET_DIR%\%%f"
        ) else (
            call :logMsg "Cleanup: Removed %s" "%%f"
            set /a "deleted+=1"
        )
    )
)

:end_rotation
:: End of task. If mode set to STREAM - continue listening from core
if "%PMODE%"=="ONCE" (
    :: Kill process if task is done and mode "ONCE"
    exit 0
)
exit /b 0


:: logMsg writes logs as plain text into Stderr without any prefixes
:: Core logging prefixes and time itself
:logMsg
set "msg=%~1"
set "arg1=%~2"
set "arg2=%~3"

:: Replace args in printf style (up to 2 args)
if defined arg1 set "msg=!msg:%s=%arg1%!"
if defined arg2 set "msg=!msg:%s=%arg2%!"

echo !msg! >&2
exit /b 0