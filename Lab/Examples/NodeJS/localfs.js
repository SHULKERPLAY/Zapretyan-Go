import readline from 'readline';
import fs from 'fs';
import path from 'path';

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const pmode = "ONCE"; // Can also be "STREAM"
const pjsonver = 1;   // Expected JSON version
const pver = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

let config = null;
let targetDir = ""; // Default directory

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
function logMsg(formatStr, ...args) {
    // Simple string formatting like %s / %d
    let formatted = formatStr;
    args.forEach(arg => {
        formatted = formatted.replace(/%[vdsa-z]/i, arg);
    });
    process.stderr.write(formatted + "\n");
}

function main() {
    // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    // within 5 seconds after startup or core can throw timeout and not
    // validate your plugin for event processing.

    // Object to send handshake into stdout on start
    const handshake = {
        mode: pmode,
        version: pver,
        jsonver: pjsonver
    };

    // Send onelined JSON with \n at the end
    try {
        process.stdout.write(JSON.stringify(handshake) + "\n");
    } catch (err) {
        logMsg("FATAL: Error sending Handshake (%v)", err);
        process.exit(1);
    }

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    // Использование readline интерфейса поверх process.stdin (аналог Scanner в Go)
    const rl = readline.createInterface({
        input: process.stdin,
        output: null,
        terminal: false,
        historySize: 0 // Disable history to save on RAM
    });

    rl.on('line', (line) => {
        const rawBytes = line.trim();
        if (rawBytes.length === 0) {
            return;
        }

        // Send RAW JSON data into your event processor
        handleEvent(rawBytes);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.
    });

    rl.on('close', () => {
        // If scanner passes till this moment without errors then data flow is closed (EOF)
        logMsg("stdin closed by core. Shutdown.");
        process.exit(0);
    });

    process.stdin.on('error', (err) => {
        logMsg("Error reading from stdin: %v", err);
        process.exit(1);
    });
}

// handleEvent decompiles event and forward data for processing
function handleEvent(rawStr) {
    let base;
    try {
        base = JSON.parse(rawStr);
    } catch (err) {
        logMsg("Error parsing base structure: %v", err);
        return;
    }

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if (base.kill === true) {
        logMsg("Got KILL signal. Shutdown.");
        process.exit(0);
    }

    // Check proto ver
    if (base.ver !== pjsonver && base.ver !== 0 && base.ver !== undefined) {
        logMsg("Warning: Event version (%v) does not match (%v)", base.ver, pjsonver);
    }

    switch (base.type) {
        // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        case "cmd":
            // If config is still empty, and JSON has "cfg" then initialize config
            if (config === null && base.cfg) {
                config = base.cfg;
                logMsg("Configuration loaded. File limit: %v, subdir: ./%v", config.save_last_entries, config.localfs_dir);
            }

            // Load Directory
            if (targetDir === "" && base.path && config && config.localfs_dir) {
                targetDir = path.join(base.path, config.localfs_dir);
                logMsg("Directory: %v", targetDir);
            }
            break;

        case "rkn": // Event with possible diffs to process
            let saveLimit = 100; // Safe default if config still not loaded
            if (config && config.save_last_entries > 0) {
                saveLimit = config.save_last_entries;
            }

            // Pass JSON to event parser
            processRknEvent(rawStr, saveLimit);
            break;

        default:
            logMsg("Unknown event type: %v", base.type);
    }
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
function processRknEvent(rawStr, saveLimit) {
    let pretty;
    try {
        const rawObj = JSON.parse(rawStr);
        // Format single string JSON as JSON with spaces
        pretty = JSON.stringify(rawObj, null, 2);
    } catch (err) {
        logMsg("Error formatting JSON: %v", err);
        return;
    }

    // Check if directory exists
    try {
        if (!fs.existsSync(targetDir)) {
            fs.mkdirSync(targetDir, { recursive: true });
        }
    } catch (err) {
        logMsg("Error creating directory %v: %v", targetDir, err);
        return;
    }

    // Save file with date and time
    const now = new Date();
    const pad = (n) => n.toString().padStart(2, '0');
    const fileName = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}-${pad(now.getMinutes())}-${pad(now.getSeconds())}.txt`;
    const filePath = path.join(targetDir, fileName);

    try {
        fs.writeFileSync(filePath, pretty + "\n", 'utf8');
    } catch (err) {
        logMsg("Error writing file %v: %v", filePath, err);
        return;
    }

    logMsg("Event written successfuly: %v", fileName);

    // Start file rotation end exit
    rotateFiles(saveLimit);
}

// rotateFiles Checks limit of files in folder and removes oldest
function rotateFiles(limit) {
    let entries;
    try {
        entries = fs.readdirSync(targetDir);
    } catch (err) {
        logMsg("Error reading directory contents for rotation: %v", err);
        return;
    }

    // Collect only .txt files to not erase something else
    const files = [];
    for (const entry of entries) {
        const fullPath = path.join(targetDir, entry);
        try {
            const stat = fs.statSync(fullPath);
            if (stat.isFile() && path.extname(entry) === '.txt') {
                files.push({
                    name: entry,
                    path: fullPath,
                    mtime: stat.mtimeMs
                });
            }
        } catch (e) {
            continue;
        }
    }

    // If files less or equal to limit, doing nothing
    if (files.length <= limit) {
        return;
    }

    // Sort files by modified time (old to new)
    files.sort((a, b) => a.mtime - b.mtime);

    // Count how many files to remove
    const filesToDelete = files.length - limit;

    // Remove N old files
    for (let i = 0; i < filesToDelete; i++) {
        try {
            fs.unlinkSync(files[i].path);
            logMsg("Cleanup: Removed %v", files[i].name);
        } catch (err) {
            logMsg("Failed to remove old file %v: %v", files[i].path, err);
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if (pmode === "ONCE") {
        // Kill process if task is done and mode "ONCE"
        process.exit(0);
    }
}

main();