//go:build darwin

package release

import "golang.org/x/sys/unix"

func publishOutput(source, target string) error {
	return unix.RenamexNp(source, target, unix.RENAME_EXCL)
}
