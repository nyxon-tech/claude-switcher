package ops

import (
	"os"
	"path/filepath"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
)

// platformChecks look for a leftover standalone data folder and a broken Cowork VM.
func platformChecks(e *Env) []Check {
	var out []Check
	// The MSIX app falls back to the real %APPDATA% for files missing from its own copy, so a
	// login left there by the older installer can come back.
	if old := filepath.Join(os.Getenv("APPDATA"), "Claude"); e.Install.Kind == platform.MSIX && isDir(old) {
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.leftover"),
			Detail: i18n.T("ops.doctor.leftover.detail", "path", old), Fix: i18n.T("ops.doctor.leftover.fix")})
	}
	// Cowork's Hyper-V VM fails with "HCS operation failed" when this disk is missing.
	bundle := filepath.Join(e.Install.DataDir, "vm_bundles", "claudevm.bundle")
	disk := filepath.Join(bundle, "sessiondata.vhdx")
	if _, err := os.Stat(disk); isDir(bundle) && err != nil {
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.cowork"),
			Detail: i18n.T("ops.doctor.cowork.detail"), Fix: i18n.T("ops.doctor.cowork.fix", "path", disk)})
	}
	return out
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
