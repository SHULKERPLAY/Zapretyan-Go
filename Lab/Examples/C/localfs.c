#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <time.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <dirent.h>
#include <unistd.h>
#include "cjson.h" // Use lightweight lib cJSON

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const char* pmode = "ONCE"; // Can also be "STREAM"
const int pjsonver = 1;     // Expected JSON version
const char* pver = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

// Global variables for config
int cfg_save_last_entries = -1;
char cfg_directory[256] = "";
char target_dir[512] = "";

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
void logMsg(const char* format, ...) {
    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    fprintf(stderr, "\n");
    va_end(args);
    fflush(stderr);
}

// Declare functions
void handleEvent(const char* raw_json);
void processRknEvent(const char* raw_json, int save_limit);
void rotateFiles(int limit);
void create_directory_recursive(const char* dir);

int main() {
    // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    // within 5 seconds after startup or core can throw timeout and not
    // validate your plugin for event processing.

    // Object to send handshake into stdout on start
    cJSON *handshake = cJSON_CreateObject();
    cJSON_AddStringToObject(handshake, "mode", pmode);
    cJSON_AddStringToObject(handshake, "version", pver);
    cJSON_AddNumberToObject(handshake, "jsonver", pjsonver);

    // Send onelined JSON with \n at the end
    char *handshake_str = cJSON_PrintUnformatted(handshake);
    if (handshake_str == NULL) {
        logMsg("FATAL: Error sending Handshake");
        exit(1);
    }
    printf("%s\n", handshake_str);
    fflush(stdout);
    free(handshake_str);
    cJSON_Delete(handshake);

    // CORE EVENTS: Core will send commands and events in STDIN of this process
    // in JSON format represented in BaseEvent struct

    // All RAM efficiency approaches below is unnecesary but recommended.
    // You can write plugin code as you wish on any programming language
    // until it matches the core requirements for validation. 

    // Use dynamic buffer for getline
    char *line = NULL;
    size_t len = 0;
    long read;

    // C does not have GC. We control memory allocation and free it
    // manually with avoiding leaks.

    while ((read = getline(&line, &len, stdin)) != -1) {
        // Удаляем символ переноса строки, если он есть
        if (read > 0 && line[read - 1] == '\n') {
            line[read - 1] = '\0';
            read--;
        }

        if (read == 0) {
            continue;
        }

        // Send RAW JSON data into your event processor
        handleEvent(line);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.
    }

    free(line);

    // Check why cycle is ended
    // In case of getline if cycle is ended that can be EOF or Error (проверяем feof/ferror)
    if (ferror(stdin)) {
        logMsg("Error reading from stdin");
    } else {
        // If scanner passes till this moment without errors then data flow is closed (EOF)
        logMsg("stdin closed by core. Shutdown.");
        exit(0);
    }
    return 0;
}

// handleEvent decompiles event and forward data for processing
void handleEvent(const char* raw_json) {
    cJSON *base = cJSON_Parse(raw_json);
    if (base == NULL) {
        logMsg("Error parsing base structure");
        return;
    }

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    cJSON *kill_item = cJSON_GetObjectItemCaseSensitive(base, "kill");
    if (cJSON_IsBool(kill_item) && kill_item->valueint) {
        logMsg("Got KILL signal. Shutdown.");
        cJSON_Delete(base);
        exit(0);
    }

    // Check proto ver
    cJSON *ver_item = cJSON_GetObjectItemCaseSensitive(base, "ver");
    int ver = cJSON_IsNumber(ver_item) ? ver_item->valueint : 0;
    if (ver != pjsonver && ver != 0) {
        logMsg("Warning: Event version (%d) does not match (%d)", ver, pjsonver);
    }

    cJSON *type_item = cJSON_GetObjectItemCaseSensitive(base, "type");
    const char *type = cJSON_IsString(type_item) ? type_item->valuestring : "";

    // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
    if (strcmp(type, "cmd") == 0) {
        cJSON *cfg = cJSON_GetObjectItemCaseSensitive(base, "cfg");
        if (cfg_save_last_entries == -1 && cfg != NULL) {
            cJSON *save_entries = cJSON_GetObjectItemCaseSensitive(cfg, "save_last_entries");
            cJSON *localfs_dir = cJSON_GetObjectItemCaseSensitive(cfg, "localfs_dir");
            
            if (cJSON_IsNumber(save_entries)) cfg_save_last_entries = save_entries->valueint;
            if (cJSON_IsString(localfs_dir)) snprintf(cfg_directory, sizeof(cfg_directory), "%s", localfs_dir->valuestring);
            
            logMsg("Configuration loaded. File limit: %d, subdir: ./%s", cfg_save_last_entries, cfg_directory);
        }

        // Load Directory
        cJSON *path_item = cJSON_GetObjectItemCaseSensitive(base, "path");
        const char *base_path = cJSON_IsString(path_item) ? path_item->valuestring : "";
        
        if (target_dir[0] == '\0' && base_path[0] != '\0' && cfg_directory[0] != '\0') {
            snprintf(target_dir, sizeof(target_dir), "%s/%s", base_path, cfg_directory);
            logMsg("Directory: %s", target_dir);
        }

    } else if (strcmp(type, "rkn") == 0) { // Event with possible diffs to process
        int save_limit = 100; // Safe default if config still not loaded
        if (cfg_save_last_entries > 0) {
            save_limit = cfg_save_last_entries;
        }

        // Pass JSON to event parser
        processRknEvent(raw_json, save_limit);

    } else {
        logMsg("Unknown event type: %s", type);
    }

    cJSON_Delete(base);
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
void processRknEvent(const char* raw_json, int save_limit) {
    cJSON *raw = cJSON_Parse(raw_json);
    if (raw == NULL) {
        logMsg("Error formatting JSON: failed to parse");
        return;
    }

    // Format single string JSON as JSON with spaces
    char *pretty = cJSON_Print(raw);
    cJSON_Delete(raw);
    
    if (pretty == NULL) {
        logMsg("Error formatting JSON");
        return;
    }

    // Check if directory exists
    create_directory_recursive(target_dir);

    // Save file with date and time
    time_t t = time(NULL);
    struct tm *tm_info = localtime(&t);
    char file_name[64];
    strftime(file_name, sizeof(file_name), "%Y-%m-%d_%H-%M-%S.txt", tm_info);

    char file_path[1024];
    snprintf(file_path, sizeof(file_path), "%s/%s", target_dir, file_name);

    FILE *f = fopen(file_path, "w");
    if (f == NULL) {
        logMsg("Error writing file %s", file_path);
        free(pretty);
        return;
    }

    fprintf(f, "%s\n", pretty);
    fclose(f);
    free(pretty);

    logMsg("Event written successfuly: %s", file_name);

    // Start file rotation end exit
    rotateFiles(save_limit);
}

// Struct for sorting files by modify time
typedef struct {
    char name[256];
    time_t mod_time;
} FileInfo;

int compareFiles(const void *a, const void *b) {
    return ((FileInfo*)a)->mod_time - ((FileInfo*)b)->mod_time;
}

// rotateFiles Checks limit of files in folder and removes oldest
void rotateFiles(int limit) {
    DIR *dir = opendir(target_dir);
    if (dir == NULL) {
        logMsg("Error reading directory contents for rotation");
        return;
    }

    // Collect only .txt files to not erase something else
    FileInfo *files = NULL;
    int count = 0;
    int capacity = 0;

    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if (entry->d_type == DT_REG) { // Only regular files
            // Check .txt extension
            char *ext = rchr(entry->d_name, '.');
            if (ext && strcmp(ext, ".txt") == 0) {
                char full_path[1024];
                snprintf(full_path, sizeof(full_path), "%s/%s", target_dir, entry->d_name);
                
                struct stat st;
                if (stat(full_path, &st) == 0) {
                    if (count >= capacity) {
                        capacity = capacity == 0 ? 16 : capacity * 2;
                        files = realloc(files, capacity * sizeof(FileInfo));
                    }
                    snprintf(files[count].name, sizeof(files[count].name), "%s", entry->d_name);
                    files[count].mod_time = st.st_mtime;
                    count++;
                }
            }
        }
    }
    closedir(dir);

    // If files less or equal to limit, doing nothing
    if (count <= limit) {
        free(files);
        return;
    }

    // Sort files by modified time (old to new)
    qsort(files, count, sizeof(FileInfo), compareFiles);

    // Count how many files to remove
    int files_to_delete = count - limit;

    // Remove N old files
    for (int i = 0; i < files_to_delete; i++) {
        char path[1024];
        snprintf(path, sizeof(path), "%s/%s", target_dir, files[i].name);
        if (unlink(path) != 0) {
            logMsg("Failed to remove old file %s", path);
        } else {
            logMsg("Cleanup: Removed %s", files[i].name);
        }
    }

    free(files);

    // End of task. If mode set to STREAM - continue listening from core
    if (strcmp(pmode, "ONCE") == 0) {
        // Kill process if task is done and mode "ONCE"
        exit(0);
    }
}

// Helper for mkdir -p
void create_directory_recursive(const char* dir) {
    char tmp[512];
    char *p = NULL;
    size_t len;

    snprintf(tmp, sizeof(tmp), "%s", dir);
    len = strlen(tmp);
    if (tmp[len - 1] == '/') tmp[len - 1] = 0;
    for (p = tmp + 1; *p; p++) {
        if (*p == '/') {
            *p = 0;
            mkdir(tmp, S_IRWXU | S_IRWXG | S_IROTH | S_IXOTH);
            *p = '/';
        }
    }
    mkdir(tmp, S_IRWXU | S_IRWXG | S_IROTH | S_IXOTH);
}