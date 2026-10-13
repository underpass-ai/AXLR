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
	// names, else the checkout's directory name, else the workspace's
	// (application.ProjectAbout).
	About string
	// FromBundle says the about came from the checkout's bundle; FromRoot
	// that it came from the checkout's name because the workspace is a
	// directory inside it. Either is bound to the session, since the
	// console's default about is derived from the workspace alone.
	FromBundle, FromRoot bool
	// SyncGuide writes the guide into the store, for a kmp_guide that
	// finds none; nil when no plugin root holds the guide assets.
	SyncGuide func(context.Context) error
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
	if top := store.Selection.TopLevel; top != "" {
		// --root inside a checkout: the memory is the checkout's, so the
		// about is its name, not the subdirectory's (seen with
		// --root Repo-B/src/deep giving project:deep on 13 October 2026).
		if about := application.ProjectAbout(domain.Workspace(top)); about != "" && about != store.About {
			store.About, store.FromRoot = about, true
		}
	}
	binary, pluginRoot := kmpstore.Binary(r.Manifest.Command)
	preparer := kmpstore.Preparer{Binary: binary, PluginRoot: pluginRoot, UserStore: kmpstore.UserDefault(dataBase)}
	selection, env := store.Selection, r.Env
	store.SyncGuide = func(ctx context.Context) error { return preparer.SyncGuide(ctx, selection, env) }
	if store.Report, err = preparer.Prepare(ctx, store.Selection, r.Env); err != nil {
		return store, true, err
	}
	about, err := kmpstore.BundleAbout(store.Selection.TopLevel)
	if err != nil {
		return store, true, err
	}
	if about != "" {
		store.About, store.FromBundle, store.FromRoot = about, true, false
	}
	return store, true, nil
}

// bundleAbout is the about to bind new sessions to: the bundle's or the
// checkout's when it differs from the workspace's own, or none.
func (s kmpStore) bundleAbout() string {
	if s.FromBundle || s.FromRoot {
		return s.About
	}
	return ""
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
	switch {
	case s.FromBundle:
		b.WriteString(" (from " + kmpstore.BundlePath + ")")
	case s.FromRoot:
		b.WriteString(" (the checkout's name)")
	}
	if !s.Report.Created {
		return b.String()
	}
	b.WriteString("; new store")
	switch {
	case s.Report.ImportFailed != "":
		fmt.Fprintf(&b, ", %s not imported: %s", kmpstore.BundlePath, s.Report.ImportFailed)
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
