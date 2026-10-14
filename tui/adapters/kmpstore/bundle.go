package kmpstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BundleAbout is the project about a checkout's bundle names: its header
// lists the abouts it carries. One project about is taken as it is; among
// several, the one equal to the directory's name ignoring case; otherwise
// none, and the directory rule applies (decision D2).
func BundleAbout(topLevel string) (string, error) {
	if topLevel == "" {
		return "", nil
	}
	file, err := os.Open(filepath.Join(topLevel, filepath.FromSlash(BundlePath)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	line, err := readLine(file)
	if err != nil {
		return "", err
	}
	var header struct {
		Abouts []string `json:"abouts"`
	}
	if err := json.Unmarshal(line, &header); err != nil {
		return "", fmt.Errorf("unreadable bundle header: %w", err)
	}
	var projects []string
	for _, about := range header.Abouts {
		if strings.HasPrefix(about, "project:") {
			projects = append(projects, about)
		}
	}
	switch len(projects) {
	case 0:
		return "", nil
	case 1:
		return projects[0], nil
	}
	name := filepath.Base(topLevel)
	for _, about := range projects {
		if strings.EqualFold(strings.TrimPrefix(about, "project:"), name) {
			return about, nil
		}
	}
	return "", nil
}

// readLine reads the first line of a file, at most 64 KiB.
func readLine(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil {
		return nil, err
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[:i]
	}
	return bytes.TrimSpace(data), nil
}
