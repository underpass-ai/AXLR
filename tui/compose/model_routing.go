package compose

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelRouting is what the console knows of an execution's candidate
// models (application.ModelRoutingPort): a local_models entry is on this
// host when its server's address is a loopback one (localhost, 127.0.0.0/8,
// ::1), and off it otherwise, a native Anthropic or OpenAI entry and a
// server elsewhere on the network included; every other model goes to
// OpenRouter, which needs its key. The cap is the context cap the console
// applies to the model.
type ModelRouting struct {
	// OnHost names the local_models entries, true for those served on
	// this host.
	OnHost  map[root.ModelID]bool
	Windows localmodels.Windows
	// Remote is true when OpenRouter can be asked: its key is set.
	Remote bool
	// Catalog is OpenRouter's model catalog, read once when a price is
	// first needed; nil knows no price.
	Catalog *CatalogPrices
}

var _ application.ModelRoutingPort = ModelRouting{}

// NewModelRouting reads the local models of the settings.
func NewModelRouting(locals LocalModels, remote bool, catalog application.ModelCatalogPort) ModelRouting {
	routing := ModelRouting{OnHost: map[root.ModelID]bool{}, Windows: locals.Windows, Remote: remote}
	for _, local := range locals.Models {
		routing.OnHost[root.ModelID(local.ID)] = OnThisHost(local.URL)
	}
	if catalog != nil && remote {
		routing.Catalog = &CatalogPrices{Catalog: catalog}
	}
	return routing
}

func (r ModelRouting) RouteFacts(_ context.Context, model root.ModelID) domain.RouteCandidate {
	onHost, configured := r.OnHost[model]
	return domain.RouteCandidate{Model: model, Local: configured && onHost, Reachable: configured || r.Remote, Cap: r.Windows.ContextCap(model)}
}

func (r ModelRouting) CatalogPrice(ctx context.Context, model root.ModelID) (float64, float64, bool) {
	if _, configured := r.OnHost[model]; configured || r.Catalog == nil {
		return 0, 0, false
	}
	return r.Catalog.Price(ctx, model)
}

// OnThisHost reports whether a server's URL names this host: localhost, a
// name under .localhost, or a loopback address.
func OnThisHost(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CatalogPrices reads a model catalog once, the first time a price is
// asked, and keeps what it listed for the process; a catalog that cannot
// be read lists no price, and is not read again.
type CatalogPrices struct {
	Catalog application.ModelCatalogPort
	once    sync.Once
	prices  map[root.ModelID]domain.AvailableModel
}

func (c *CatalogPrices) Price(ctx context.Context, model root.ModelID) (float64, float64, bool) {
	c.once.Do(func() {
		c.prices = map[root.ModelID]domain.AvailableModel{}
		models, err := c.Catalog.List(context.WithoutCancel(ctx))
		if err != nil {
			return
		}
		for _, listed := range models {
			c.prices[listed.ID] = listed
		}
	})
	listed, found := c.prices[model]
	if !found {
		return 0, 0, false
	}
	prompt, promptKnown := listed.PromptRate.PerToken()
	completion, completionKnown := listed.CompletionRate.PerToken()
	return prompt, completion, promptKnown && completionKnown
}
