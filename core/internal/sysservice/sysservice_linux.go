//go:build linux

package sysservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type initSystem string

const (
	sysSystemd  initSystem = "systemd"
	sysOpenRC   initSystem = "openrc"
	sysUpstart  initSystem = "upstart"
	sysSysVinit initSystem = "sysvinit"
	sysUnknown  initSystem = "unknown"
)

func getPrivilegeError() error {
	return errors.New("error: root access required. Restart app with 'sudo' or from root user")
}

func checkPrivileges() (bool, error) {
	return os.Getuid() == 0, nil
}

// Checks current init system from PID 1 analysys
func detectInitSystem() initSystem {
	exeLink, err := filepath.EvalSymlinks("/proc/1/exe")
	if err != nil {
		// Fallback checks on dir structures if /proc is blocked
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			return sysSystemd
		}
		if _, err := os.Stat("/run/openrc"); err == nil {
			return sysOpenRC
		}
		if _, err := os.Stat("/sbin/initctl"); err == nil {
			return sysUpstart
		}
		return sysSysVinit
	}

	// Check name
	base := filepath.Base(exeLink)
	if strings.Contains(base, "systemd") {
		return sysSystemd
	}
	if strings.Contains(base, "openrc") {
		return sysOpenRC
	}
	if strings.Contains(base, "upstart") {
		return sysUpstart
	}

	// If PID 1 has name 'init' then check the environment
	if base == "init" || base == "sh" {
		if _, err := os.Stat("/run/openrc"); err == nil {
			return sysOpenRC
		}
		if _, err := os.Stat("/sbin/initctl"); err == nil {
			return sysUpstart
		}
		return sysSysVinit
	}
	return sysUnknown
}

func installService() error {
	binPath, workDir, err := getBinaryInfo()
	if err != nil {
		return err
	}

	init := detectInitSystem()
	switch init {
	case sysSystemd:
		return installSystemd(binPath, workDir)
	case sysOpenRC:
		return installOpenRC(binPath, workDir)
	case sysUpstart:
		return installUpstart(binPath, workDir)
	case sysSysVinit:
		return installSysVinit(binPath, workDir)
	default:
		return fmt.Errorf("error: failed to recognise Linux init system (Unknown init)")
	}
}

func uninstallService() error {
	init := detectInitSystem()
	switch init {
	case sysSystemd:
		return uninstallSystemd()
	case sysOpenRC:
		return uninstallOpenRC()
	case sysUpstart:
		return uninstallUpstart()
	case sysSysVinit:
		return uninstallSysVinit()
	default:
		return fmt.Errorf("error: failed to recognise Linux init system on deletion")
	}
}

// ===============
// SYSTEMD INSTALL
// ===============

const systemdTemplate = `[Unit]
Description=Zapretyan-Go core service
After=network-online.target
StartLimitIntervalSec=1000
StartLimitBurst=10

[Service]
KillSignal=SIGINT
Restart=on-failure
RestartSec=60s
ExecStart="{{.BinPath}}" --run
WorkingDirectory={{.WorkDir}}

[Install]
WantedBy=multi-user.target
`

func installSystemd(binPath, workDir string) error {
	_ = uninstallSystemd() // Remove old service

	tmpl, _ := template.New("systemd").Parse(systemdTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{"BinPath": binPath, "WorkDir": workDir})

	unitPath := "/etc/systemd/system/" + strings.ToLower(ServiceName) + ".service"
	if err := os.WriteFile(unitPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", strings.ToLower(ServiceName)).Run()
	return exec.Command("systemctl", "start", strings.ToLower(ServiceName)).Run()
}

func uninstallSystemd() error {
	name := strings.ToLower(ServiceName)
	_ = exec.Command("systemctl", "stop", name).Run()
	_ = exec.Command("systemctl", "disable", name).Run()
	_ = os.Remove("/etc/systemd/system/" + name + ".service")
	return exec.Command("systemctl", "daemon-reload").Run()
}

// ==============
// OPENRC INSTALL
// ==============

const openrcTemplate = `#!/sbin/openrc-run
description="Zapretyan-Go core service"
supervisor="supervise-daemon"
command="{{.BinPath}}"
command_args="--run"
directory="{{.WorkDir}}"
respawn_delay=60
respawn_max=10

depend() {
    need net
    after firewall
}
`

func installOpenRC(binPath, workDir string) error {
	_ = uninstallOpenRC() // Remove old service

	tmpl, _ := template.New("openrc").Parse(openrcTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{"BinPath": binPath, "WorkDir": workDir})

	scriptPath := "/etc/init.d/" + strings.ToLower(ServiceName)
	if err := os.WriteFile(scriptPath, buf.Bytes(), 0755); err != nil { // RIGHTS 755 IS REQUIRED!
		return err
	}

	_ = exec.Command("rc-update", "add", strings.ToLower(ServiceName), "default").Run()
	return exec.Command(scriptPath, "start").Run()
}

func uninstallOpenRC() error {
	name := strings.ToLower(ServiceName)
	scriptPath := "/etc/init.d/" + name
	_ = exec.Command(scriptPath, "stop").Run()
	_ = exec.Command("rc-update", "del", name).Run()
	return os.Remove(scriptPath)
}

// ===============
// UPSTART INSTALL
// ===============

const upstartTemplate = `description "Zapretyan-Go core service"
start on filesystem and net-device-up IFACE!=lo
stop on runlevel [016]

respawn
respawn limit 10 60

script
    cd {{.WorkDir}}
    exec "{{.BinPath}}" --run
end script
`

func installUpstart(binPath, workDir string) error {
	_ = uninstallUpstart()

	tmpl, _ := template.New("upstart").Parse(upstartTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{"BinPath": binPath, "WorkDir": workDir})

	confPath := "/etc/init/" + strings.ToLower(ServiceName) + ".conf"
	if err := os.WriteFile(confPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	return exec.Command("initctl", "start", strings.ToLower(ServiceName)).Run()
}

func uninstallUpstart() error {
	name := strings.ToLower(ServiceName)
	_ = exec.Command("initctl", "stop", name).Run()
	return os.Remove("/etc/init/" + name + ".conf")
}

// ================
// SYSVINIT INSTALL
// ================

const sysvinitTemplate = `#!/bin/sh
### BEGIN INIT INFO
# Required-Start:    $network $local_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Description:       Zapretyan-Go core service
### END INIT INFO

NAME="{{.Name}}"
PIDFILE="/var/run/$NAME.pid"
COMMAND="{{.BinPath}}"
DIR="{{.WorkDir}}"

case "$1" in
  start)
    echo "Starting $NAME..."
    cd "$DIR"
    start-stop-daemon --start --background --make-pidfile --pidfile "$PIDFILE" --exec "$COMMAND" -- --run
    ;;
  stop)
    echo "Stopping $NAME..."
    start-stop-daemon --stop --pidfile "$PIDFILE"
    rm -f "$PIDFILE"
    ;;
  restart)
    $0 stop
    sleep 2
    $0 start
    ;;
  *)
    echo "Usage: $0 {start|stop|restart}"
    exit 1
esac
exit 0
`

func installSysVinit(binPath, workDir string) error {
	_ = uninstallSysVinit()

	tmpl, _ := template.New("sysv").Parse(sysvinitTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{
		"Name":    strings.ToLower(ServiceName),
		"BinPath": binPath,
		"WorkDir": workDir,
	})

	scriptPath := "/etc/init.d/" + strings.ToLower(ServiceName)
	if err := os.WriteFile(scriptPath, buf.Bytes(), 0755); err != nil {
		return err
	}

	// The registration command depends on distrib (Debian/Ubuntu uses update-rc.d, RedHat - chkconfig)
	if _, err := exec.LookPath("update-rc.d"); err == nil {
		_ = exec.Command("update-rc.d", strings.ToLower(ServiceName), "defaults").Run()
	} else if _, err := exec.LookPath("chkconfig"); err == nil {
		_ = exec.Command("chkconfig", "--add", strings.ToLower(ServiceName)).Run()
	}

	return exec.Command(scriptPath, "start").Run()
}

func uninstallSysVinit() error {
	name := strings.ToLower(ServiceName)
	scriptPath := "/etc/init.d/" + name
	_ = exec.Command(scriptPath, "stop").Run()

	if _, err := exec.LookPath("update-rc.d"); err == nil {
		_ = exec.Command("update-rc.d", "-f", name, "remove").Run()
	} else if _, err := exec.LookPath("chkconfig"); err == nil {
		_ = exec.Command("chkconfig", "--del", name).Run()
	}

	return os.Remove(scriptPath)
}

func runService(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	_ = ctx
	_ = cancel
	wg.Done()
} // No additional actions required for linux build
