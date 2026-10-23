package compose

import (
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/catalog"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/registry"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/tokens"
	"github.com/underpass-ai/AXLR/tui/application"
)

// Equipment is what a continuation's own tools need beyond the model loop.
type Equipment struct {
	Settings storage.UserSettings
	// State is AXLR's private state directory: <XDG_STATE_HOME>/axlr for
	// the console, state_dir for the service. Forged tools, catalogs and
	// the registry key live under it.
	State string
	Store application.SessionStorePort
	// Usage keeps each session's ledger beside its snapshot.
	Usage application.SessionUsagePort
	// Labels and About bind a successor to its predecessor's title and to
	// the checkout's memory about; nil Labels binds nothing.
	Labels  application.SessionLabelsPort
	About   string
	Windows localmodels.Windows
	Tools   application.ToolExecutionPort
	// Memory is true with KMP connected: axlr_remember and axlr_recall.
	Memory bool
	// Note says what could not be equipped; nil says nothing.
	Note func(string)
}

// Equip gives a continuation the usage ledger and cost cap, the context cap
// with its successor sessions (decision D10), forged and catalog tools when
// settings enable them, and the memory tools with KMP. The catalog use case
// it returns serves the console's /tools; it is the zero value with forged
// tools off.
func Equip(c *application.ContinueTurnUseCase, e Equipment) (application.CatalogUseCase, error) {
	c.Usage, c.MaxSessionUSD = e.Usage, e.Settings.MaxSessionUSD
	c.TurnToolCalls = e.Settings.TurnToolCalls
	// The context cap: one tokenizer for every model, no cut and no
	// checkpoint, a successor session past the cap.
	if counter, err := tokens.New(); err != nil {
		if e.Note != nil {
			e.Note("token counter unavailable, the byte budgets apply: " + err.Error())
		}
	} else {
		c.Tokens, c.ContextCap, c.Scale = counter, e.Windows.ContextCap, &application.TokenScale{}
		c.Successor = Successors{Create: application.CreateSessionUseCase{Store: e.Store, Labels: e.Labels, About: e.About}, Labels: e.Labels, Store: e.Store}
	}
	var tools application.CatalogUseCase
	if e.Settings.ForgedToolsEnabled() {
		// The model forges workspace tools and runs them in the same session.
		forged, err := storage.NewForgedToolStore(filepath.Join(e.State, "forged-tools"))
		if err != nil {
			return application.CatalogUseCase{}, err
		}
		c.Forge = forged
		// The catalogs of pre-forged tools: axlr_catalog, axlr_install_tool
		// and /tools read their signed indexes, cached under the state.
		client := &catalog.Client{Dir: filepath.Join(e.State, "catalog"), Entries: e.Settings.EffectiveCatalogs()}
		c.Catalog = client
		// The registry's client identity: one key under the state, signed
		// requests for /tools enroll and /tools publish.
		tools = application.CatalogUseCase{Catalog: client, Forge: forged, Keys: &registry.KeyStore{Dir: filepath.Join(e.State, "registry")}, Registry: &registry.Client{}, Files: forged}
		// axlr_publish_tool takes the steps /tools publish takes, on a card.
		c.Publisher = tools
	}
	if e.Memory {
		// axlr_remember writes the model's memories in one call;
		// axlr_recall reads them in one.
		c.Remember = ceremonyhost.Memory{Tools: e.Tools}
		c.Recall = ceremonyhost.Memory{Tools: e.Tools}
	}
	return tools, nil
}
