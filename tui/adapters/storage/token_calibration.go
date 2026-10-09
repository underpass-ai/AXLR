package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	tokenCalibrationVersion  = 1
	maxTokenCalibrationBytes = 256 << 10
	// maxCalibratedModels bounds the file; the model measured longest ago
	// goes first.
	maxCalibratedModels = 64
)

type tokenCalibrationFile struct {
	Version int                              `json:"version"`
	Models  map[string]tokenCalibrationEntry `json:"models"`
}

type tokenCalibrationEntry struct {
	BytesPerToken float64   `json:"bytes_per_token"`
	Samples       int       `json:"samples"`
	Updated       time.Time `json:"updated"`
}

// TokenCalibration keeps each model's measured bytes per prompt token in one
// private file beside the sessions, so a console starts from what earlier
// consoles measured, and consoles running together add to the same average.
// The ratio a console applies to a model is fixed the first time it is
// asked once the model is calibrated, so the projection's ceiling does not
// move under a running session and break the provider's prompt cache.
type TokenCalibration struct {
	path    string
	mu      sync.Mutex
	applied map[root.ModelID]int
	now     func() time.Time
}

var _ application.TokenCalibrationPort = (*TokenCalibration)(nil)

func NewTokenCalibration(path string) (*TokenCalibration, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("token calibration path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &TokenCalibration{path: path, applied: map[root.ModelID]int{}, now: time.Now}, nil
}

// Observe adds one request of model to its average.
func (c *TokenCalibration) Observe(ctx context.Context, model root.ModelID, requestBytes, promptTokens int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, c.path)
	if err != nil {
		return err
	}
	defer release()
	file, err := c.read()
	if err != nil {
		// A damaged file only loses what earlier consoles measured.
		file = tokenCalibrationFile{Version: tokenCalibrationVersion, Models: map[string]tokenCalibrationEntry{}}
	}
	entry := file.Models[string(model)]
	measured := domain.BytesPerToken{Ratio: entry.BytesPerToken, Samples: entry.Samples}.Observe(requestBytes, promptTokens)
	if measured.Samples == entry.Samples {
		return nil
	}
	file.Models[string(model)] = tokenCalibrationEntry{BytesPerToken: measured.Ratio, Samples: measured.Samples, Updated: c.now().UTC().Truncate(time.Second)}
	if len(file.Models) > maxCalibratedModels {
		ids := make([]string, 0, len(file.Models))
		for id := range file.Models {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return file.Models[ids[i]].Updated.Before(file.Models[ids[j]].Updated) })
		for _, id := range ids[:len(ids)-maxCalibratedModels] {
			delete(file.Models, id)
		}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(ctx, c.path, ".bytes-per-token-*", data)
}

// BytesPerToken is the ratio, in hundredths of a byte, this console applies
// to model: none until the model has enough measured requests, then the
// value it had the first time it was asked.
func (c *TokenCalibration) BytesPerToken(model root.ModelID) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hundredths, ok := c.applied[model]; ok {
		return hundredths, true
	}
	file, err := c.read()
	if err != nil {
		return 0, false
	}
	entry := file.Models[string(model)]
	hundredths, ok := domain.BytesPerToken{Ratio: entry.BytesPerToken, Samples: entry.Samples}.Hundredths()
	if ok {
		c.applied[model] = hundredths
	}
	return hundredths, ok
}

func (c *TokenCalibration) read() (tokenCalibrationFile, error) {
	file := tokenCalibrationFile{Version: tokenCalibrationVersion, Models: map[string]tokenCalibrationEntry{}}
	handle, err := openNoFollow(c.path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return file, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxTokenCalibrationBytes {
		return file, errors.New("invalid token calibration file")
	}
	data, err := io.ReadAll(io.LimitReader(handle, maxTokenCalibrationBytes+1))
	if err != nil {
		return file, err
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&file); err != nil || file.Version != tokenCalibrationVersion {
		return tokenCalibrationFile{}, errors.New("invalid token calibration file")
	}
	if file.Models == nil {
		file.Models = map[string]tokenCalibrationEntry{}
	}
	return file, nil
}
