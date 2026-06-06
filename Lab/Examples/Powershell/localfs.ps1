# CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

# Plugin internal specs
$pmode = "ONCE" # Can also be "STREAM"
$pjsonver = 1   # Expected JSON version
$pver = "1.0.1@u11i51pi" # You can specify everything you want. This value will show in core log when validating this plugin

$script:config = $null
$script:targetDir = "" # Default directory

# CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
# If you will output logs into stdout they would simply be IGNORED!

# logMsg writes logs as plain text into Stderr without any prefixes
# Core logging prefixes and time itself
function logMsg([string]$format, [array]$argsList) {
    if ($argsList -and $argsList.Count -gt 0) {
        [Console]::Error.WriteLine(($format -f $argsList))
    } else {
        [Console]::Error.WriteLine($format)
    }
}

# handleEvent decompiles event and forward data for processing
function handleEvent([string]$rawStr) {
    try {
        $baseEvent = ConvertFrom-Json $rawStr -ErrorAction Stop
    } catch {
        logMsg "Error parsing base structure: $_"
        return
    }

    if ($null -eq $baseEvent) { return }

    # Key validation requirement: If in event body "kill" is true then plugin must close its process

    # Exit if "kill": true
    if ($baseEvent.kill -eq $true) {
        logMsg "Got KILL signal. Shutdown."
        [Environment]::Exit(0)
    }

    # Check proto ver
    $ver = if ($baseEvent.ver) { $baseEvent.ver } else { 0 }
    if ($ver -ne $pjsonver -and $ver -ne 0) {
        logMsg "Warning: Event version ({0}) does not match ({1})" @($ver, $pjsonver)
    }

    switch ($baseEvent.type) {
        # HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        "cmd" {
            # If config is still empty, and JSON has "cfg" then initialize config
            if ($null -eq $script:config -and $baseEvent.cfg) {
                $script:config = $baseEvent.cfg
                logMsg "Configuration loaded. File limit: {0}, subdir: ./{1}" @(($script:config.save_last_entries), ($script:config.localfs_dir))
            }

            # Load Directory
            if ([string]::IsNullOrEmpty($script:targetDir) -and $baseEvent.path -and $script:config.localfs_dir) {
                $script:targetDir = Join-Path $baseEvent.path $script:config.localfs_dir
                logMsg "Directory: {0}" @($script:targetDir)
            }
        }

        "rkn" { # Event with possible diffs to process
            $saveLimit = 100 # Safe default if config still not loaded
            if ($script:config -and $script:config.save_last_entries -gt 0) {
                $saveLimit = $script:config.save_last_entries
            }

            # Pass JSON to event parser
            processRknEvent $rawStr $saveLimit
        }

        Default {
            logMsg "Unknown event type: {0}" @($baseEvent.type)
        }
    }
}

# processRknEvent saves RAW JSON into file with spacing and clean old files above limit
function processRknEvent([string]$rawStr, [int]$saveLimit) {
    try {
        $rawObj = ConvertFrom-Json $rawStr -ErrorAction Stop
        # Format single string JSON as JSON with spaces
        $pretty = ConvertTo-Json $rawObj -Depth 100
    } catch {
        logMsg "Error formatting JSON: $_"
        return
    }

    # Check if directory exists
    try {
        if (-not (Test-Path -Path $script:targetDir -PathType Container)) {
            New-Item -ItemType Directory -Force -Path $script:targetDir | Out-Null
        }
    } catch {
        logMsg "Error creating directory {0}: {1}" @($script:targetDir, $_)
        return
    }

    # Save file with date and time
    $fileName = (Get-Date -Format "yyyy-MM-dd_HH-mm-ss") + ".txt"
    $filePath = Join-Path $script:targetDir $fileName

    try {
        # Принудительно используем UTF8 без BOM для совместимости с Linux/Go средами
        [File]::WriteAllText($filePath, ($pretty + "`n"))
    } catch {
        logMsg "Error writing file {0}: {1}" @($filePath, $_)
        return
    }

    logMsg "Event written successfuly: {0}" @($fileName)

    # Start file rotation end exit
    rotateFiles $saveLimit
}

# rotateFiles Checks limit of files in folder and removes oldest
function rotateFiles([int]$limit) {
    try {
        # Collect only .txt files to not erase something else
        $files = Get-ChildItem -Path $script:targetDir -Filter "*.txt" -File | Sort-Object LastWriteTime
    } catch {
        logMsg "Error reading directory contents for rotation: $_"
        return
    }

    # If files less or equal to limit, doing nothing
    if ($files.Count -le $limit) {
        return
    }

    # Count how many files to remove
    $filesToDelete = $files.Count - $limit

    # Remove N old files
    for ($i = 0; $i -lt $filesToDelete; $i++) {
        try {
            Remove-Item -Path $files[$i].FullName -Force -ErrorAction Stop
            logMsg "Cleanup: Removed {0}" @($files[$i].Name)
        } catch {
            logMsg "Failed to remove old file {0}: {1}" @($files[$i].FullName, $_)
        }
    }

    # End of task. If mode set to STREAM - continue listening from core
    if ($pmode -eq "ONCE") {
        # Kill process if task is done and mode "ONCE"
        [Environment]::Exit(0)
    }
}

function main() {
    # VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    # within 5 seconds after startup or core can throw timeout and not
    # validate your plugin for event processing.

    # Object to send handshake into stdout on start
    $handshake = @{
        mode    = $pmode
        version = $pver
        jsonver = $pjsonver
    }

    # Send onelined JSON with \n at the end
    try {
        $handshakeJson = ConvertTo-Json $handshake -Compress
        [Console]::WriteLine($handshakeJson)
    } catch {
        logMsg "FATAL: Error sending Handshake ({0})" @($_.Message)
        [Environment]::Exit(1)
    }

    # CORE EVENTS: Core will send commands and events in STDIN of this process
    # in JSON format represented in BaseEvent struct

    # All RAM efficiency approaches below is unnecesary but recommended.
    # You can write plugin code as you wish on any programming language
    # until it matches the core requirements for validation. 

    # Эффективный пайплайн построчного чтения из стандартного ввода
    while ($null -ne ($line = [Console]::ReadLine())) {
        $rawBytes = $line.Trim()
        if ($rawBytes.Length -eq 0) {
            continue
        }

        # Send RAW JSON data into your event processor
        handleEvent $rawBytes
        # If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        # And there is not so much need for RAM optimizations.

        # If mode is "STREAM" the program will wait for next event.
    }

    # If scanner passes till this moment without errors then data flow is closed (EOF)
    logMsg "stdin closed by core. Shutdown."
    [Environment]::Exit(0)
}

# Запуск скрипта
main