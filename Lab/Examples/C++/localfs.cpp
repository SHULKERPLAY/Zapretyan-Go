#include <iostream>
#include <string>
#include <vector>
#include <chrono>
#include <iomanip>
#include <fstream>
#include <algorithm>
#include <filesystem>
#include <cstdarg>
#include "json.hpp" // using nlohmann::json

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

using json = nlohmann::json;
namespace fs = std::filesystem;

// Plugin internal specs
const std::string pmode = "ONCE"; // Can also be "STREAM"
const int pjsonver = 1;           // Expected JSON version
const std::string pver = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

// Config struct
struct PluginConfig {
    int save_last_entries = 0;
    std::string directory;
};

PluginConfig* config = nullptr;
fs::path targetDir;

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
void logMsg(const char* format, ...) {
    va_list args;
    va_start(args, format);
    // For C++ we can use patterns but save original style Go/C with vfprintf
    vfprintf(stderr, format, args);
    fprintf(stderr, "\n");
    va_end(args);
    fflush(stderr);
}

// Declaring functions
void handleEvent(const std::string& rawEventStr);
void processRknEvent(const std::string& rawEventStr, int saveLimit);
void rotateFiles(int limit);

int main() {
    // Disable sync of iostream with stdio for speeding up work with threads
    std::ios_base::sync_with_stdio(false);

    // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    // within 5 seconds after startup or core can throw timeout and not
    // validate your plugin for event processing.

    // Object to send handshake into stdout on start
    json handshake = {
        {"mode", pmode},
        {"version", pver},
        {"jsonver", pjsonver}
    };

    // Send onelined JSON with \n at the end
    std::cout << handshake.dump() << "\n";
    std::cout.flush();

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    std::string line;
    // Allocating memory (Same as start buffer in Go) to avoid frequent reallocations
    line.reserve(64 * 1024); 

    while (std::getline(std::cin, line)) {
        if (line.empty()) {
            continue;
        }

        // Send RAW JSON data into your event processor
        handleEvent(line);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.

        // C++ does not have GC. Memory of `line` reusing on every iteration 
        // and temp objects inside handleEvent removes automaticly (RAII).
    }

    // Check why cycle is ended
    if (std::cin.bad()) {
        logMsg("Error reading from stdin");
    } else {
        // If scanner passes till this moment without errors then data flow is closed (EOF)
        logMsg("stdin closed by core. Shutdown.");
        if (config) delete config;
        std::exit(0);
    }
    return 0;
}

// handleEvent decompiles event and forward data for processing
void handleEvent(const std::string& rawEventStr) {
    json base;
    try {
        base = json::parse(rawEventStr);
    } catch (const json::parse_error& err) {
        logMsg("Error parsing base structure: %s", err.what());
        return;
    }

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if (base.contains("kill") && base["kill"].get<bool>()) {
        logMsg("Got KILL signal. Shutdown.");
        if (config) delete config;
        std::exit(0);
    }

    // Check proto ver
    int ver = base.value("ver", 0);
    if (ver != pjsonver && ver != 0) {
        logMsg("Warning: Event version (%d) does not match (%d)", ver, pjsonver);
    }

    std::string type = base.value("type", "");

    if (type == "cmd") {
        // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        // If config is still empty, and JSON has "cfg" then initialize config
        if (config == nullptr && base.contains("cfg") && !base["cfg"].is_null()) {
            config = new PluginConfig();
            config->save_last_entries = base["cfg"].value("save_last_entries", 0);
            config->directory = base["cfg"].value("localfs_dir", "");
            logMsg("Configuration loaded. File limit: %d, subdir: ./%s", config->save_last_entries, config->directory.c_str());
        }

        // Load Directory
        std::string base_path = base.value("path", "");
        if (targetDir.empty() && !base_path.empty() && config && !config->directory.empty()) {
            targetDir = fs::path(base_path) / config->directory;
            logMsg("Directory: %s", targetDir.string().c_str());
        }

    } else if (type == "rkn") { // Event with possible diffs to process
        int saveLimit = 100; // Safe default if config still not loaded
        if (config && config->save_last_entries > 0) {
            saveLimit = config->save_last_entries;
        }

        // Pass JSON to event parser
        processRknEvent(rawEventStr, saveLimit);

    } else {
        logMsg("Unknown event type: %s", type.c_str());
    }
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
void processRknEvent(const std::string& rawEventStr, int saveLimit) {
    std::string prettyStr;
    try {
        json raw = json::parse(rawEventStr);
        // Format single string JSON as JSON with spaces (ширина отступа - 2 пробела)
        prettyStr = raw.dump(2);
    } catch (const std::exception& err) {
        logMsg("Error formatting JSON: %s", err.what());
        return;
    }

    // Check if directory exists
    std::error_code ec;
    fs::create_directories(targetDir, ec);
    if (ec) {
        logMsg("Error creating directory %s: %s", targetDir.string().c_str(), ec.message().c_str());
        return;
    }

    // Save file with date and time
    auto now = std::chrono::system_clock::now();
    auto in_time_t = std::chrono::system_clock::to_time_t(now);
    std::stringstream ss;
    ss << std::put_time(std::localtime(&in_time_t), "%Y-%m-%d_%H-%M-%S") << ".txt";
    std::string fileName = ss.str();
    
    fs::path filePath = targetDir / fileName;

    std::ofstream outFile(filePath);
    if (!outFile.is_open()) {
        logMsg("Error writing file %s", filePath.string().c_str());
        return;
    }

    outFile << prettyStr << "\n";
    outFile.close();

    logMsg("Event written successfuly: %s", fileName.c_str());

    // Start file rotation end exit
    rotateFiles(saveLimit);
}

// rotateFiles Checks limit of files in folder and removes oldest
void rotateFiles(int limit) {
    std::error_code ec;
    if (!fs::exists(targetDir) || !fs::is_directory(targetDir)) {
        logMsg("Error reading directory contents for rotation: Directory does not exist");
        return;
    }

    // Collect only .txt files to not erase something else
    struct FileInfo {
        fs::path path;
        fs::file_time_type modTime;
    };
    std::vector<FileInfo> files;

    for (const auto& entry : fs::directory_iterator(targetDir, ec)) {
        if (entry.is_regular_file() && entry.path().extension() == ".txt") {
            files.push_back({entry.path(), entry.last_write_time()});
        }
    }

    if (ec) {
        logMsg("Error reading directory contents for rotation: %s", ec.message().c_str());
        return;
    }

    // If files less or equal to limit, doing nothing
    if (static_cast<int>(files.size()) <= limit) {
        return;
    }

    // Sort files by modified time (old to new)
    std::sort(files.begin(), files.end(), [](const FileInfo& a, const FileInfo& b) {
        return a.modTime < b.modTime;
    });

    // Count how many files to remove
    int filesToDelete = files.size() - limit;

    // Remove N old files
    for (int i = 0; i < filesToDelete; i++) {
        if (fs::remove(files[i].path, ec)) {
            logMsg("Cleanup: Removed %s", files[i].path.filename().string().c_str());
        } else {
            logMsg("Failed to remove old file %s: %s", files[i].path.string().c_str(), ec.message().c_str());
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if (pmode == "ONCE") {
        // Kill process if task is done and mode "ONCE"
        if (config) delete config;
        std::exit(0);
    }
}