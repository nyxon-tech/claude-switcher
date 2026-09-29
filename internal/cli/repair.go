package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// repairCommand rebuilds the Cowork VM's disk, which only exists on Windows (Hyper-V).
func (a *app) repairCommand() *cobra.Command {
	cmd := a.forceQuitFlag(a.command("repair", "repair", "more", nargs(0, 0), a.repair))
	cmd.Hidden = !canRepair
	return cmd
}

// repair rebuilds <data>\vm_bundles\claudevm.bundle\sessiondata.vhdx: without it Cowork fails with
// "HCS operation failed".
func (a *app) repair(cmd *cobra.Command, _ []string) error {
	if !canRepair {
		return errText("cli.repair.windows-only")
	}
	env, err := a.open()
	if err != nil {
		return err
	}
	bundle := filepath.Join(env.Install.DataDir, "vm_bundles", "claudevm.bundle")
	disk := filepath.Join(bundle, "sessiondata.vhdx")
	switch {
	case !exists(bundle):
		return a.result(map[string]any{"repaired": false}, "cli.repair.no-vm")
	case exists(disk):
		return a.result(map[string]any{"repaired": false}, "cli.repair.fine")
	}
	a.warn(a.err, i18n.T("ops.doctor.cowork"))
	if err := a.confirm(cmd.Context(), i18n.T("cli.repair.confirm")); err != nil {
		return err
	}
	err = a.write(cmd.Context(), env, false, func() error {
		switch running, err := env.Desktop.Running(); {
		case err != nil:
			return err
		case running:
			return &ops.Error{Code: ops.DesktopRunning}
		}
		// A declined elevation prompt only shows as a failed diskpart, so the disk itself tells.
		if err := rebuildDisk(disk); err != nil || !exists(disk) {
			return &cliError{err: errText("cli.repair.failed"), hint: i18n.T("ops.doctor.cowork.fix", "path", disk)}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.result(map[string]any{"repaired": true, "disk": disk}, "cli.repair.done")
}
