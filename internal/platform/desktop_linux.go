package platform

// Linux support is beta: the official build is new and its process layout is not verified.

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func desktopProcesses(Install) ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	uid := ownerUID()
	var ps []Proc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() { // our own path may contain "claude-desktop"
			continue
		}
		dir := "/proc/" + e.Name()
		exe, _ := os.Readlink(dir + "/exe") // unreadable for other users' and sandboxed processes
		cmdline, _ := os.ReadFile(dir + "/cmdline")
		args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		if !isLinuxDesktop(exe, args) {
			continue
		}
		if status, _ := os.ReadFile(dir + "/status"); uid >= 0 && statusUID(string(status)) != uid {
			continue
		}
		stat, _ := os.ReadFile(dir + "/stat")
		ps = append(ps, Proc{PID: pid, PPID: statPPID(string(stat)), Exe: cmp.Or(exe, args[0])})
	}
	markMain(ps)
	return ps, nil
}

func (d desktop) Quit() error { return signalDesktop(d.in, syscall.SIGTERM, true) }

func (d desktop) ForceQuit() error {
	if err := signalDesktop(d.in, syscall.SIGKILL, false); err != nil {
		return err
	}
	return waitGone(d.in, 5*time.Second)
}

func (d desktop) Launch() error {
	if d.in.AppPath == "" {
		return errNoApp
	}
	cmd := exec.Command(d.in.AppPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // its own session, so it outlives our terminal
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude desktop: %w", err)
	}
	go cmd.Wait()
	return nil
}

func (desktop) StopHelpers() {}
