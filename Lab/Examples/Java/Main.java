import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.util.Arrays;
import java.util.Comparator;

// CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

public class Main {
    // Plugin internal specs
    private static final String PMODE = "ONCE"; // Can also be "STREAM"
    private static final int PJSONVER = 1;     // Expected JSON version
    private static final String PVER = "1.0.1@u11i51pi"; // You can specify everything you want. This value will show in core log when validating this plugin

    private static JsonNode config = null;
    private static String targetDir = ""; // Default directory
    private static final ObjectMapper mapper = new ObjectMapper();

    // CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
    // If you will output logs into stdout they would simply be IGNORED!

    // logMsg writes logs as plain text into Stderr without any prefixes
    // Core logging prefixes and time itself
    private static void logMsg(String format, Object... a) {
        System.err.printf(format + "\n", a);
    }

    public static void main(String[] args) {
        // Pretty JSON output
        mapper.enable(SerializationFeature.INDENT_OUTPUT);

        // VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
        // within 5 seconds after startup or core can throw timeout and not
        // validate your plugin for event processing.

        // Object to send handshake into stdout on start
        ObjectNode handshake = mapper.createObjectNode();
        handshake.put("mode", PMODE);
        handshake.put("version", PVER);
        handshake.put("jsonver", PJSONVER);

        // Send onelined JSON with \n at the end
        try {
            // Write in one line without formatting for handshake
            String handshakeJson = mapper.writeValueAsString(handshake);
            System.out.println(handshakeJson);
            System.out.flush();
        } catch (Exception err) {
            logMsg("FATAL: Error sending Handshake (%s)", err.getMessage());
            System.exit(1);
        }

        // CORE EVENTS: Core will send commands and events in STDIN of this process
        // in JSON format represented in BaseEvent struct

        // All RAM efficiency approaches below is unnecesary but recommended.
        // You can write plugin code as you wish on any programming language
        // until it matches the core requirements for validation. 

        try (BufferedReader reader = new BufferedReader(new InputStreamReader(System.in))) {
            String line;
            // Use BufferedReader (like Scanner in Go)
            while ((line = reader.readLine()) != null) {
                String rawBytes = line.trim();
                if (rawBytes.isEmpty()) {
                    continue;
                }

                // Send RAW JSON data into your event processor
                handleEvent(rawBytes);
                // If mode is "ONCE" the program will exit with code 0 inside handleEvent function
                // And there is not so much need for RAM optimizations.

                // If mode is "STREAM" the program will wait for next event.
            }
        } catch (Exception err) {
            logMsg("Error reading from stdin: %s", err.getMessage());
        }

        // If scanner passes till this moment without errors then data flow is closed (EOF)
        logMsg("stdin closed by core. Shutdown.");
        System.exit(0);
    }

    // handleEvent decompiles event and forward data for processing
    private static void handleEvent(String rawStr) {
        JsonNode baseEvent;
        try {
            baseEvent = mapper.readTree(rawStr);
        } catch (Exception err) {
            logMsg("Error parsing base structure: %s", err.getMessage());
            return;
        }

        // Key validation requirement: If in event body "kill" is true then plugin must close its process

        // Exit if "kill": true
        if (baseEvent.has("kill") && baseEvent.get("kill").asBoolean()) {
            logMsg("Got KILL signal. Shutdown.");
            System.exit(0);
        }

        // Check proto ver
        int ver = baseEvent.has("ver") ? baseEvent.get("ver").asInt() : 0;
        if (ver != PJSONVER && ver != 0) {
            logMsg("Warning: Event version (%d) does not match (%d)", ver, PJSONVER);
        }

        String type = baseEvent.has("type") ? baseEvent.get("type").asText() : "";

        switch (type) {
            // HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
            case "cmd":
                // If config is still empty, and JSON has "cfg" then initialize config
                if (config == null && baseEvent.has("cfg") && !baseEvent.get("cfg").isNull()) {
                    config = baseEvent.get("cfg");
                    logMsg("Configuration loaded. File limit: %d, subdir: ./%s",
                            config.has("save_last_entries") ? config.get("save_last_entries").asInt() : 0,
                            config.has("localfs_dir") ? config.get("localfs_dir").asText() : "");
                }

                // Load Directory
                String basePath = baseEvent.has("path") ? baseEvent.get("path").asText() : "";
                String localDir = (config != null && config.has("localfs_dir")) ? config.get("localfs_dir").asText() : "";
                if (targetDir.isEmpty() && !basePath.isEmpty() && !localDir.isEmpty()) {
                    targetDir = Paths.get(basePath, localDir).toString();
                    logMsg("Directory: %s", targetDir);
                }
                break;

            case "rkn": // Event with possible diffs to process
                int saveLimit = 100; // Safe default if config still not loaded
                if (config != null && config.has("save_last_entries")) {
                    int limitFromCfg = config.get("save_last_entries").asInt();
                    if (limitFromCfg > 0) {
                        saveLimit = limitFromCfg;
                    }
                }

                // Pass JSON to event parser
                processRknEvent(rawStr, saveLimit);
                break;

            default:
                logMsg("Unknown event type: %s", type);
                break;
        }
    }

    // This plugin's task: Dump every "rkn" event as readable JSON on disk

    // processRknEvent saves RAW JSON into file with spacing and clean old files above limit
    private static void processRknEvent(String rawStr, int saveLimit) {
        String pretty;
        try {
            Object jsonObject = mapper.readValue(rawStr, Object.class);
            // Format single String JSON as JSON with spaces
            pretty = mapper.writerWithDefaultPrettyPrinter().writeValueAsString(jsonObject);
        } catch (Exception err) {
            logMsg("Error formatting JSON: %s", err.getMessage());
            return;
        }

        // Check if directory exists
        try {
            Path path = Paths.get(targetDir);
            if (!Files.exists(path)) {
                Files.createDirectories(path);
            }
        } catch (Exception err) {
            logMsg("Error creating directory %s: %s", targetDir, err.getMessage());
            return;
        }

        // Save file with date and time
        String fileName = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyy-MM-dd_HH-mm-ss")) + ".txt";
        Path filePath = Paths.get(targetDir, fileName);

        try {
            Files.writeString(filePath, pretty + "\n");
        } catch (Exception err) {
            logMsg("Error writing file %s: %s", filePath.toString(), err.getMessage());
            return;
        }

        logMsg("Event written successfuly: %s", fileName);

        // Start file rotation end exit
        rotateFiles(saveLimit);
    }

    // rotateFiles Checks limit of files in folder and removes oldest
    private static void rotateFiles(int limit) {
        File folder = new File(targetDir);
        // Collect only .txt files to not erase something else
        File[] files = folder.listFiles((dir, name) -> name.toLowerCase().endsWith(".txt") && new File(dir, name).isFile());

        if (files == null) {
            logMsg("Error reading directory contents for rotation.");
            return;
        }

        // If files less or equal to limit, doing nothing
        if (files.length <= limit) {
            return;
        }

        // Sort files by modified time (old to new)
        Arrays.sort(files, Comparator.comparingLong(File::lastModified));

        // Count how many files to remove
        int filesToDelete = files.length - limit;

        // Remove N old files
        for (int i = 0; i < filesToDelete; i++) {
            if (files[i].delete()) {
                logMsg("Cleanup: Removed %s", files[i].getName());
            } else {
                logMsg("Failed to remove old file %s", files[i].getAbsolutePath());
            }
        }

        // End of task. If mode set to STREAM - continue listening from core
        if ("ONCE".equals(PMODE)) {
            // Kill process if task is done and mode "ONCE"
            System.exit(0);
        }
    }
}