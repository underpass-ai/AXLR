//go:build !windows

package provider

import "syscall"

// The POSIX codes are matched through syscall's own errors.
var refusedErrnos, resetErrnos, unreachableErrnos []syscall.Errno
