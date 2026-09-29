//go:build !windows

package cli

import "errors"

const canRepair = false

func rebuildDisk(string) error { return errors.ErrUnsupported }
