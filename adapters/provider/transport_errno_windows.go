package provider

import "syscall"

// Winsock reports a refused, reset or unreachable connection with its own
// codes, which syscall.ECONNREFUSED and the others do not match.
var (
	refusedErrnos     = []syscall.Errno{10061}        // WSAECONNREFUSED
	resetErrnos       = []syscall.Errno{10054, 10053} // WSAECONNRESET, WSAECONNABORTED
	unreachableErrnos = []syscall.Errno{10065, 10051} // WSAEHOSTUNREACH, WSAENETUNREACH
)
