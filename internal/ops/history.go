package ops

import (
	"context"
	"slices"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// History lists every recorded change, newest first (v2's included). Changes is len(Entries).
func (e *Env) History() ([]store.Journal, error) { return e.Vault.Journals() }

// Undo reverses the newest change that is not undone yet and returns it.
func (e *Env) Undo() (store.Journal, error) {
	js, err := e.Vault.Journals()
	if err != nil {
		return store.Journal{}, err
	}
	i := slices.IndexFunc(js, func(j store.Journal) bool { return !j.Undone })
	if i < 0 {
		return store.Journal{}, errf(NothingToUndo)
	}
	if err := e.closed(); err != nil {
		return store.Journal{}, err
	}
	return js[i], e.Vault.Undo(js[i])
}

// How often WaitClosed looks, and how long it lets Desktop finish writing once it is gone.
var (
	pollEvery = 500 * time.Millisecond
	grace     = 2 * time.Second
)

// WaitClosed returns once Desktop is closed, or when ctx is done. It only watches: the user
// quits Desktop, or the caller calls Desktop.Quit. After a wait it gives Desktop a moment to
// finish writing.
func (e *Env) WaitClosed(ctx context.Context) error {
	running, err := e.Desktop.Running()
	if err != nil || !running {
		return err
	}
	for running {
		if err := sleep(ctx, pollEvery); err != nil {
			return err
		}
		if running, err = e.Desktop.Running(); err != nil {
			return err
		}
	}
	return sleep(ctx, grace)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
