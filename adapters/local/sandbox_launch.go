package local

import "os"

// launch is how a sandbox starts one program: the command to run instead,
// the variables it adds to the environment, the files it passes from fd 3
// on, and finish, called once the process has ended, which releases what
// the launch holds and returns a note for the command's stderr ("" for
// none). stopped says the process was killed, by a timeout or a
// cancellation, rather than left to exit.
type launch struct {
	program string
	args    []string
	env     []string
	files   []*os.File
	finish  func(stopped bool) string
}

// done is a finish with nothing to release.
func done(bool) string { return "" }
