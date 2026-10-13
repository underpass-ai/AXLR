package kmpstore

import (
	"io"
	"os"
	"path/filepath"
)

// Exists reports whether a directory holds a KMP store: its format marker.
func Exists(path string) bool {
	info, err := os.Stat(filepath.Join(path, "FORMAT_VERSION"))
	return err == nil && info.Mode().IsRegular()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
