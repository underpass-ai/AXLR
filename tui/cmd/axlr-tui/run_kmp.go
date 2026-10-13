package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/kmpstore"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// kmpGuideRoot is the plugin directory of the KMP engine AXLR installed,
// <root>/scripts/run-embedded-mcp.sh, when it holds the guide assets a
// store without a guide is synced from; empty otherwise.
func kmpGuideRoot(registrations []plugins.Registration) string {
	for _, r := range registrations {
		if r.Manifest.ID != "kmp" || r.Manifest.Command == "" {
			continue
		}
		root := filepath.Dir(filepath.Dir(r.Manifest.Command))
		if info, err := os.Stat(filepath.Join(root, "guide", "guide.requests.json")); err == nil && info.Mode().IsRegular() {
			return root
		}
	}
	return ""
}

// kmpStore is the memory of one console launch: the store the console
// chose and prepared, and the workspace's project about.
type kmpStore struct {
	Selection kmpstore.Selection
	Report    kmpstore.Report
	// About is the workspace's project about: the one its committed bundle
	// names, else the directory rule (application.ProjectAbout).
	About string
	// FromBundle says the about came from the checkout's bundle.
	FromBundle bool
}

// prepareKMPStore chooses the store of the stdio kmp registration for this
// workspace, prepares it and pins it in the registration's child environment
// before the plugin manager, the shared engine and the repair workbenches
// read that list, so every console path opens the same store (decision D1
// of the lifecycle integration design). Without a stdio kmp registration
// it does nothing. A preparation that fails (an import, the guide) leaves
// the store pinned and is reported; the console still starts.
func prepareKMPStore(ctx context.Context, registrations []plugins.Registration, workspace, dataBase string) (store kmpStore, found bool, err error) {
	index := -1
	for i, r := range registrations {
		if r.Manifest.ID == "kmp" && r.Manifest.Command != "" {
			index = i
			break
		}
	}
	if index < 0 {
		return kmpStore{}, false, nil
	}
	r := registrations[index]
	store.Selection = kmpstore.Resolve(workspace, r.Env, dataBase)
	registrations[index].Env = kmpstore.WithStore(r.Env, store.Selection.Path)
	store.About = application.ProjectAbout(domain.Workspace(workspace))
	binary, pluginRoot := kmpstore.Binary(r.Manifest.Command)
	preparer := kmpstore.Preparer{Binary: binary, PluginRoot: pluginRoot, UserStore: kmpstore.UserDefault(dataBase)}
	if store.Report, err = preparer.Prepare(ctx, store.Selection, r.Env); err != nil {
		return store, true, err
	}
	about, err := kmpstore.BundleAbout(store.Selection.TopLevel)
	if err != nil {
		return store, true, err
	}
	if about != "" {
		store.About, store.FromBundle = about, true
	}
	return store, true, nil
}

// line is the startup line: where this launch's memory lives, why, and what
// the console did to a store it created.
func (s kmpStore) line(dataBase string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "KMP store %s (", s.Selection.Path)
	switch s.Selection.Reason {
	case kmpstore.ReasonProject:
		fmt.Fprintf(&b, "the checkout %s", s.Selection.TopLevel)
		if s.Selection.Replaced != "" {
			fmt.Fprintf(&b, "; replaces the installer's pin %s", s.Selection.Replaced)
		}
	case kmpstore.ReasonOperator:
		b.WriteString("pinned in mcp.json")
	default:
		b.WriteString("the per-user default, outside a checkout")
	}
	fmt.Fprintf(&b, "), about %s", s.About)
	if s.FromBundle {
		b.WriteString(" (from " + kmpstore.BundlePath + ")")
	}
	if !s.Report.Created {
		return b.String()
	}
	b.WriteString("; new store")
	switch {
	case s.Report.Imported > 0:
		fmt.Fprintf(&b, ", %d events imported from %s", s.Report.Imported, kmpstore.BundlePath)
	case s.Report.Imported < 0:
		fmt.Fprintf(&b, ", %s imported", kmpstore.BundlePath)
	}
	if len(s.Report.Configured) > 0 {
		fmt.Fprintf(&b, ", %d configuration files copied from %s", len(s.Report.Configured), kmpstore.UserDefault(dataBase))
	}
	if s.Report.GuideSynced {
		b.WriteString(", guide synced")
	}
	return b.String()
}

// selectBundleAbout binds a session that has no selected about to the one
// the checkout's bundle names, as the person would with axlr_session: it
// is an explicit canonical scope, so axlr_session reports it as selected
// rather than as the default. An about already selected is kept.
func selectBundleAbout(ctx context.Context, labels application.SessionLabelsPort, id domain.SessionID, about string) error {
	_, err := labels.Initialize(ctx, id, domain.SessionLabel{About: about})
	return err
}
