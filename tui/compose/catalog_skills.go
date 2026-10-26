package compose

import (
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/catalog"
	"github.com/underpass-ai/AXLR/tui/adapters/registry"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// CatalogSkills is what the console and the service need of catalog
// skills: the installed versions under <state>/skills, the reader the
// skill index and axlr_skill serve the session's versions from, and the use
// case /skills and `axlr-tui skills` run.
type CatalogSkills struct {
	Store   *storage.CatalogSkillStore
	Reader  *application.SessionSkillReader
	UseCase application.SkillCatalogUseCase
}

// NewCatalogSkills opens the installed skills under state, AXLR's private
// state directory, and reads the settings' catalogs from the same cache as
// the tools.
func NewCatalogSkills(settings storage.UserSettings, state string) (CatalogSkills, error) {
	store, err := storage.NewCatalogSkillStore(filepath.Join(state, "skills"))
	if err != nil {
		return CatalogSkills{}, err
	}
	client := &catalog.Client{Dir: filepath.Join(state, "catalog"), Entries: settings.EffectiveCatalogs()}
	return CatalogSkills{
		Store:  store,
		Reader: &application.SessionSkillReader{Installed: store},
		UseCase: application.SkillCatalogUseCase{
			Catalog: client, Stager: client, Installed: store,
			Keys: &registry.KeyStore{Dir: filepath.Join(state, "registry")}, Registry: &registry.Client{}, Sources: storage.SkillSources{},
		},
	}, nil
}
