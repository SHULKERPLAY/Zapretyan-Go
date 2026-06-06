#!/bin/bash

# CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

# Plugin internal specs
PMODE="ONCE" # Can also be "STREAM"
PJSONVER=1   # Expected JSON version
PVER="1.0.1@u11i51pi" # You can specify everything you want. This value will show in core log when validating this plugin

CONFIG_SAVE_LAST_ENTRIES=-1
CONFIG_DIRECTORY=""
TARGET_DIR=""

# CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
# If you will output logs into stdout they would simply be IGNORED!

# logMsg writes logs as plain text into Stderr without any prefixes
# Core logging prefixes and time itself
logMsg() {
    local format_str="$1"
    shift
    if [ $# -gt 0 ]; then
        printf "${format_str}\n" "$@" >&2
    else
        printf "${format_str}\n" >&2
    fi
}

# Functions declaration (in Bash functions must be declared before call)
rotateFiles() {
    local limit="$1"

    if [ ! -d "$TARGET_DIR" ]; then
        logMsg "Error reading directory contents for rotation: Directory does not exist"
        return
    fi

    # Collect only .txt files to not erase something else
    # In Bash we can use stat for getting modify time and sort files
    local files=()
    
    # Disabling globbing in case of file gone. Check only regular files
    shopt -s nullglob
    local raw_files=("$TARGET_DIR"/*.txt)
    shopt -u nullglob

    if [ ${#raw_files[@]} -eq 0 ]; then
        return
    fi

    # Sort files by modtime
    # Use stat -c %Y for Linux (macOS might use stat -f %m)
    local sorted_files
    sorted_files=$(stat -c "%Y %p" "${raw_files[@]}" 2>/dev/null | sort -n | cut -d' ' -f2-)

    # Move to array
    local files_array=()
    while IFS= read -r line; do
        [ -n "$line" ] && files_array+=("$line")
    done <<< "$sorted_files"

    # If files less or equal to limit, doing nothing
    if [ ${#files_array[@]} -le "$limit" ]; then
        return
    fi

    # Count how many files to remove
    local files_to_delete=$(( ${#files_array[@]} - limit ))

    # Remove N old files
    for ((i=0; i<files_to_delete; i++)); do
        local file_path="${files_array[i]}"
        local file_name
        file_name=$(basename "$file_path")
        
        if rm "$file_path" 2>/dev/null; then
            logMsg "Cleanup: Removed %s" "$file_name"
        else
            logMsg "Failed to remove old file %s" "$file_path"
        fi
    done

    # End of task. If mode set to STREAM - continue listening from core
    if [ "$PMODE" = "ONCE" ]; then
        # Kill process if task is done and mode "ONCE"
        exit 0
    fi
}

processRknEvent() {
    local raw_json="$1"
    local save_limit="$2"

    # Format single string JSON as JSON with spaces
    local pretty
    pretty=$(echo "$raw_json" | jq '.' 2>/dev/null)
    if [ -z "$pretty" ]; then
        logMsg "Error formatting JSON"
        return
    fi

    # Check if directory exists
    if [ ! -d "$TARGET_DIR" ]; then
        if ! mkdir -p "$TARGET_DIR" 2>/dev/null; then
            logMsg "Error creating directory %s" "$TARGET_DIR"
            return
        fi
    fi

    # Save file with date and time
    local file_name
    file_name=$(date +"%Y-%m-%d_%H-%M-%S.txt")
    local file_path="${TARGET_DIR}/${file_name}"

    if echo "$pretty" > "$file_path" 2>/dev/null; then
        logMsg "Event written successfuly: %s" "$file_name"
    else
        logMsg "Error writing file %s" "$file_path"
        return
    fi

    # Start file rotation end exit
    rotateFiles "$save_limit"
}

handleEvent() {
    local raw_json="$1"

    # Validate JSON with jq
    if ! echo "$raw_json" | jq -e . >/dev/null 2>&1; then
        logMsg "Error parsing base structure"
        return
    fi

    # Key validation requirement: If in event body "kill" is true then plugin must close its process

    # Exit if "kill": true
    local kill_signal
    kill_signal=$(echo "$raw_json" | jq -r '.kill // false')
    if [ "$kill_signal" = "true" ]; then
        logMsg "Got KILL signal. Shutdown."
        exit 0
    fi

    # Check proto ver
    local ver
    ver=$(echo "$raw_json" | jq -r '.ver // 0')
    if [ "$ver" -ne "$PJSONVER" ] && [ "$ver" -ne 0 ]; then
        logMsg "Warning: Event version (%s) does not match (%s)" "$ver" "$PJSONVER"
    fi

    local type
    type=$(echo "$raw_json" | jq -r '.type // ""')

    if [ "$type" = "cmd" ]; then
        # HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        # If config is still empty, and JSON has "cfg" then initialize config
        if [ "$CONFIG_SAVE_LAST_ENTRIES" -eq -1 ]; then
            local has_cfg
            has_cfg=$(echo "$raw_json" | jq -r '.cfg // empty')
            if [ -n "$has_cfg" ]; then
                CONFIG_SAVE_LAST_ENTRIES=$(echo "$raw_json" | jq -r '.cfg.save_last_entries // 0')
                CONFIG_DIRECTORY=$(echo "$raw_json" | jq -r '.cfg.localfs_dir // ""')
                logMsg "Configuration loaded. File limit: %s, subdir: ./%s" "$CONFIG_SAVE_LAST_ENTRIES" "$CONFIG_DIRECTORY"
            fi
        fi

        # Load Directory
        local base_path
        base_path=$(echo "$raw_json" | jq -r '.path // ""')
        if [ -z "$TARGET_DIR" ] && [ -n "$base_path" ] && [ -n "$CONFIG_DIRECTORY" ]; then
            TARGET_DIR="${base_path}/${CONFIG_DIRECTORY}"
            logMsg "Directory: %s" "$TARGET_DIR"
        fi

    elif [ "$type" = "rkn" ]; then # Event with possible diffs to process
        local save_limit=100 # Safe default if config still not loaded
        if [ "$CONFIG_SAVE_LAST_ENTRIES" -gt 0 ]; then
            save_limit="$CONFIG_SAVE_LAST_ENTRIES"
        fi

        # Pass JSON to event parser
        processRknEvent "$raw_json" "$save_limit"
    else
        logMsg "Unknown event type: %s" "$type"
    fi
}

# Check if jq installed
if ! command -v jq >/dev/null 2>&1; then
    logMsg "FATAL: jq command not found. Please install jq to use this plugin."
    exit 1
fi

# VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
# within 5 seconds after startup or core can throw timeout and not
# validate your plugin for event processing.

# Object to send handshake into stdout on start (строим onelined JSON)
handshake_json=$(jq -n --arg mode "$PMODE" --arg version "$PVER" --argjson jsonver "$PJSONVER" \
    '{mode: $mode, version: $version, jsonver: $jsonver}')

# Send onelined JSON with \n at the end
if [ -z "$handshake_json" ]; then
    logMsg "FATAL: Error sending Handshake"
    exit 1
fi
echo "$handshake_json"

# CORE EVENTS: Core will send commands and events in STDIN of this process
# in JSON format represented in BaseEvent struct

# All RAM efficiency approaches below is unnecesary but recommended.
# You can write plugin code as you wish on any programming language
# until it matches the core requirements for validation. 

# Per-Line read of input (like Scanner in Go)
while IFS= read -r line || [ -n "$line" ]; do
    raw_bytes=$(echo "$line" | xargs) # Cut spaces (trim)
    if [ -z "$raw_bytes" ]; then
        continue
    fi

    # Send RAW JSON data into your event processor
    handleEvent "$raw_bytes"
    # If mode is "ONCE" the program will exit with code 0 inside handleEvent function
    # And there is not so much need for RAM optimizations.

    # If mode is "STREAM" the program will wait for next event.
done

# If scanner passes till this moment without errors then data flow is closed (EOF)
logMsg "stdin closed by core. Shutdown."
exit 0