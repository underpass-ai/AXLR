package main

// The console's tests run without the setup assistant; its own tests drive
// the wizard directly.
func init() { setupTerminal = func() bool { return false } }
