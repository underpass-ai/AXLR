package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"

	"github.com/underpass-ai/AXLR/tui/application"
)

// SkillSources reads the skills a workspace publishes, under
// <workspace>/.axlr/skills/<name>/, through os.Root so neither a path nor a
// symlink reaches outside the workspace.
type SkillSources struct{}

var _ application.SkillSourcePort = SkillSources{}

var skillSourceName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)

// maxSkillSourceFiles bounds what a source directory may hold: the eight
// files of a skill, catalog.json and tests.json, with room to say which.
const maxSkillSourceFiles = 16

// ReadSkillSource implements application.SkillSourcePort: the regular
// files of the directory; a subdirectory or a link refuses it.
func (SkillSources) ReadSkillSource(ctx context.Context, workspace, name string) (map[string][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !skillSourceName.MatchString(name) {
		return nil, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	root, err := openWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir := path.Join(application.SkillSourceDir, name)
	entries, err := fs.ReadDir(root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, application.ErrNoSkillSource
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > maxSkillSourceFiles {
		return nil, fmt.Errorf("%s holds %d entries; a skill has at most eight files beside catalog.json and tests.json", dir, len(entries))
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("%s/%s is not a regular file; a skill's files are flat text files", dir, entry.Name())
		}
		file, err := root.Open(path.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, application.MaxSkillFileBytes+1))
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > application.MaxSkillFileBytes {
			return nil, fmt.Errorf("%s/%s exceeds 256 KiB", dir, entry.Name())
		}
		files[entry.Name()] = data
	}
	return files, nil
}
