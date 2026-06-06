require 'json'
require 'fileutils'
require 'date'

# CREATED WITH HELP OF GEMINI 3.5 FLASH. USE WITH CAUTION

# Plugin internal specs
PMODE = "ONCE" # Can also be "STREAM"
PJSONVER = 1   # Expected JSON version
PVER = "1.0.1@u11i51pi" # You can specify everything you want. This value will show in core log when validating this plugin

$config = nil
$target_dir = "" # Default directory

# CORE MESSAGES: Core always listening your plugin logs on STDERR pipe.
# If you will output logs into stdout they would simply be IGNORED!

# logMsg writes logs as plain text into Stderr without any prefixes
# Core logging prefixes and time itself
def log_msg(format_str, *args)
  if args.empty?
    STDERR.puts(format_str)
  else
    STDERR.puts(sprintf(format_str, *args))
  end
  STDERR.flush
end

def main
  # VALIDATING: No matter what, your plugin must output its data JSON in STDOUT
  # within 5 seconds after startup or core can throw timeout and not
  # validate your plugin for event processing.

  # Object to send handshake into stdout on start
  handshake = {
    mode: PMODE,
    version: PVER,
    jsonver: PJSONVER
  }

  # Send onelined JSON with \n at the end
  begin
    puts handshake.to_json
    STDOUT.flush
  rescue => e
    log_msg("FATAL: Error sending Handshake (%s)", e.message)
    exit(1)
  end

  # CORE EVENTS: Core will send commands and events in STDIN of this process
  # in JSON format represented in BaseEvent struct

  # All RAM efficiency approaches below is unnecesary but recommended.
  # You can write plugin code as you wish on any programming language
  # until it matches the core requirements for validation. 

  begin
    # Using each_line for sequention input processing (like Scanner in Go)
    STDIN.each_line do |line|
      raw_bytes = line.strip
      next if raw_bytes.empty?

      # Send RAW JSON data into your event processor
      handle_event(raw_bytes)
      # If mode is "ONCE" the program will exit with code 0 inside handleEvent function
      # And there is not so much need for RAM optimizations.

      # If mode is "STREAM" the program will wait for next event.
    end
  rescue => e
    log_msg("Error reading from stdin: %s", e.message)
    exit(1)
  end

  # If scanner passes till this moment without errors then data flow is closed (EOF)
  log_msg("stdin closed by core. Shutdown.")
  exit(0)
end

# handleEvent decompiles event and forward data for processing
def handle_event(raw_str)
  begin
    base_event = JSON.parse(raw_str)
  rescue => e
    log_msg("Error parsing base structure: %s", e.message)
    return
  end

  # Key validation requirement: If in event body "kill" is true then plugin must close its process

  # Exit if "kill": true
  if base_event['kill'] == true
    log_msg("Got KILL signal. Shutdown.")
    exit(0)
  end

  # Check proto ver
  ver = base_event['ver'] || 0
  if ver != PJSONVER && ver != 0
    log_msg("Warning: Event version (%d) does not match (%d)", ver, PJSONVER)
  end

  type = base_event['type'] || ""

  case type
  # HINT: Usually if core sends "type":"cmd" with "kill":false then core just sent config data to plugin
  when "cmd"
    # If config is still empty, and JSON has "cfg" then initialize config
    if $config.nil? && base_event['cfg']
      $config = base_event['cfg']
      log_msg("Configuration loaded. File limit: %d, subdir: ./%s", 
              $config['save_last_entries'] || 0, 
              $config['localfs_dir'] || "")
    end

    # Load Directory
    base_path = base_event['path'] || ""
    local_dir = $config ? ($config['localfs_dir'] || "") : ""
    if $target_dir.empty? && !base_path.empty? && !local_dir.empty?
      $target_dir = File.join(base_path, local_dir)
      log_msg("Directory: %s", $target_dir)
    end

  when "rkn" # Event with possible diffs to process
    save_limit = 100 # Safe default if config still not loaded
    if $config && $config['save_last_entries'] && $config['save_last_entries'] > 0
      save_limit = $config['save_last_entries']
    end

    # Pass JSON to event parser
    process_rkn_event(raw_str, save_limit)

  else
    log_msg("Unknown event type: %s", type)
  end
end

# This plugin's task: Dump every "rkn" event as readable JSON on disk

# processRknEvent saves RAW JSON into file with spacing and clean old files above limit
def process_rkn_event(raw_str, save_limit)
  begin
    raw_obj = JSON.parse(raw_str)
    # Format single string JSON as JSON with spaces
    pretty = JSON.pretty_generate(raw_obj)
  rescue => e
    log_msg("Error formatting JSON: %s", e.message)
    return
  end

  # Check if directory exists
  begin
    FileUtils.mkdir_p($target_dir) unless Dir.exist?($target_dir)
  rescue => e
    log_msg("Error creating directory %s: %s", $target_dir, e.message)
    return
  end

  # Save file with date and time
  file_name = Time.now.strftime("%Y-%m-%d_%H-%M-%S") + ".txt"
  file_path = File.join($target_dir, file_name)

  begin
    File.write(file_path, pretty + "\n")
  rescue => e
    log_msg("Error writing file %s: %s", file_path, e.message)
    return
  end

  log_msg("Event written successfuly: %s", file_name)

  # Start file rotation end exit
  rotate_files(save_limit)
end

# rotateFiles Checks limit of files in folder and removes oldest
def rotate_files(limit)
  unless Dir.exist?($target_dir)
    log_msg("Error reading directory contents for rotation: Directory does not exist")
    return
  end

  # Collect only .txt files to not erase something else
  files = []
  begin
    Dir.foreach($target_dir) do |entry|
      next if entry == '.' || entry == '..'
      
      file_path = File.join($target_dir, entry)
      if File.file?(file_path) && File.extname(entry) == '.txt'
        files << {
          path: file_path,
          name: entry,
          mtime: File.mtime(file_path)
        }
      end
    end
  rescue => e
    log_msg("Error reading directory contents for rotation: %s", e.message)
    return
  end

  # If files less or equal to limit, doing nothing
  return if files.length <= limit

  # Sort files by modified time (old to new)
  files.sort_by! { |f| f[:mtime] }

  # Count how many files to remove
  files_to_delete = files.length - limit

  # Remove N old files
  files_to_delete.times do |i|
    begin
      File.delete(files[i][:path])
      log_msg("Cleanup: Removed %s", files[i][:name])
    rescue => e
      log_msg("Failed to remove old file %s: %s", files[i][:path], e.message)
    end
  end

  # End of task. If mode set to STREAM - continue listening from core
  if PMODE == "ONCE"
    # Kill process if task is done and mode "ONCE"
    exit(0)
  end
end

# Start main app cycle
main if __FILE__ == $0