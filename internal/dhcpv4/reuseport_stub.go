//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package dhcpv4

import "errors"

func setReusePort(fd uintptr) error {
	return errors.New("reuseport unsupported")
}

func reusePortSupported() bool {
	return false
}
