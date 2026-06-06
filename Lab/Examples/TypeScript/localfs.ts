import * as path from "https://deno.land/std/path/mod.ts";
import { format as dateFormat } from "https://deno.land/std/datetime/mod.ts";

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const pmode = "ONCE"; // Can also be "STREAM"
const pjsonver = 1;   // Expected JSON version
const pver = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

interface PluginConfig {
    save_last_entries: number;
    localfs_dir: string;
}

let config: PluginConfig | null = null;
let targetDir = ""; // Default directory

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
function logMsg(formatStr: string, ...args: any[]): void {
    // Simple line formatting like %s / %d
    let formatted = formatStr;
    args.forEach(arg => {
        formatted = formatted.replace(/%[vdsa-z]/i, arg);
    });
    const encoder = new TextEncoder();
    Deno.stderr.writeSync(encoder.encode(formatted + "\n"));
}

function main(): void {
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
        const encoder = new TextEncoder();
        Deno.stdout.writeSync(encoder.encode(JSON.stringify(handshake) + "\n"));
    } catch (err: any) {
        logMsg("FATAL: Error sending Handshake (%v)", err.message);
        Deno.exit(1);
    }

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    // Using buffered line read from Deno.stdin (like Scanner in Go)
    const buf = new Uint8Array(64 * 1024);
    let leftOver = "";

    while (true) {
        const n = Deno.stdin.readSync(buf);
        if (n === null) {
            // If scanner passes till this moment without errors then data flow is closed (EOF)
            logMsg("stdin closed by core. Shutdown.");
            Deno.exit(0);
        }

        const decoder = new TextDecoder();
        const chunk = decoder.decode(buf.subarray(0, n));
        const lines = (leftOver + chunk).split("\n");
        
        // Save latest (possibly incomplete) line chunk
        leftOver = lines.pop() || "";

        for (const line of lines) {
            const rawBytes = line.trim();
            if (rawBytes.length === 0) {
                continue;
            }

            // Send RAW JSON data into your event processor
            handleEvent(rawBytes);
            // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
            // And there is not so much need for RAM optimizations.

            // If mode is "STREAM" the program will wait for next event.
        }
    }
}

// handleEvent decompiles event and forward data for processing
function handleEvent(rawStr: string): void {
    let base: any;
    try {
        base = JSON.parse(rawStr);
    } catch (err: any) {
        logMsg("Error parsing base structure: %v", err.message);
        return;
    }

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if (base.kill === true) {
        logMsg("Got KILL signal. Shutdown.");
        Deno.exit(0);
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
                config = base.cfg as PluginConfig;
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
function processRknEvent(rawStr: string, saveLimit: number): void {
    let pretty: string;
    try {
        const rawObj = JSON.parse(rawStr);
        // Format single string JSON as JSON with spaces
        pretty = JSON.stringify(rawObj, null, 2);
    } catch (err: any) {
        logMsg("Error formatting JSON: %v", err.message);
        return;
    }

    // Check if directory exists
    try {
        Deno.mkdirSync(targetDir, { recursive: true });
    } catch (err: any) {
        logMsg("Error creating directory %v: %v", targetDir, err.message);
        return;
    }

    // Save file with date and time
    const now = new Date();
    const fileName = dateFormat(now, "yyyy-MM-dd_HH-mm-ss") + ".txt";
    const filePath = path.join(targetDir, fileName);

    try {
        Deno.writeTextFileSync(filePath, pretty + "\n");
    } catch (err: any) {
        logMsg("Error writing file %v: %v", filePath, err.message);
        return;
    }

    logMsg("Event written successfuly: %v", fileName);

    // Start file rotation end exit
    rotateFiles(saveLimit);
}

// rotateFiles Checks limit of files in folder and removes oldest
function rotateFiles(limit: number): void {
    const files: Array<{ name: string; path: string; mtime: number }> = [];

    try {
        for (const entry of Deno.readDirSync(targetDir)) {
            if (entry.isFile && entry.name.endsWith(".txt")) {
                const fullPath = path.join(targetDir, entry.name);
                const info = Deno.statSync(fullPath);
                files.push({
                    name: entry.name,
                    path: fullPath,
                    mtime: info.mtime?.getTime() || 0
                });
            }
        }
    } catch (err: any) {
        logMsg("Error reading directory contents for rotation: %v", err.message);
        return;
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
            Deno.removeSync(files[i].path);
            logMsg("Cleanup: Removed %v", files[i].name);
        } catch (err: any) {
            logMsg("Failed to remove old file %v: %v", files[i].path, err.message);
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if (pmode === "ONCE") {
        // Kill process if task is done and mode "ONCE"
        Deno.exit(0);
    }
}

main();