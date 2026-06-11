package sysservice

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const ServiceName = "ZapretyanGo"

// Get absolute path to current binary and its workdir
func getBinaryInfo() (binPath string, workDir string, err error) {
	binPath, err = os.Executable()
	if err != nil {
		return "", "", err
	}
	// Go through all possible symlinks to get correct path
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return "", "", err
	}
	workDir = filepath.Dir(binPath)
	return binPath, workDir, nil
}

// Check rights and install service
func Install() error {
	hasPerm, err := checkPrivileges()
	if err != nil {
		return err
	}
	if !hasPerm {
		if runtime.GOOS == "windows" { 
			RunAsAdmin()
		}
		return getPrivilegeError()
	}
	return installService()
}

// Check rights and uninstall service
func Uninstall() error {
	hasPerm, err := checkPrivileges()
	if err != nil {
		return err
	}
	if !hasPerm {
		if runtime.GOOS == "windows" { 
			RunAsAdmin()
		}
		return getPrivilegeError()
	}
	return uninstallService()
}

// Exported function to enter service mode on any OS with --run flag
func Run(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer slog.Debug("sysservice.Run() ended")
	slog.Info("ENTERING SYSTEM SERVICE MODE")
	runService(ctx, cancel, wg)
}
