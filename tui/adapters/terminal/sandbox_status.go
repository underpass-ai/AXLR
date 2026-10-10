package terminal

// SandboxStatus is what the launch found of exec_sandbox: its configured
// mode (off, auto or required) and, when that mode wants confinement but
// this machine cannot give it, why.
type SandboxStatus struct {
	Mode        string
	Unavailable string
}

// Off reports a configured sandbox that does not confine local commands:
// under auto they run unconfined, under required they are refused. On 10
// October 2026 only stderr said so, before the alternate screen hid it, and
// nothing in the console did.
func (s SandboxStatus) Off() bool {
	return s.Mode != "" && s.Mode != "off" && s.Unavailable != ""
}

// sandboxInfo is the paragraph Info and help give an inactive sandbox: what
// happens to local commands, and the cause the launch's probe reported.
func (s SandboxStatus) sandboxInfo(theme Theme) string {
	if !s.Off() {
		return ""
	}
	key := "sandbox.offAuto"
	if s.Mode == "required" {
		key = "sandbox.offRequired"
	}
	return theme.Tf(key, s.Mode, singleLine(s.Unavailable))
}
