package domain

import "testing"

func TestWriteModeRejectsUnknownOperation(t *testing.T) {
	if _, err := NewWriteMode("append"); err == nil {
		t.Fatal("accepted unsupported write mode")
	}
	if mode, err := NewWriteMode("replace"); err != nil || mode != ReplaceMode {
		t.Fatalf("%q %v", mode, err)
	}
}

func TestArgvRejectsNULWithoutInterpretingShellCharacters(t *testing.T) {
	if _, err := NewArgv([]string{"a\x00b"}); err == nil {
		t.Fatal("accepted NUL")
	}
	args, err := NewArgv([]string{"a b", "$HOME"})
	if err != nil || len(args) != 2 || args[1] != "$HOME" {
		t.Fatalf("%v %v", args, err)
	}
}
