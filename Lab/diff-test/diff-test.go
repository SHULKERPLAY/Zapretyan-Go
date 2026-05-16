package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
)

// Reply JSON structure
type DiffResult struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

func main() {
	oldFile := "old.txt"
	newFile := "new.txt"

	// Read old file and write to frequency map
	// int needs if file has duplicate strings
	oldLines := make(map[string]int)
	
	fOld, err := os.Open(oldFile)
	if err != nil {
		log.Fatal(err)
	}
	defer fOld.Close()

	scannerOld := bufio.NewScanner(fOld)
	for scannerOld.Scan() {
		oldLines[scannerOld.Text()]++
	}

	result := DiffResult{
		Added:   []string{},
		Removed: []string{},
	}

	// Read new file and compare
	fNew, err := os.Open(newFile)
	if err != nil {
		log.Fatal(err)
	}
	defer fNew.Close()

	scannerNew := bufio.NewScanner(fNew)
	for scannerNew.Scan() {
		line := scannerNew.Text()
		
		if count, exists := oldLines[line]; exists && count > 0 {
			// If string in both lines - reduce counter
			oldLines[line]--
		} else {
			// String not existing in old file. Count as addition
			result.Added = append(result.Added, line)
		}
	}

	// All that left in oldLines with counter > 0 is deleted strings
	for line, count := range oldLines {
		for i := 0; i < count; i++ {
			result.Removed = append(result.Removed, line)
		}
	}

	// Sort srtings ascending
	sort.Strings(result.Added)
	sort.Strings(result.Removed)

	// Write in JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	// Output JSON to stdout
	fmt.Println(string(jsonData))
}