import sys
import os
import json
from datetime import datetime

# CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

# Plugin internal specs
PMODE = "ONCE" # Can also be "STREAM"
PJSONVER = 1   # Expected JSON version
PVER = "1.0.1@u11i51pi" # You can specify everything you want. This value will show in core log when validating this plugin

config = None
target_dir = "" # Default directory

# CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
# If you will output logs into stdout they would simply be IGNORED!

# log_msg writes logs as plain text into Stderr without any prefixes
# Core logging prefixes and time itself
def log_msg(format_str, *args):
    if args:
        sys.stderr.write((format_str % args) + "\n")
    else:
        sys.stderr.write(format_str + "\n")
    sys.stderr.flush()

def main():
    global config, target_dir

    # VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    # within 5 seconds after startup or core can throw timeout and not
    # validate your plugin for event processing.

    # Object to send handshake into stdout on start
    handshake = {
        "mode": PMODE,
        "version": PVER,
        "jsonver": PJSONVER
    }

    # Send onelined JSON with \n at the end
    try:
        sys.stdout.write(json.dumps(handshake) + "\n")
        sys.stdout.flush()
    except Exception as e:
        log_msg("FATAL: Error sending Handshake (%s)", e)
        sys.exit(1)

    # CORE EVENTS: Core will send commands and events in STDIN of this process
    # in JSON format represented in BaseEvent struct

    # All RAM efficiency approaches below is unnecesary but recommended.
    # You can write plugin code as you wish on any programming language
    # until it matches the core requirements for validation. 

    try:
        # Iterations by sys.stdin works as buffered line scanner
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue

            # Send RAW JSON data into your event processor
            handle_event(line)
            # If mode is "ONCE" the program will exit with code 0 inside handleEvent function
            # And there is not so much need for RAM optimizations.

            # If mode is "STREAM" the program will wait for next event.

    except Exception as e:
        log_msg("Error reading from stdin: %s", e)
        sys.exit(1)

    # If scanner passes till this moment without errors then data flow is closed (EOF)
    log_msg("stdin closed by core. Shutdown.")
    sys.exit(0)

# handle_event decompiles event and forward data for processing
def handle_event(raw_str):
    global config, target_dir
    try:
        base = json.loads(raw_str)
    except Exception as e:
        log_msg("Error parsing base structure: %s", e)
        return

    # Key validation requirement: If in event body "kill" is true then plugin must close its process

    # Exit if "kill": true
    if base.get("kill") is True:
        log_msg("Got KILL signal. Shutdown.")
        sys.exit(0)

    # Check proto ver
    ver = base.get("ver", 0)
    if ver != PJSONVER and ver != 0:
        log_msg("Warning: Event version (%d) does not match (%d)", ver, PJSONVER)

    event_type = base.get("type")

    # HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
    if event_type == "cmd":
        # If config is still empty, and JSON has "cfg" then initialize config
        cfg = base.get("cfg")
        if config is None and cfg is not None:
            config = cfg
            log_msg("Configuration loaded. File limit: %d, subdir: ./%s", 
                    config.get("save_last_entries", 0), config.get("localfs_dir", ""))

        # Load Directory
        base_path = base.get("path", "")
        if not target_dir and base_path and config and config.get("localfs_dir"):
            target_dir = os.path.join(base_path, config.get("localfs_dir"))
            log_msg("Directory: %s", target_dir)

    elif event_type == "rkn": # Event with possible diffs to process
        save_limit = 100 # Safe default if config still not loaded
        if config and config.get("save_last_entries", 0) > 0:
            save_limit = config.get("save_last_entries")

        # Pass JSON to event parser
        process_rkn_event(raw_str, save_limit)

    else:
        log_msg("Unknown event type: %s", event_type)

# This plugin's task: Dump every "rkn" event as readable JSON on disk

# process_rkn_event saves RAW JSON into file with spacing and clean old files above limit
def process_rkn_event(raw_str, save_limit):
    try:
        raw_obj = json.loads(raw_str)
        # Format single string JSON as JSON with spaces
        pretty_bytes = json.dumps(raw_obj, indent=2, ensure_ascii=False)
    except Exception as e:
        log_msg("Error formatting JSON: %s", e)
        return

    # Check if directory exists
    try:
        os.makedirs(target_dir, exist_ok=True)
    except Exception as e:
        log_msg("Error creating directory %s: %s", target_dir, e)
        return

    # Save file with date and time
    file_name = datetime.now().strftime("%Y-%m-%d_%H-%M-%S") + ".txt"
    file_path = os.path.join(target_dir, file_name)

    try:
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(pretty_bytes + "\n")
    except Exception as e:
        log_msg("Error writing file %s: %s", file_path, e)
        return

    log_msg("Event written successfuly: %s", file_name)

    # Start file rotation end exit
    rotate_files(save_limit)

# rotate_files Checks limit of files in folder and removes oldest
def rotate_files(limit):
    try:
        entries = os.scandir(target_dir)
    except Exception as e:
        log_msg("Error reading directory contents for rotation: %s", e)
        return

    # Collect only .txt files to not erase something else
    files = []
    for entry in entries:
        if entry.is_file() and entry.name.endswith(".txt"):
            try:
                stat_info = entry.stat()
                files.append((entry.path, entry.name, stat_info.st_mtime))
            except Exception:
                continue

    # If files less or equal to limit, doing nothing
    if len(files) <= limit:
        return

    # Sort files by modified time (old to new)
    files.sort(key=lambda x: x[2])

    # Count how many files to remove
    files_to_delete = len(files) - limit

    # Remove N old files
    for i in range(files_to_delete):
        path, name, _ = files[i]
        try:
            os.remove(path)
            log_msg("Cleanup: Removed %s", name)
        except Exception as e:
            log_msg("Failed to remove old file %s: %s", path, e)

    # End of task. If mode set to STREAM - continue listening from core
    if PMODE == "ONCE":
        # Kill process if task is done and mode "ONCE"
        sys.exit(0)

if __name__ == "__main__":
    main()