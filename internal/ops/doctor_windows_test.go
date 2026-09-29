package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

func TestDoctorFindsMissingCoworkDisk(t *testing.T) {
	f := newFixture(t)
	bundle := filepath.Join(f.data, "vm_bundles", "claudevm.bundle")
	must(t, os.MkdirAll(bundle, 0o700))
	if c, ok := find(f.Doctor(), i18n.T("ops.doctor.cowork")); !ok || c.Level != Warn {
		t.Fatal("a Cowork bundle without sessiondata.vhdx should warn")
	}
	f.put(filepath.Join(bundle, "sessiondata.vhdx"), "")
	if _, ok := find(f.Doctor(), i18n.T("ops.doctor.cowork")); ok {
		t.Fatal("a complete bundle should not warn")
	}
}
