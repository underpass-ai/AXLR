package storage

import (
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
	modelStatsVersion = 1
	// maxModelStatsModels bounds the file: the models least recently
	// observed go first.
	maxModelStatsModels = 64
	maxModelStatsBytes  = 4 << 20
)

var _ application.ModelStatsPort = (*ModelStatsStore)(nil)

// ModelStatsStore keeps each model's recent requests in one private file,
// <state>/model-stats.json, shared by every console and service of the
// install: each request adds its observation under the file's lock, and
// the router reads them back as stats. A damaged file only loses what
// earlier requests said.
type ModelStatsStore struct {
	path string
	mu   sync.Mutex
}

type modelStatsFile struct {
	Version int                        `json:"version"`
	Models  map[string]modelStatsEntry `json:"models"`
}

type modelStatsEntry struct {
	Providers map[string][]modelStatsLine `json:"providers"`
}

// modelStatsLine is one observation: durations in milliseconds, cost in
// dollars, absent when the request reported none.
type modelStatsLine struct {
	At               time.Time `json:"at"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	CostUSD          *float64  `json:"cost_usd,omitempty"`
	FirstByteMS      int64     `json:"first_byte_ms,omitempty"`
	TotalMS          int64     `json:"total_ms,omitempty"`
	Failed           bool      `json:"failed,omitempty"`
}

func NewModelStatsStore(path string) (*ModelStatsStore, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("model stats path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &ModelStatsStore{path: path}, nil
}

// ObserveModel adds one request of model to its window.
func (s *ModelStatsStore) ObserveModel(ctx context.Context, model root.ModelID, observation domain.ModelObservation) error {
	if _, err := root.NewModelID(string(model)); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, s.path)
	if err != nil {
		return err
	}
	defer release()
	file, err := s.read()
	if err != nil {
		file = modelStatsFile{Version: modelStatsVersion, Models: map[string]modelStatsEntry{}}
	}
	history := file.Models[string(model)].history()
	history.Add(observation)
	file.Models[string(model)] = entryOf(history)
	if len(file.Models) > maxModelStatsModels {
		ids := make([]string, 0, len(file.Models))
		updated := make(map[string]time.Time, len(file.Models))
		for id, entry := range file.Models {
			ids = append(ids, id)
			updated[id] = entry.history().Updated()
		}
		sort.Slice(ids, func(i, j int) bool { return updated[ids[i]].Before(updated[ids[j]]) })
		for _, id := range ids[:len(ids)-maxModelStatsModels] {
			delete(file.Models, id)
		}
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return writePrivateFile(ctx, s.path, ".model-stats-*", data)
}

// ModelStats reads the stats of models at now; a model never observed has
// zero stats. A file that cannot be read reads as none observed.
func (s *ModelStatsStore) ModelStats(ctx context.Context, models []root.ModelID, now time.Time) (map[root.ModelID]domain.ModelStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[root.ModelID]domain.ModelStats, len(models))
	file, err := s.read()
	if err != nil {
		return out, nil
	}
	for _, model := range models {
		if entry, seen := file.Models[string(model)]; seen {
			out[model] = entry.history().Stats(now)
		}
	}
	return out, nil
}

func (s *ModelStatsStore) read() (modelStatsFile, error) {
	handle, err := openNoFollow(s.path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return modelStatsFile{Version: modelStatsVersion, Models: map[string]modelStatsEntry{}}, nil
	}
	if err != nil {
		return modelStatsFile{}, err
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, maxModelStatsBytes+1))
	if err != nil {
		return modelStatsFile{}, err
	}
	if len(data) > maxModelStatsBytes {
		return modelStatsFile{}, errors.New("the model stats file exceeds 4 MiB")
	}
	var file modelStatsFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != modelStatsVersion {
		return modelStatsFile{}, errors.New("the model stats file is not one this console reads")
	}
	if file.Models == nil {
		file.Models = map[string]modelStatsEntry{}
	}
	return file, nil
}

func (e modelStatsEntry) history() domain.ModelHistory {
	history := domain.ModelHistory{Providers: map[string][]domain.ModelObservation{}}
	for provider, lines := range e.Providers {
		observations := make([]domain.ModelObservation, 0, len(lines))
		for _, line := range lines {
			observation := domain.ModelObservation{At: line.At, Provider: provider, PromptTokens: line.PromptTokens, CompletionTokens: line.CompletionTokens, FirstByte: time.Duration(line.FirstByteMS) * time.Millisecond, Total: time.Duration(line.TotalMS) * time.Millisecond, Failed: line.Failed}
			if line.CostUSD != nil && *line.CostUSD >= 0 {
				observation.Cost, observation.CostKnown = *line.CostUSD, true
			}
			observations = append(observations, observation)
		}
		if len(observations) > domain.ModelHistorySamples {
			observations = observations[len(observations)-domain.ModelHistorySamples:]
		}
		if len(observations) > 0 {
			history.Providers[provider] = observations
		}
	}
	return history
}

func entryOf(history domain.ModelHistory) modelStatsEntry {
	entry := modelStatsEntry{Providers: make(map[string][]modelStatsLine, len(history.Providers))}
	for provider, observations := range history.Providers {
		lines := make([]modelStatsLine, 0, len(observations))
		for _, observation := range observations {
			line := modelStatsLine{At: observation.At.UTC(), PromptTokens: observation.PromptTokens, CompletionTokens: observation.CompletionTokens, FirstByteMS: observation.FirstByte.Milliseconds(), TotalMS: observation.Total.Milliseconds(), Failed: observation.Failed}
			if observation.CostKnown {
				cost := observation.Cost
				line.CostUSD = &cost
			}
			lines = append(lines, line)
		}
		entry.Providers[provider] = lines
	}
	return entry
}
