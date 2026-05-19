package main

import (
	"encoding/json"
	// "fmt"
	"os"
)

// Handshake is immidiate stdout output on start
type Handshake struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	JsonVer int    `json:"jsonver"`
}

func main() {
	// Announce extension mode
	mode := "STREAM"

	// Callback Object
	response := Handshake{
		Mode:    mode,
		Version: "1.0.0@36hbsug1",
		JsonVer: 1,
	}

	// Send object in stdout as JSON
	json.NewEncoder(os.Stdout).Encode(response)

	// decoder := json.NewDecoder(os.Stdin)

	if mode == "ONCE" {
		os.Exit(0)
	}

	if mode == "STREAM" {
		// Loop reading RPC channel
		for {
			// Parse data without restarting process
			// fmt.Fprintf(os.Stderr, "[plugin] Get in Stream: %s\n", p.Message)
		}
	}
}