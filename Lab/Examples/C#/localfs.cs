using System;
using System.IO;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Globalization;
using System.Collections.Generic;
using System.Linq;

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

// Plugin internal specs
const string pmode = "ONCE"; // Can also be "STREAM"
const int pjsonver = 1;      // Expected JSON version
const string pver = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

JsonNode? config = null;
string targetDir = ""; // Default directory

// CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
// If you will output logs into stdout they would simply be IGNORED!

// logMsg writes logs as plain text into Stderr without any prefixes
// Core logging prefixes and time itself
void LogMsg(string format, params object[] a)
{
    Console.Error.WriteLine(string.Format(CultureInfo.InvariantCulture, format, a));
}

// VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
// within 5 seconds after startup or core can throw timeout and not
// validate your plugin for event processing.

// Object to send handshake into stdout on start
var handshake = new
{
    mode = pmode,
    version = pver,
    jsonver = pjsonver
};

// Send onelined JSON with \n at the end
try
{
    string handshakeJson = JsonSerializer.Serialize(handshake);
    Console.WriteLine(handshakeJson);
}
catch (Exception err)
{
    LogMsg("FATAL: Error sending Handshake ({0})", err.Message);
    Environment.Exit(1);
}

// CORE EVENTS: Core will send commands and events in STDIN of this process
// in JSON format represented in BaseEvent struct

// All RAM efficiency approaches below is unnecesary but recommended.
// You can write plugin code as you wish on any programming language
// until it matches the core requirements for validation. 

try
{
    // Using StreamReader for per-line reading (as Scanner in Go)
    using var reader = new StreamReader(Console.OpenStandardInput());
    string? line;
    while ((line = reader.ReadLine()) != null)
    {
        string rawBytes = line.Trim();
        if (rawBytes.Length == 0)
        {
            continue;
        }

        // Send RAW JSON data into your event processor
        HandleEvent(rawBytes);
        // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
        // And there is not so much need for RAM optimizations.

        // If mode is "STREAM" the program will wait for next event.
    }
}
catch (Exception err)
{
    LogMsg("Error reading from stdin: {0}", err.Message);
}

// If scanner passes till this moment without errors then data flow is closed (EOF)
LogMsg("stdin closed by core. Shutdown.");
Environment.Exit(0);

// handleEvent decompiles event and forward data for processing
void HandleEvent(string rawStr)
{
    JsonNode? baseEvent;
    try
    {
        baseEvent = JsonNode.Parse(rawStr);
    }
    catch (Exception err)
    {
        LogMsg("Error parsing base structure: {0}", err.Message);
        return;
    }

    if (baseEvent == null) return;

    // Key validation requirement: If in event body "kill" is true then plugin must close its process

    // Exit if "kill": true
    if (baseEvent["kill"]?.GetValue<bool>() == true)
    {
        LogMsg("Got KILL signal. Shutdown.");
        Environment.Exit(0);
    }

    // Check proto ver
    int ver = baseEvent["ver"]?.GetValue<int>() ?? 0;
    if (ver != pjsonver && ver != 0)
    {
        LogMsg("Warning: Event version ({0}) does not match ({1})", ver, pjsonver);
    }

    string type = baseEvent["type"]?.GetValue<string>() ?? "";

    switch (type)
    {
        // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        case "cmd":
            // If config is still empty, and JSON has "cfg" then initialize config
            if (config == null && baseEvent["cfg"] != null)
            {
                config = baseEvent["cfg"];
                LogMsg("Configuration loaded. File limit: {0}, subdir: ./{1}", 
                    config["save_last_entries"]?.GetValue<int>() ?? 0, 
                    config["localfs_dir"]?.GetValue<string>() ?? "");
            }

            // Load Directory
            string basePath = baseEvent["path"]?.GetValue<string>() ?? "";
            string localDir = config?["localfs_dir"]?.GetValue<string>() ?? "";
            if (string.IsNullOrEmpty(targetDir) && !string.IsNullOrEmpty(basePath) && !string.IsNullOrEmpty(localDir))
            {
                targetDir = Path.Combine(basePath, localDir);
                LogMsg("Directory: {0}", targetDir);
            }
            break;

        case "rkn": // Event with possible diffs to process
            int saveLimit = 100; // Safe default if config still not loaded
            if (config != null)
            {
                int limitFromCfg = config["save_last_entries"]?.GetValue<int>() ?? 0;
                if (limitFromCfg > 0) saveLimit = limitFromCfg;
            }

            // Pass JSON to event parser
            ProcessRknEvent(rawStr, saveLimit);
            break;

        default:
            LogMsg("Unknown event type: {0}", type);
            break;
    }
}

// This plugin's task: Dump every "rkn" event as readable JSON on disk

// processRknEvent saves RAW JSON into file with spacing and clean old files above limit
void ProcessRknEvent(string rawStr, int saveLimit)
{
    string pretty;
    try
    {
        using var doc = JsonDocument.Parse(rawStr);
        // Format single string JSON as JSON with spaces
        pretty = JsonSerializer.Serialize(doc, new JsonSerializerOptions { WriteIndented = true });
    }
    catch (Exception err)
    {
        LogMsg("Error formatting JSON: {0}", err.Message);
        return;
    }

    // Check if directory exists
    try
    {
        if (!Directory.Exists(targetDir))
        {
            Directory.CreateDirectory(targetDir);
        }
    }
    catch (Exception err)
    {
        LogMsg("Error creating directory {0}: {1}", targetDir, err.Message);
        return;
    }

    // Save file with date and time
    string fileName = DateTime.Now.ToString("yyyy-MM-dd_HH-mm-ss") + ".txt";
    string filePath = Path.Combine(targetDir, fileName);

    try
    {
        File.WriteAllText(filePath, pretty + "\n");
    }
    catch (Exception err)
    {
        LogMsg("Error writing file {0}: {1}", filePath, err.Message);
        return;
    }

    LogMsg("Event written successfuly: {0}", fileName);

    // Start file rotation end exit
    RotateFiles(saveLimit);
}

// rotateFiles Checks limit of files in folder and removes oldest
void RotateFiles(int limit)
{
    FileInfo[] files;
    try
    {
        var dirInfo = new DirectoryInfo(targetDir);
        // Collect only .txt files to not erase something else
        files = dirInfo.GetFiles("*.txt");
    }
    catch (Exception err)
    {
        LogMsg("Error reading directory contents for rotation: {0}", err.Message);
        return;
    }

    // If files less or equal to limit, doing nothing
    if (files.Length <= limit)
    {
        return;
    }

    // Sort files by modified time (old to new)
    var sortedFiles = files.OrderBy(f => f.LastWriteTime).ToList();

    // Count how many files to remove
    int filesToDelete = sortedFiles.Count - limit;

    // Remove N old files
    for (int i = 0; i < filesToDelete; i++)
    {
        try
        {
            sortedFiles[i].Delete();
            LogMsg("Cleanup: Removed {0}", sortedFiles[i].Name);
        }
        catch (Exception err)
        {
            LogMsg("Failed to remove old file {0}: {1}", sortedFiles[i].FullName, err.Message);
        }
    }

    // End of task. If mode set to STREAM - continue listening from core
    if (pmode == "ONCE")
    {
        // Kill process if task is done and mode "ONCE"
        Environment.Exit(0);
    }
}