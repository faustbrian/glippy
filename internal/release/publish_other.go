//go:build !darwin && !linux

package release

import "errors"

func publishOutput(_, _ string) error {
	return errors.New("atomic release publication is supported only on Darwin and Linux")
}
