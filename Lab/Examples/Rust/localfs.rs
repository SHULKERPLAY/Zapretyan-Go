use std::io::{self, BufRead, Write};
use std::path::PathBuf;
use std::fs;
use std::process;
use chrono::Local;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const PMODE: &str = "ONCE"; // Can also be "STREAM"
const PJSONVER: i32 = 1;     // Expected JSON version
const PVER: &str = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

// Thread-safe variables for config (as global vars in Go)
static mut CONFIG_SAVE_LAST_ENTRIES: i32 = -1;
static mut CONFIG_DIRECTORY: String = String::new();
static mut TARGET_DIR: String = String::new();

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// log_msg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
macro_rules! log_msg {
    ($($arg:tt)*) => {{
        let mut stderr = io::stderr();
        let _ = writeln!(stderr, $($arg)*);
        let _ = stderr.flush();
    }};
}

fn main() {
    // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    // within 5 seconds after startup or core can throw timeout and not
    // validate your plugin for event processing.

    // Object to send handshake into stdout on start
    let handshake = json!({
        "mode": PMODE,
        "version": PVER,
        "jsonver": PJSONVER
    });

    // Send onelined JSON with \n at the end
    let handshake_str = match serde_json::to_string(&handshake) {
        Ok(s) => s,
        Err(e) => {
            log_msg!("FATAL: Error sending Handshake ({})", e);
            process::exit(1);
        }
    };
    println!("{}", handshake_str);
    io::stdout().flush().unwrap();

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    let stdin = io::stdin();
    // Use BufReader for per-line scan (like Scanner in Go)
    let mut reader = stdin.lock();
    let mut line = String::new();

    // Reuse same variable to save on RAM
    while reader.read_line(&mut line).unwrap_or(0) > 0 {
        let raw_bytes = line.trim();
        if raw_bytes.is_empty() {
            line.clear();
            continue;
        }

        // Send RAW JSON data into your event processor
        handle_event(raw_bytes);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.

        // Clear line for next iteration (internal RAM bufer saves)
        line.clear();
    }

    // If scanner passes till this moment without errors then data flow is closed (EOF)
    log_msg!("stdin closed by core. Shutdown.");
    process::exit(0);
}

// handleEvent decompiles event and forward data for processing
fn handle_event(raw_str: &str) {
    let base: Value = match serde_json::from_str(raw_str) {
        Ok(v) => v,
        Err(e) => {
            log_msg!("Error parsing base structure: {}", e);
            return;
        }
    };

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if base["kill"].as_bool().unwrap_or(false) {
        log_msg("Got KILL signal. Shutdown.");
        process::exit(0);
    }

    // Check proto ver
    let ver = base["ver"].as_i64().unwrap_or(0) as i32;
    if ver != PJSONVER && ver != 0 {
        log_msg!("Warning: Event version ({}) does not match ({})", ver, PJSONVER);
    }

    let event_type = base["type"].as_str().unwrap_or("");

    match event_type {
        // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        "cmd" => {
            unsafe {
                // If config is still empty, and JSON has "cfg" then initialize config
                if CONFIG_SAVE_LAST_ENTRIES == -1 && !base["cfg"].is_null() {
                    CONFIG_SAVE_LAST_ENTRIES = base["cfg"]["save_last_entries"].as_i64().unwrap_or(0) as i32;
                    CONFIG_DIRECTORY = base["cfg"]["localfs_dir"].as_str().unwrap_or("").to_string();
                    log_msg!("Configuration loaded. File limit: {}, subdir: .//{}", CONFIG_SAVE_LAST_ENTRIES, CONFIG_DIRECTORY);
                }

                // Load Directory
                let base_path = base["path"].as_str().unwrap_or("");
                if TARGET_DIR.is_empty() && !base_path.is_empty() && !CONFIG_DIRECTORY.is_empty() {
                    let mut path = PathBuf::from(base_path);
                    path.push(&CONFIG_DIRECTORY);
                    TARGET_DIR = path.to_string_lossy().into_owned();
                    log_msg!("Directory: {}", TARGET_DIR);
                }
            }
        },
        "rkn" => { // Event with possible diffs to process
            let mut save_limit = 100; // Safe default if config still not loaded
            unsafe {
                if CONFIG_SAVE_LAST_ENTRIES > 0 {
                    save_limit = CONFIG_SAVE_LAST_ENTRIES;
                }
            }

            // Pass JSON to event parser
            process_rkn_event(raw_str, save_limit);
        },
        _ => {
            log_msg!("Unknown event type: {}", event_type);
        }
    }
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
fn process_rkn_event(raw_str: &str, save_limit: i32) {
    let raw_obj: Value = match serde_json::from_str(raw_str) {
        Ok(v) => v,
        Err(e) => {
            log_msg!("Error formatting JSON: failed to parse ({})", e);
            return;
        }
    };

    // Format single string JSON as JSON with spaces
    let pretty = match serde_json::to_string_pretty(&raw_obj) {
        Ok(s) => s,
        Err(e) => {
            log_msg!("Error formatting JSON: {}", e);
            return;
        }
    };

    let target_path = unsafe { &TARGET_DIR };

    // Check if directory exists
    if let Err(e) = fs::create_dir_all(target_path) {
        log_msg!("Error creating directory {}: {}", target_path, e);
        return;
    }

    // Save file with date and time
    let file_name = Local::now().format("%Y-%m-%d_%H-%M-%S.txt").to_string();
    let mut file_path = PathBuf::from(target_path);
    file_path.push(&file_name);

    if let Err(e) = fs::write(&file_path, format!("{}\n", pretty)) {
        log_msg!("Error writing file {}: {}", file_path.display(), e);
        return;
    }

    log_msg!("Event written successfuly: {}", file_name);

    // Start file rotation end exit
    rotate_files(save_limit);
}

// rotateFiles Checks limit of files in folder and removes oldest
fn rotate_files(limit: i32) {
    let target_path = unsafe { &TARGET_DIR };

    let entries = match fs::read_dir(target_path) {
        Ok(e) => e,
        Err(e) => {
            log_msg!("Error reading directory contents for rotation: {}", e);
            return;
        }
    };

    // Collect only .txt files to not erase something else
    struct FileItem {
        path: PathBuf,
        name: String,
        mod_time: std::time::SystemTime,
    }

    let mut files: Vec<FileItem> = Vec::new();

    for entry in entries.flatten() {
        if let Ok(file_type) = entry.file_type() {
            if file_type.is_file() {
                let path = entry.path();
                if path.extension().and_then(|s| s.to_str()) == Some("txt") {
                    if let Ok(metadata) = entry.metadata() {
                        if let Ok(mod_time) = metadata.modified() {
                            files.push(FileItem {
                                path,
                                name: entry.file_name().to_string_lossy().into_owned(),
                                mod_time,
                            });
                        }
                    }
                }
            }
        }
    }

    // If files less or equal to limit, doing nothing
    if files.len() <= limit as usize {
        return;
    }

    // Sort files by modified time (old to new)
    files.sort_by(|a, b| a.mod_time.cmp(&b.mod_time));

    // Count how many files to remove
    let files_to_delete = files.len() - limit as usize;

    // Remove N old files
    for file in files.iter().take(files_to_delete) {
        if let Err(e) = fs::remove_file(&file.path) {
            log_msg!("Failed to remove old file {}: {}", file.path.display(), e);
        } else {
            log_msg!("Cleanup: Removed {}", file.name);
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if PMODE == "ONCE" {
        // Kill process if task is done and mode "ONCE"
        process::exit(0);
    }
}