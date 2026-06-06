<?php

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const PMODE = "ONCE"; // Can also be "STREAM"
const PJSONVER = 1;   // Expected JSON version
const PVER = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

$config = null;
$targetDir = ""; // Default directory

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
function logMsg(string $format, ...$args): void {
    $formatted = count($args) > 0 ? sprintf($format, ...$args) : $format;
    fwrite(STDERR, $formatted . "\n");
}

function main(): void {
    global $config, $targetDir;

    // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    // within 5 seconds after startup or core can throw timeout and not
    // validate your plugin for event processing.

    // Object to send handshake into stdout on start
    $handshake = [
        "mode" => PMODE,
        "version" => PVER,
        "jsonver" => PJSONVER
    ];

    // Send onelined JSON with \n at the end
    $handshakeJson = json_encode($handshake);
    if ($handshakeJson === false) {
        logMsg("FATAL: Error sending Handshake");
        exit(1);
    }
    echo $handshakeJson . "\n";
    flush();

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    // Per-line reading from stdin (like Scanner in Go)
    while (($line = fgets(STDIN)) !== false) {
        $rawBytes = trim($line);
        if ($rawBytes === "") {
            continue;
        }

        // Send RAW JSON data into your event processor
        handleEvent($rawBytes);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.
    }

    // Check why cycle is ended
    // If fgets return false and it is not EOF then it is an error
    if (!feof(STDIN)) {
        logMsg("Error reading from stdin");
    } else {
        // If scanner passes till this moment without errors then data flow is closed (EOF)
        logMsg("stdin closed by core. Shutdown.");
        exit(0);
    }
}

// handleEvent decompiles event and forward data for processing
function handleEvent(string $rawStr): void {
    global $config, $targetDir;

    $baseEvent = json_decode($rawStr, true);
    if ($baseEvent === null) {
        logMsg("Error parsing base structure");
        return;
    }

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if (isset($baseEvent['kill']) && $baseEvent['kill'] === true) {
        logMsg("Got KILL signal. Shutdown.");
        exit(0);
    }

    // Check proto ver
    $ver = $baseEvent['ver'] ?? 0;
    if ($ver !== PJSONVER && $ver !== 0) {
        logMsg("Warning: Event version (%d) does not match (%d)", $ver, PJSONVER);
    }

    $type = $baseEvent['type'] ?? "";

    switch ($type) {
        // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        case "cmd":
            // If config is still empty, and JSON has "cfg" then initialize config
            if ($config === null && isset($baseEvent['cfg'])) {
                $config = $baseEvent['cfg'];
                logMsg("Configuration loaded. File limit: %d, subdir: ./%s", 
                    $config['save_last_entries'] ?? 0, 
                    $config['localfs_dir'] ?? ""
                );
            }

            // Load Directory
            $basePath = $baseEvent['path'] ?? "";
            $localDir = $config['localfs_dir'] ?? "";
            if ($targetDir === "" && $basePath !== "" && $localDir !== "") {
                $targetDir = rtrim($basePath, '/\\') . DIRECTORY_SEPARATOR . $localDir;
                logMsg("Directory: %s", $targetDir);
            }
            break;

        case "rkn": // Event with possible diffs to process
            $saveLimit = 100; // Safe default if config still not loaded
            if ($config !== null && isset($config['save_last_entries']) && $config['save_last_entries'] > 0) {
                $saveLimit = $config['save_last_entries'];
            }

            // Pass JSON to event parser
            processRknEvent($rawStr, $saveLimit);
            break;

        default:
            logMsg("Unknown event type: %s", $type);
            break;
    }
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
function processRknEvent(string $rawStr, int $saveLimit): void {
    global $targetDir;

    $rawObj = json_decode($rawStr);
    if ($rawObj === null) {
        logMsg("Error formatting JSON: failed to decode");
        return;
    }

    // Format single string JSON as JSON with spaces
    $pretty = json_encode($rawObj, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE);
    if ($pretty === false) {
        logMsg("Error formatting JSON: failed to encode pretty");
        return;
    }

    // Check if directory exists
    if (!is_dir($targetDir)) {
        if (!mkdir($targetDir, 0777, true) && !is_dir($targetDir)) {
            logMsg("Error creating directory %s", $targetDir);
            return;
        }
    }

    // Save file with date and time
    $fileName = date("Y-m-d_H-i-s") . ".txt";
    $filePath = $targetDir . DIRECTORY_SEPARATOR . $fileName;

    if (file_put_contents($filePath, $pretty . "\n") === false) {
        logMsg("Error writing file %s", $filePath);
        return;
    }

    logMsg("Event written successfuly: %s", $fileName);

    // Start file rotation end exit
    rotateFiles($saveLimit);
}

// rotateFiles Checks limit of files in folder and removes oldest
function rotateFiles(int $limit): void {
    global $targetDir;

    if (!is_dir($targetDir)) {
        logMsg("Error reading directory contents for rotation: not a directory");
        return;
    }

    // Collect only .txt files to not erase something else
    $files = [];
    $dirMatches = glob($targetDir . DIRECTORY_SEPARATOR . "*.txt");
    
    if ($dirMatches === false) {
        logMsg("Error reading directory contents for rotation");
        return;
    }

    foreach ($dirMatches as $filePath) {
        if (is_file($filePath)) {
            $files[] = [
                'path' => $filePath,
                'name' => basename($filePath),
                'mtime' => filemtime($filePath)
            ];
        }
    }

    // If files less or equal to limit, doing nothing
    if (count($files) <= $limit) {
        return;
    }

    // Sort files by modified time (old to new)
    usort($files, function ($a, $b) {
        return $a['mtime'] <=> $b['mtime'];
    });

    // Count how many files to remove
    $filesToDelete = count($files) - $limit;

    // Remove N old files
    for ($i = 0; i < $filesToDelete; i++) {
        if (!unlink($files[$i]['path'])) {
            logMsg("Failed to remove old file %s", $files[$i]['path']);
        } else {
            logMsg("Cleanup: Removed %s", $files[$i]['name']);
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if (PMODE === "ONCE") {
        // Kill process if task is done and mode "ONCE"
        exit(0);
    }
}

// Start main app cycle
main();