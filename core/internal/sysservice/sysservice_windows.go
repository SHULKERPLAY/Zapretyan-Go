//go:build windows

package sysservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"zapretyan-go/internal/config"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

func getPrivilegeError() error {
	return errors.New("error: Administrator rights required. Start console as Administrator")
}

// Check Admin rights in windows through security token
func checkPrivileges() (bool, error) {
	var sid *windows.SID
	// Use well known Admins SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false, err
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	return token.IsMember(sid)
}

func installService() error {
	binPath, _, err := getBinaryInfo()
	if err != nil {
		return err
	}

	// Protection from spaces in path.
	// The --run arg letting app know that it executed as Windows svc.
	formattedBinPath := fmt.Sprintf(`"%s" --run`, binPath)

	// Force uninstall old service if it stuck or not existing
	slog.Info("Uninstall old service if exist...")
	_ = uninstallService()

	// Create service
	// NOTE: sc.exe requires space after "=" (e.g. binPath= )
	cmd := exec.Command("sc.exe", "create", ServiceName,
		"binPath=", formattedBinPath,
		"start=", "auto",
		"depend=", "Tcpip", // We have network stack dependency
		"DisplayName=", "Zapretyan-Go Core Daemon",
	)
	// Hide window of SC if started from GUI
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error creating windows service: %w", err)
	}
	slog.Info("Registrated winodows service", "name", ServiceName, "path", formattedBinPath)


	// Setup restart logic on crash (as Restart in Linux)
	// Restart after 60000ms
	slog.Info("Setup restart params")
	cmdFailure := exec.Command("sc.exe", "failure", ServiceName, "reset=", "1000", "actions=", "restart/60000")
	cmdFailure.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmdFailure.Run()

	// Start service
	slog.Info("Starting service...")
	cmdStart := exec.Command("sc.exe", "start", ServiceName)
	cmdStart.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmdStart.Run()
	
	slog.Info("To uninstall service in the future use --uninstall flag. Do not move this executable anywhere while service is installed. If you already moved executable just use --install again.")

	return nil
}

func uninstallService() error {
	// Stop service
	slog.Info("Stopping service...")
	cmdStop := exec.Command("sc.exe", "stop", ServiceName)
	cmdStop.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmdStop.Run()

	// Remove record from registry of system services
	slog.Info("Removing service from windows registry...")
	cmdDel := exec.Command("sc.exe", "delete", ServiceName)
	cmdDel.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmdDel.Run(); err != nil {
		// If service not exist
		if strings.Contains(err.Error(), "1060") {
			slog.Warn("Service not existing!")
			return nil
		}
		return err
	}
	slog.Info("Service uninstalled successfuly")
	return nil
}

func runService(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer slog.Debug("sysservice.win.runService() ended")
	// Initialize service struct passing context and waitgroup inside
	s := &myService{
		ctx:    ctx,
		cancel: cancel,
		wg: wg,
	}

	// Start windows svc manager (Blocking operation)
	err := svc.Run("ZapretyanGo", s)
	if err != nil {
		slog.Error("Critical service error", "err", err)
		wg.Done() // Reduce counter on error as winsvc handler stopped work
	}
}

// Service structure with cancel function and waitgroup pointer
type myService struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg 	   *sync.WaitGroup
}

func (m *myService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Tell windows that service start is pending
	// Also set CheckPoint and WaitHint (Wait 5 секунд until next update)
	changes <- svc.Status{
		State:      svc.StartPending,
		CheckPoint: 1,
		WaitHint:   5000,
	}

	// Start loop to wait until all plugins is ready (config.Params.ExtReady)
	// Executing in same flow but sometimes send keepalive to windows to not kill the service on 30s timeout
	checkTick := time.NewTicker(500 * time.Millisecond)
	defer checkTick.Stop()

	checkpoint := uint32(1)

	for !config.Params.ExtReady {
		select {
		case <-checkTick.C:
			// Every 500ms increasing checkpoint showing Windows that app is not stuck and working
			checkpoint++
			changes <- svc.Status{
				State:      svc.StartPending,
				CheckPoint: checkpoint,
				WaitHint:   5000,
			}
		case req := <-r:
			// If user stopped service before it was started
			if req.Cmd == svc.Stop || req.Cmd == svc.Shutdown {
				changes <- svc.Status{State: svc.StopPending}
				m.cancel()  // Call context cancel for entire app
				m.wg.Done() // Ready to shutdown our windows listener
				m.wg.Wait() // Waiting to all goroutines to end
				changes <- svc.Status{State: svc.Stopped}
				os.Exit(0)
			}
		}
	}

	// When out of starting loop show windows that service is running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Shutdown wait loop
	for {
		select {
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				changes <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				// Service stopped by msc or taskmanager
				changes <- svc.Status{State: svc.StopPending}
				slog.Warn("GOT SHUTDOWN FROM WINDOWS SVC!")

				// Call context cancel for entire app to shutdown
				slog.Info("Starting core Shutdown. Notify modules...")
				m.cancel() // Send shutdown with ctx.Done() in all functions

				// Create channel for strict shutdown timeout
				shutdownDone := make(chan struct{})
				go func() {
					m.wg.Done() // Ready to shutdown our windows listener
					m.wg.Wait() // Wait until all goroutines call wg.Done()
					close(shutdownDone)
				}()

				select {
				case <-shutdownDone:
					slog.Info("All modules successfuly stopped!")
					changes <- svc.Status{State: svc.Stopped} // Tell windows that service has stopped
				case <-time.After(10 * time.Second):
					slog.Warn("TIMEOUT! Some functions did not stopped in time. Force shutdown.")
					changes <- svc.Status{State: svc.Stopped}
					os.Exit(0)
				}
			}
		case <-m.ctx.Done():
			// If app is closing from inside (Context canceled from another function)
			changes <- svc.Status{State: svc.StopPending}
			m.wg.Done() // Ready to shutdown our windows listener
			changes <- svc.Status{State: svc.Stopped}
			return
		}
	}
}
