package platform

import (
	"errors"
	"os"
	"time"
)

// Proc is one running Desktop process.
type Proc struct {
	PID, PPID int
	Exe       string // full image path, or only the image name when the path cannot be read
	Main      bool   // the app itself rather than one of its helpers
}

// DesktopProcesses lists the running processes of Desktop: the app, its helpers and, on
// Windows, the Cowork VM worker. A Custom install has none.
func DesktopProcesses(in Install) ([]Proc, error) {
	if in.Kind == Custom {
		return nil, nil
	}
	return desktopProcesses(in)
}

func newDesktop(in Install) Desktop {
	if in.Kind == Custom {
		return nopDesktop{}
	}
	return desktop{in}
}

// desktop controls a real installation. Quit, ForceQuit, Launch and StopHelpers are per OS.
type desktop struct{ in Install }

func (d desktop) Running() (bool, error) {
	ps, err := desktopProcesses(d.in)
	return len(ps) > 0, err
}

// nopDesktop is used for Custom installs (a --data-dir folder): nothing runs, nothing launches.
type nopDesktop struct{}

func (nopDesktop) Running() (bool, error) { return false, nil }
func (nopDesktop) Quit() error            { return nil }
func (nopDesktop) ForceQuit() error       { return nil }
func (nopDesktop) Launch() error          { return nil }
func (nopDesktop) StopHelpers()           {}

var (
	errStillRunning = errors.New("claude desktop is still running")
	errNoApp        = errors.New("claude desktop app not found")
)

// waitGone polls until no Desktop process is left, or returns errStillRunning after timeout.
func waitGone(in Install, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ps, err := desktopProcesses(in)
		if err != nil || len(ps) == 0 {
			return err
		}
		if time.Now().After(deadline) {
			return errStillRunning
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// markMain marks the processes whose parent is not itself a Desktop process: the app, not the
// helpers it started.
func markMain(ps []Proc) {
	pids := make(map[int]bool, len(ps))
	for _, p := range ps {
		pids[p.PID] = true
	}
	for i := range ps {
		ps[i].Main = !pids[ps[i].PPID]
	}
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
