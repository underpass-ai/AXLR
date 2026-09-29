package domain

type ExecCommand struct {
	Program     Program
	Args        Argv
	Cwd         WorkingDirectory
	Stdin       Text
	Timeout     Timeout
	OutputLimit ByteLimit
}
