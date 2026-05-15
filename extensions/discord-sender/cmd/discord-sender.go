package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Payload struct {
	Message string `json:"message"`
}

type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

func main() {
	// Announce extension mode
	mode := "STREAM"

	// Callback Object
	response := Handshake{
		Mode:    mode,
		Version: "1.0.0",
	}

	// Send object in stdout as JSON
	json.NewEncoder(os.Stdout).Encode(response)

	decoder := json.NewDecoder(os.Stdin)

	if mode == "ONCE" {
		var p Payload
		if err := decoder.Decode(&p); err == nil {
			// Task completed
			os.WriteFile("log.txt", []byte(p.Message), 0644)
		}
		os.Exit(0)
	}

	if mode == "STREAM" {
		// Loop reading RPC channel
		for {
			var p Payload
			if err := decoder.Decode(&p); err != nil {
				// If channel closed (EOF), exit
				break
			}
			// Parse data without restarting process
			fmt.Fprintf(os.Stderr, "[plugin] Get in Stream: %s\n", p.Message)
		}
	}
}