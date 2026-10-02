package storage

import "os"

// writeSidecar atomically replaces one private metadata file beside a snapshot.
func (s *SessionStore) writeSidecar(pattern, target string, data []byte) error {
	file, err := os.CreateTemp(s.dir, pattern)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return s.replace(file.Name(), target)
}
