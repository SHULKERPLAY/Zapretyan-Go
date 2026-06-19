package util

import (
	"fmt"
	"os"
)

// logMsg writes logs as plain text into Stderr without any prefixes
func LogMsg(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}
