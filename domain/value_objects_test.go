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

func TestRelativePathRejectsTrailingSeparator(t *testing.T) {
	for _, path := range []string{"notes/", "notes//", "docs/notes/", "./"} {
		if _, err := NewRelativePath(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, path := range []string{"notes", "docs/notes.md", "./notes"} {
		if _, err := NewRelativePath(path); err != nil {
			t.Fatalf("rejected %q: %v", path, err)
		}
	}
	for path, want := range map[string]WorkingDirectory{"src/": "src", "src//": "src", "./": ".", "src": "src"} {
		if dir, err := NewWorkingDirectory(path); err != nil || dir != want {
			t.Fatalf("working directory %q: %q %v", path, dir, err)
		}
	}
	if _, err := NewWorkingDirectory("/"); err == nil {
		t.Fatal("accepted the filesystem root as a working directory")
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
