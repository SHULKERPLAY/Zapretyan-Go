package cfgreader

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func preparePath(configPath string) string {
	// Convert slashes to current OS
	fixedPath := filepath.FromSlash(configPath)

	// If Windows add ".exe" file extension or ignore if alredy has ".exe"
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(strings.ToLower(fixedPath), ".exe") {
			fixedPath += ".exe"
		}
	}

	return fixedPath
}

func main() {
	// Path from TOML
	rawPath := "./extensions/discord"
	
	finalPath := preparePath(rawPath)
	fmt.Println("Start Path:", finalPath)
}