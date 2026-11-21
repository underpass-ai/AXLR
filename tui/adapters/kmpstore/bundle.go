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
	abouts, err := bundleAbouts(filepath.Join(topLevel, filepath.FromSlash(BundlePath)))
	if err != nil {
		return "", err
	}
	var projects []string
	for _, about := range abouts {
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

// bundleAbouts lists the abouts a bundle's header names; none when the
// file does not exist.
func bundleAbouts(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	line, err := readLine(file)
	if err != nil {
		return nil, err
	}
	var header struct {
		Abouts []string `json:"abouts"`
	}
	if err := json.Unmarshal(line, &header); err != nil {
		return nil, fmt.Errorf("unreadable bundle header: %w", err)
	}
	return header.Abouts, nil
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
