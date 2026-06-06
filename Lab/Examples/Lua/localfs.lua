local json = require("dkjson")
local lfs = require("lfs")

-- CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

-- Plugin internal specs
local PMODE = "ONCE" -- Can also be "STREAM"
local PJSONVER = 1   -- Expected JSON version
local PVER = "1.0.1@u11i51pi" -- You can specify everything you want. This value will show in core log when validating this plugin

local config = nil
local targetDir = "" -- Default directory

-- CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
-- If you will output logs into stdout they would simply be IGNORED!

-- logMsg writes logs as plain text into Stderr without any prefixes
-- Core logging prefixes and time itself
local function logMsg(formatStr, ...)
    if ... then
        io.stderr:write(string.format(formatStr, ...) .. "\n")
    else
        io.stderr:write(formatStr .. "\n")
    end
    io.stderr:flush()
end

-- Functions declaration (in Lua functions must be declared before using in cycle)
local handleEvent, processRknEvent, rotateFiles

function main()
    -- VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
    -- within 5 seconds after startup or core can throw timeout and not
    -- validate your plugin for event processing.

    -- Object to send handshake into stdout on start
    local handshake = {
        mode = PMODE,
        version = PVER,
        jsonver = PJSONVER
    }

    -- Send onelined JSON with \n at the end
    local handshakeJson, err = json.encode(handshake)
    if not handshakeJson then
        logMsg("FATAL: Error sending Handshake (%s)", err)
        os.exit(1)
    end
    io.stdout:write(handshakeJson .. "\n")
    io.stdout:flush()

    -- CORE EVENTS: Core will send commands and events in STDIN of this process
    -- in JSON format represented in BaseEvent struct

    -- All RAM efficiency approaches below is unnecesary but recommended.
    -- You can write plugin code as you wish on any programming language
    -- until it matches the core requirements for validation. 

    -- Per-line reading from stdin with io.lines() works as buffered scanner (like Scanner in Go)
    for line in io.lines() do
        -- Trim spaces
        local rawBytes = line:match("^%s*(.-)%s*$")
        if rawBytes ~= "" then
            -- Send RAW JSON data into your event processor
            handleEvent(rawBytes)
            -- If mode is "ONCE" the program will exit with code 0 inside handleEvent function
            -- And there is not so much need for RAM optimizations.

            -- If mode is "STREAM" the program will wait for next event.
        end
    end

    -- If scanner passes till this moment without errors then data flow is closed (EOF)
    logMsg("stdin closed by core. Shutdown.")
    os.exit(0)
end

-- handleEvent decompiles event and forward data for processing
function handleEvent(rawStr)
    local baseEvent, pos, err = json.decode(rawStr)
    if err then
        logMsg("Error parsing base structure: %s", err)
        return
    end

    -- Key validation requirement: If in event body "kill" is true then plugin must close its process

    -- Exit if "kill": true
    if baseEvent.kill == true then
        logMsg("Got KILL signal. Shutdown.")
        os.exit(0)
    end

    -- Check proto ver
    local ver = baseEvent.ver or 0
    if ver ~= PJSONVER and ver ~= 0 then
        logMsg("Warning: Event version (%s) does not match (%s)", ver, PJSONVER)
    end

    local eventType = baseEvent.type or ""

    if eventType == "cmd" then
        -- HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
        -- If config is still empty, and JSON has "cfg" then initialize config
        if config == nil and baseEvent.cfg then
            config = baseEvent.cfg
            logMsg("Configuration loaded. File limit: %s, subdir: ./%s", 
                config.save_last_entries or 0, config.localfs_dir or "")
        end

        -- Load Directory
        local basePath = baseEvent.path or ""
        local localDir = config and config.localfs_dir or ""
        if targetDir == "" and basePath ~= "" and localDir ~= "" then
            -- Paths conversion
            local sep = package.config:sub(1,1)
            targetDir = basePath:gsub("[%s" .. sep .. "]+$", "") .. sep .. localDir:gsub("^[%s" .. sep .. "]+", "")
            logMsg("Directory: %s", targetDir)
        end

    elseif eventType == "rkn" then -- Event with possible diffs to process
        local saveLimit = 100 -- Safe default if config still not loaded
        if config and config.save_last_entries and config.save_last_entries > 0 then
            saveLimit = config.save_last_entries
        end

        -- Pass JSON to event parser
        processRknEvent(rawStr, saveLimit)
    else
        logMsg("Unknown event type: %s", eventType)
    end
end

-- This plugin's task: Dump every "rkn" event as readable JSON on disk

-- processRknEvent saves RAW JSON into file with spacing and clean old files above limit
function processRknEvent(rawStr, saveLimit)
    local rawObj, _, err = json.decode(rawStr)
    if err then
        logMsg("Error formatting JSON: %s", err)
        return
    end

    -- Format single string JSON as JSON with spaces
    local pretty = json.encode(rawObj, { indent = true })

    -- Check if directory exists
    -- Check attributes of folder with lfs and create if not exist
    local attr = lfs.attributes(targetDir)
    if not attr then
        -- Create directory. lfs cannot mkdir -p we wait to basepath already created by core
        local success, mkdirErr = lfs.mkdir(targetDir)
        if not success then
            logMsg("Error creating directory %s: %s", targetDir, mkdirErr)
            return
        end
    end

    -- Save file with date and time
    local fileName = os.date("%Y-%m-%d_%H-%M-%S") .. ".txt"
    local sep = package.config:sub(1,1)
    local filePath = targetDir .. sep .. fileName

    local f, openErr = io.open(filePath, "w")
    if not f then
        logMsg("Error writing file %s: %s", filePath, openErr)
        return
    end

    f:write(pretty .. "\n")
    f:close()

    logMsg("Event written successfuly: %s", fileName)

    -- Start file rotation end exit
    rotateFiles(saveLimit)
end

-- rotateFiles Checks limit of files in folder and removes oldest
function rotateFiles(limit)
    local files = {}

    -- Iterating dirs with lfs
    for entry in lfs.dir(targetDir) do
        if entry ~= "." and entry ~= ".." then
            local sep = package.config:sub(1,1)
            local fullPath = targetDir .. sep .. entry
            local attr = lfs.attributes(fullPath)
            
            -- Collect only .txt files to not erase something else
            if attr and attr.mode == "file" and entry:match("%.txt$") then
                table.insert(files, {
                    path = fullPath,
                    name = entry,
                    mtime = attr.modification
                })
            end
        end
    end

    -- If files less or equal to limit, doing nothing
    if #files <= limit then
        return
    end

    -- Sort files by modified time (old to new)
    table.sort(files, function(a, b)
        return a.mtime < b.mtime
    end)

    -- Count how many files to remove
    local filesToDelete = #files - limit

    -- Remove N old files
    for i = 1, filesToDelete do
        local file = files[i]
        local success, removeErr = os.remove(file.path)
        if success then
            logMsg("Cleanup: Removed %s", file.name)
        else
            logMsg("Failed to remove old file %s: %s", file.path, removeErr)
        end
    end

    -- End of task. If mode set to STREAM - continue listening from core
    if PMODE == "ONCE" then
        -- Kill process if task is done and mode "ONCE"
        os.exit(0)
    end
end

main()