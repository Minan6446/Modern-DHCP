//go:build linux || darwin || freebsd || netbsd || openbsd

package dhcpv4

import "golang.org/x/sys/unix"

func setReusePort(fd uintptr) error {
	if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		return err
	}
	return unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
}

func reusePortSupported() bool {
	return true
}
