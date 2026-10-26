package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/underpass-ai/AXLR/tui/application"
)

// CatalogSkillStore keeps the catalog skills installed for this user, in
// Dir, owner-only: <catalog>/<name>/<version>/ holds the skill's files and
// skill.json, the record of the install with each file's digest. Every
// version stays beside the others until it is removed, so an execution
// pinned to one is not moved by the install of the next; every read checks
// the file against its recorded digest.
type CatalogSkillStore struct {
	Dir string
	mu  sync.Mutex
}

var _ application.InstalledSkillsPort = (*CatalogSkillStore)(nil)

const (
	skillRecordFile     = "skill.json"
	maxSkillRecordBytes = 64 << 10
	maxInstalledSkills  = 512
)

var (
	skillStoreCatalog = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	skillStoreName    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	skillStoreVersion = regexp.MustCompile(`^\d{1,9}\.\d{1,9}\.\d{1,9}$`)
	skillStoreFile    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
)

// NewCatalogSkillStore keeps the skills in dir, created owner-only.
func NewCatalogSkillStore(dir string) (*CatalogSkillStore, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("catalog skills directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &CatalogSkillStore{Dir: dir}, nil
}

func checkSkillKey(catalog, name, version string) error {
	if !skillStoreCatalog.MatchString(catalog) {
		return fmt.Errorf("catalog %q is not a catalog name", catalog)
	}
	if !skillStoreName.MatchString(name) {
		return fmt.Errorf("skill %q is not a skill name", name)
	}
	if version != "" && !skillStoreVersion.MatchString(version) {
		return fmt.Errorf("version %q is not major.minor.patch", version)
	}
	return nil
}

func checkSkillFile(file string) error {
	if !skillStoreFile.MatchString(file) || file == "." || file == ".." || file == skillRecordFile || strings.HasPrefix(file, ".") {
		return fmt.Errorf("skill file %q is not a file of the skill", file)
	}
	return nil
}

// List implements application.InstalledSkillsPort.
func (s *CatalogSkillStore) List(ctx context.Context) ([]application.InstalledSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(ctx)
}

func (s *CatalogSkillStore) list(ctx context.Context) ([]application.InstalledSkill, error) {
	var skills []application.InstalledSkill
	catalogs, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, catalog := range catalogs {
		if !catalog.IsDir() || !skillStoreCatalog.MatchString(catalog.Name()) {
			continue
		}
		names, err := os.ReadDir(filepath.Join(s.Dir, catalog.Name()))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if !name.IsDir() || !skillStoreName.MatchString(name.Name()) {
				continue
			}
			versions, err := os.ReadDir(filepath.Join(s.Dir, catalog.Name(), name.Name()))
			if err != nil {
				return nil, err
			}
			for _, version := range versions {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if !version.IsDir() || !skillStoreVersion.MatchString(version.Name()) {
					continue
				}
				record, err := s.readRecord(catalog.Name(), name.Name(), version.Name())
				if err != nil {
					return nil, fmt.Errorf("installed skill %s/%s@%s: %w", catalog.Name(), name.Name(), version.Name(), err)
				}
				skills = append(skills, record)
				if len(skills) > maxInstalledSkills {
					return nil, fmt.Errorf("more than %d skill versions are installed; remove some", maxInstalledSkills)
				}
			}
		}
	}
	sort.Slice(skills, func(i, j int) bool {
		a, b := skills[i], skills[j]
		if a.Catalog != b.Catalog {
			return a.Catalog < b.Catalog
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return versionLess(a.Version, b.Version)
	})
	return skills, nil
}

// versionLess orders major.minor.patch numerically.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3 && i < len(as) && i < len(bs); i++ {
		an, _ := strconv.Atoi(as[i])
		bn, _ := strconv.Atoi(bs[i])
		if an != bn {
			return an < bn
		}
	}
	return a < b
}

func (s *CatalogSkillStore) versionDir(catalog, name, version string) string {
	return filepath.Join(s.Dir, catalog, name, version)
}

func (s *CatalogSkillStore) readRecord(catalog, name, version string) (application.InstalledSkill, error) {
	data, err := readPrivate(filepath.Join(s.versionDir(catalog, name, version), skillRecordFile), maxSkillRecordBytes)
	if err != nil {
		return application.InstalledSkill{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record application.InstalledSkill
	if err := decoder.Decode(&record); err != nil {
		return application.InstalledSkill{}, fmt.Errorf("its record is not one this console reads: %v", err)
	}
	if record.Catalog != catalog || record.Name != name || record.Version != version {
		return application.InstalledSkill{}, errors.New("its record names another skill")
	}
	if _, has := record.Files[application.SkillMainFile]; !has {
		return application.InstalledSkill{}, errors.New("its record lists no SKILL.md")
	}
	for file := range record.Files {
		if err := checkSkillFile(file); err != nil {
			return application.InstalledSkill{}, err
		}
	}
	return record, nil
}

// Install implements application.InstalledSkillsPort: the version's files
// and record are written in a fresh directory, which then replaces the
// version's, so a reader never sees half an install.
func (s *CatalogSkillStore) Install(ctx context.Context, skill application.InstalledSkill, files []application.SkillFile) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := checkSkillKey(skill.Catalog, skill.Name, skill.Version); err != nil {
		return false, err
	}
	if skill.Version == "" {
		return false, errors.New("an installed skill has a version")
	}
	if len(files) == 0 || len(files) != len(skill.Files) {
		return false, errors.New("the record and the files of the skill differ")
	}
	parent := filepath.Join(s.Dir, skill.Catalog, skill.Name)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return false, err
	}
	staging, err := os.MkdirTemp(parent, ".install-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(staging)
	for _, file := range files {
		if err := checkSkillFile(file.Path); err != nil {
			return false, err
		}
		want, listed := skill.Files[file.Path]
		if !listed || hexDigest(file.Content) != want {
			return false, fmt.Errorf("file %s does not match the skill's record", file.Path)
		}
		if err := os.WriteFile(filepath.Join(staging, file.Path), file.Content, 0o600); err != nil {
			return false, err
		}
	}
	record, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(staging, skillRecordFile), append(record, '\n'), 0o600); err != nil {
		return false, err
	}
	target := s.versionDir(skill.Catalog, skill.Name, skill.Version)
	replaced := false
	if _, err := os.Lstat(target); err == nil {
		replaced = true
		aside := staging + ".old"
		if err := os.Rename(target, aside); err != nil {
			return false, err
		}
		defer os.RemoveAll(aside)
	}
	if err := os.Rename(staging, target); err != nil {
		return false, err
	}
	return replaced, nil
}

// Remove implements application.InstalledSkillsPort.
func (s *CatalogSkillStore) Remove(ctx context.Context, catalog, name, version string) ([]application.InstalledSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkSkillKey(catalog, name, version); err != nil {
		return nil, err
	}
	installed, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var removed []application.InstalledSkill
	for _, skill := range installed {
		if skill.Catalog != catalog || skill.Name != name || (version != "" && skill.Version != version) {
			continue
		}
		if err := os.RemoveAll(s.versionDir(catalog, name, skill.Version)); err != nil {
			return removed, err
		}
		removed = append(removed, skill)
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("skill %s of catalog %s is not installed", name, catalog)
	}
	// The name's directory goes with its last version.
	_ = os.Remove(filepath.Join(s.Dir, catalog, name))
	return removed, nil
}

// Read implements application.InstalledSkillsPort.
func (s *CatalogSkillStore) Read(ctx context.Context, catalog, name, version, file string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkSkillKey(catalog, name, version); err != nil {
		return nil, err
	}
	if err := checkSkillFile(file); err != nil {
		return nil, err
	}
	record, err := s.readRecord(catalog, name, version)
	if err != nil {
		return nil, err
	}
	want, listed := record.Files[file]
	if !listed {
		names := make([]string, 0, len(record.Files))
		for listedName := range record.Files {
			names = append(names, listedName)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("skill %s %s has no file %s; its files are %s", name, version, file, strings.Join(names, ", "))
	}
	data, err := readPrivate(filepath.Join(s.versionDir(catalog, name, version), file), application.MaxSkillFileBytes)
	if err != nil {
		return nil, err
	}
	if hexDigest(data) != want {
		return nil, fmt.Errorf("file %s of skill %s %s changed since it was installed; install it again", file, name, version)
	}
	return data, nil
}

// readPrivate reads a regular file, not a link, of at most limit bytes.
func readPrivate(path string, limit int64) ([]byte, error) {
	file, err := openNoFollow(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), limit)
	}
	return data, nil
}

func hexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
