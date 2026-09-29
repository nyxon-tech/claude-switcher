package platform

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func desktopProcesses(Install) ([]Proc, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,ruid=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return desktopFromPS(string(out), ownerUID()), nil
}

func (d desktop) Quit() error {
	cmd := exec.Command("osascript", "-e", `tell application id "`+d.in.AppID+`" to quit`)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("quit claude desktop: %w", err)
	}
	go cmd.Wait() // a quit dialog can keep osascript waiting; Quit does not wait for it
	return nil
}

func (d desktop) ForceQuit() error {
	if err := signalDesktop(d.in, syscall.SIGTERM, true); err != nil {
		return err
	}
	if waitGone(d.in, 5*time.Second) == nil {
		return nil
	}
	if err := signalDesktop(d.in, syscall.SIGKILL, false); err != nil {
		return err
	}
	return waitGone(d.in, time.Second)
}

func (d desktop) Launch() error {
	err := exec.Command("open", "-g", "-b", d.in.AppID).Run()
	if err != nil && d.in.AppPath != "" {
		err = exec.Command("open", "-g", "-a", d.in.AppPath).Run()
	}
	if err != nil {
		return fmt.Errorf("start claude desktop: %w", err)
	}
	return nil
}

func (desktop) StopHelpers() {}
