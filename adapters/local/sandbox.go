package local

import "github.com/underpass-ai/AXLR/domain"

// Sandbox confines the processes ProcessAdapter starts. Without it a local
// process runs with the person's full rights: in full autonomy the model's
// commands could write anywhere in $HOME, read keys and reach the network.
// With it, the filesystem outside the workspace is read-only except the
// Writable paths and a private /tmp, and Network false cuts the process off
// the network. It runs under bubblewrap (Program) on Linux.
type Sandbox struct {
	// Program is the absolute path of bwrap.
	Program string
	// Network keeps the host's network; false gives the process a network
	// namespace of its own, with nothing in it.
	Network bool
	// Writable are absolute paths outside the workspace the process may
	// write, such as a build cache.
	Writable []string
	// Unavailable, when set, says why a required sandbox cannot run here;
	// every process is then refused with it instead of running unconfined.
	Unavailable string
}

// refusal is why no process may start: the sandbox is required but
// unavailable here.
func (s *Sandbox) refusal() error {
	if s.Unavailable == "" {
		return nil
	}
	return domain.Reject("sandbox_unavailable", "exec_sandbox is required but unavailable: "+s.Unavailable)
}
