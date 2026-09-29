package platform

import (
	"cmp"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func desktopProcesses(in Install) ([]Proc, error) {
	all, err := processes()
	if err != nil {
		return nil, err
	}
	ps := appProcesses(all)
	// The Cowork VM runs in a Hyper-V worker that holds files in Desktop's data folder. Every
	// Hyper-V VM (WSL2, Docker) has one too, so it only counts when this install has the VM.
	if exists(filepath.Join(in.DataDir, "vm_bundles", "claudevm.bundle")) {
		ps = append(ps, named(all, "vmwp.exe")...)
	}
	return ps, nil
}

// Quit cannot be done from outside: closing the window leaves Desktop running in the tray.
func (desktop) Quit() error { return ErrManualQuit }

func (d desktop) ForceQuit() error {
	all, err := processes()
	if err != nil {
		return err
	}
	ps := appProcesses(all)
	// The app first, so it cannot restart the helpers ended after it.
	for _, main := range []bool{true, false} {
		for _, p := range ps {
			if p.Main == main {
				terminate(p)
			}
		}
	}
	return waitGone(d.in, 5*time.Second)
}

func (d desktop) Launch() error {
	switch {
	case d.in.Kind == MSIX && d.in.AppID != "": // an empty AppID would open the Apps folder
		return startDetached("explorer.exe", `shell:AppsFolder\`+d.in.AppID)
	case d.in.Kind != MSIX && d.in.AppPath != "":
		return startDetached(d.in.AppPath)
	}
	return errNoApp
}

func (desktop) StopHelpers() {
	all, err := processes()
	if err != nil {
		return
	}
	for _, p := range named(all, "chrome-native-host.exe") {
		if strings.Contains(strings.ToLower(p.Exe), "claude") {
			terminate(p)
		}
	}
}

// startDetached starts a program with no console and, when the job we run in allows it,
// outside that job, so it keeps running after we exit.
func startDetached(name string, args ...string) error {
	const detached = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP
	var err error
	for _, flags := range []uint32{detached | windows.CREATE_BREAKAWAY_FROM_JOB, detached} {
		cmd := exec.Command(name, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		if err = cmd.Start(); err == nil {
			return cmd.Process.Release()
		}
	}
	return fmt.Errorf("start claude desktop: %w", err)
}

// appProcesses returns Desktop and its helpers, leaving out Claude Code workers and the CLI.
func appProcesses(all []Proc) []Proc {
	ps := slices.DeleteFunc(named(all, "claude.exe"), func(p Proc) bool { return !isWindowsDesktopExe(p.Exe) })
	markMain(ps)
	return ps
}

// named returns the processes with an image name, their full path filled in when readable.
func named(all []Proc, name string) []Proc {
	var ps []Proc
	for _, p := range all {
		if strings.EqualFold(p.Exe, name) {
			p.Exe = cmp.Or(imagePath(p.PID), p.Exe)
			ps = append(ps, p)
		}
	}
	return ps
}

// processes lists every running process with Exe holding only its image name.
func processes() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	defer windows.CloseHandle(snap)
	var ps []Proc
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		ps = append(ps, Proc{PID: int(e.ProcessID), PPID: int(e.ParentProcessID), Exe: windows.UTF16ToString(e.ExeFile[:])})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return ps, nil
}

// imagePath returns a process's full image path, or "" when it cannot be read.
func imagePath(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	return handlePath(h)
}

func handlePath(h windows.Handle) string {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// terminate ends p unless its PID now belongs to another program. Errors are left to the
// caller's next process listing.
func terminate(p Proc) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.PID))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if strings.EqualFold(handlePath(h), p.Exe) {
		_ = windows.TerminateProcess(h, 1)
	}
}
