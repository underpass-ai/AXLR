package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/compose"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// skillsCommand is `axlr-tui skills`: the console's /skills for programs
// and for the control plane, which gives a bot its skills without a screen.
const skillsCommand = "skills"

const skillsUsage = `usage: axlr-tui skills <command>
  list [--refresh]                                  installed catalog skills, the catalogs' skills and newer versions
  search <terms...>                                 catalog skills by name, tags, category and summary
  install <name>[@version] [--from <catalog>] [--yes]  stage, verify and show a skill; --yes installs it
  update [name] [--yes]                             install each newer version beside the old one
  remove <name>[@version] [--from <catalog>]        remove one version, or every version
  publish <name> [--version x.y.z] [--to <catalog>] [--root <workspace>]
                                                    sign and upload .axlr/skills/<name>/ to a catalog's registry`

// skillsEnvironment is what the command reads: the settings and AXLR's
// state, as the console resolves them.
type skillsEnvironment struct {
	settings storage.UserSettings
	state    string
	skills   compose.CatalogSkills
	now      func() time.Time
	poll     time.Duration
	deadline time.Duration
}

func runSkillsCommand(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stderr, skillsUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	env, err := openSkillsEnvironment(ctx, getenv)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui skills:", err)
		return 1
	}
	return env.run(ctx, args, stdout, stderr)
}

func openSkillsEnvironment(ctx context.Context, getenv func(string) string) (skillsEnvironment, error) {
	configBase, stateBase := getenv("XDG_CONFIG_HOME"), getenv("XDG_STATE_HOME")
	home := getenv("HOME")
	if !filepath.IsAbs(configBase) || !filepath.IsAbs(stateBase) {
		if !filepath.IsAbs(home) {
			return skillsEnvironment{}, errors.New("absolute HOME, or XDG_CONFIG_HOME and XDG_STATE_HOME, is required")
		}
		if !filepath.IsAbs(configBase) {
			configBase = filepath.Join(home, ".config")
		}
		if !filepath.IsAbs(stateBase) {
			stateBase = filepath.Join(home, ".local", "state")
		}
	}
	store, err := storage.NewUserSettingsStore(filepath.Join(configBase, "axlr", "settings.json"), storage.DefaultUserSettings())
	if err != nil {
		return skillsEnvironment{}, err
	}
	settings, err := store.Load(ctx)
	if err != nil {
		return skillsEnvironment{}, err
	}
	state := filepath.Join(stateBase, "axlr")
	skills, err := compose.NewCatalogSkills(settings, state)
	if err != nil {
		return skillsEnvironment{}, err
	}
	return skillsEnvironment{settings: settings, state: state, skills: skills, now: time.Now, poll: application.PublishPollInterval, deadline: application.PublishPollDeadline}, nil
}

func (e skillsEnvironment) run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	command, rest := args[0], args[1:]
	flags := flag.NewFlagSet("axlr-tui skills "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	refresh := flags.Bool("refresh", false, "fetch every catalog's index again")
	yes := flags.Bool("yes", false, "install what the card shows, without asking")
	from := flags.String("from", "", "the catalog")
	to := flags.String("to", "", "the catalog whose registry receives the skill")
	version := flags.String("version", "", "the version to publish, instead of catalog.json's")
	root := flags.String("root", ".", "the workspace holding .axlr/skills/<name>/")
	// Flags may follow the positional arguments: `install x@1.0.0 --yes`.
	var positional []string
	for len(rest) > 0 {
		if err := flags.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		rest = flags.Args()
		if len(rest) > 0 {
			positional, rest = append(positional, rest[0]), rest[1:]
		}
	}
	use := e.skills.UseCase
	use.Offered = func(ctx context.Context) []string { return e.offered(ctx, *root) }
	fail := func(err error) int {
		fmt.Fprintln(stderr, "axlr-tui skills "+command+":", err)
		return 1
	}
	switch command {
	case "list":
		overview, err := use.Overview(ctx, *refresh)
		if err != nil {
			return fail(err)
		}
		writeSkillOverview(stdout, overview)
		return 0
	case "search":
		matches, catalogs, err := use.Search(ctx, positional)
		if err != nil {
			return fail(err)
		}
		writeCatalogWarnings(stdout, catalogs)
		if len(matches) == 0 {
			fmt.Fprintln(stdout, "no catalog skill matches")
		}
		for _, match := range matches {
			line := fmt.Sprintf("%s %s  [%s]  %s", match.Skill.Name, match.Skill.Version, match.Catalog, match.Skill.Summary)
			if len(match.Installed) > 0 {
				line += "  (installed " + strings.Join(match.Installed, ", ") + ")"
			}
			fmt.Fprintln(stdout, line)
		}
		return 0
	case "install":
		if len(positional) != 1 {
			return fail(errors.New("name one skill: install <name>[@version]"))
		}
		request := parseSkillRef(positional[0], *from)
		return e.install(ctx, use, request, *yes, stdout, stderr)
	case "update":
		if len(positional) > 1 {
			return fail(errors.New("update takes at most one name"))
		}
		overview, err := use.Overview(ctx, true)
		if err != nil {
			return fail(err)
		}
		code, any := 0, false
		for _, update := range overview.Updates {
			if len(positional) == 1 && update.Name != positional[0] {
				continue
			}
			any = true
			fmt.Fprintf(stdout, "%s: %s → %s (catalog %s)\n", update.Name, update.Installed, update.Available, update.Catalog)
			if result := e.install(ctx, use, application.InstallRequest{Name: update.Name, Version: update.Available, Catalog: update.Catalog}, *yes, stdout, stderr); result != 0 {
				code = result
			}
		}
		if !any {
			fmt.Fprintln(stdout, "every installed skill is at its catalog's newest version")
		}
		return code
	case "remove":
		if len(positional) != 1 {
			return fail(errors.New("name one skill: remove <name>[@version]"))
		}
		removed, err := use.Remove(ctx, parseSkillRef(positional[0], *from))
		if err != nil {
			return fail(err)
		}
		for _, skill := range removed {
			fmt.Fprintf(stdout, "removed %s\n", skill.Key())
		}
		return 0
	case "publish":
		if len(positional) != 1 {
			return fail(errors.New("name one skill of the workspace: publish <name>"))
		}
		workspace, err := filepath.Abs(*root)
		if err != nil {
			return fail(err)
		}
		return e.publish(ctx, use, workspace, application.PublishRequest{Name: positional[0], Version: *version, Catalog: *to}, stdout, stderr)
	default:
		fmt.Fprintln(stderr, skillsUsage)
		return 2
	}
}

// parseSkillRef reads name[@version].
func parseSkillRef(ref, catalog string) application.InstallRequest {
	name, version, _ := strings.Cut(ref, "@")
	return application.InstallRequest{Name: name, Version: version, Catalog: catalog}
}

// offered names the tools a skill may expect that this console has: the
// built-in ones and the tools installed in the workspace.
func (e skillsEnvironment) offered(ctx context.Context, root string) []string {
	names := application.BuiltinToolNames()
	workspace, err := filepath.Abs(root)
	if err != nil {
		return names
	}
	forged, err := storage.NewForgedToolStore(filepath.Join(e.state, "forged-tools"))
	if err != nil {
		return names
	}
	if tools, err := forged.List(ctx, workspace); err == nil {
		for _, tool := range tools {
			names = append(names, tool.Name)
		}
	}
	return names
}

func (e skillsEnvironment) install(ctx context.Context, use application.SkillCatalogUseCase, request application.InstallRequest, yes bool, stdout, stderr io.Writer) int {
	staged, err := use.Stage(ctx, request)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui skills install:", err)
		return 1
	}
	writeSkillCard(stdout, staged)
	if !yes {
		fmt.Fprintf(stdout, "not installed: run again with --yes to install %s\n", staged.Key())
		return 3
	}
	installed, replaced, err := use.Install(ctx, staged, e.now())
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui skills install:", err)
		return 1
	}
	verb := "installed"
	if replaced {
		verb = "reinstalled"
	}
	fmt.Fprintf(stdout, "%s %s (bundle %s)\n", verb, installed.Key(), shortDigest(installed.BundleSHA256))
	return 0
}

func (e skillsEnvironment) publish(ctx context.Context, use application.SkillCatalogUseCase, workspace string, request application.PublishRequest, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "axlr-tui skills publish:", err)
		return 1
	}
	draft, err := use.PrepareSkill(ctx, workspace, request)
	if err != nil {
		return fail(err)
	}
	bundle, err := use.BundleSkill(ctx, draft)
	if err != nil {
		return fail(err)
	}
	if bundle.KeyCreated {
		fmt.Fprintf(stdout, "created this console's registry key %s; the registry must know it (/tools enroll)\n", bundle.KeyID)
	}
	fmt.Fprintf(stdout, "publishing %s %s to catalog %s (%s), bundle %s, key %s\n", draft.Name, draft.Metadata.Version, draft.Catalog, draft.API, shortDigest(bundle.Digest), bundle.KeyID)
	report, err := use.SubmitSkill(ctx, draft, bundle)
	if err != nil {
		return fail(err)
	}
	deadline := e.now().Add(e.deadline)
	for !report.Settled() && e.now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(e.poll):
		}
		if report, err = use.PollSkill(ctx, draft); err != nil {
			return fail(err)
		}
	}
	writeVersionReport(stdout, report)
	switch {
	case report.Status == "rejected" || report.TestsPassed != nil && !*report.TestsPassed:
		return 1
	case !report.Settled():
		fmt.Fprintln(stdout, "the registry has not settled the version yet; read it again later")
		return 3
	}
	return 0
}

func writeVersionReport(out io.Writer, report application.VersionReport) {
	tests := "pending"
	if report.TestsPassed != nil {
		tests = map[bool]string{true: "passed", false: "failed"}[*report.TestsPassed]
	}
	fmt.Fprintf(out, "%s %s: %s, checks %s\n", report.Name, report.Version, report.Status, tests)
	for _, finding := range report.Findings {
		fmt.Fprintf(out, "  finding: %+v\n", finding)
	}
	for _, failed := range report.FailedTests {
		fmt.Fprintf(out, "  failed: %s\n", failed)
	}
	if report.RejectedReason != "" {
		fmt.Fprintf(out, "  rejected: %s\n", report.RejectedReason)
	}
}

func writeCatalogWarnings(out io.Writer, catalogs []application.CatalogState) {
	for _, state := range catalogs {
		if state.Warning != "" {
			fmt.Fprintf(out, "catalog %s: %s\n", state.Name, state.Warning)
		}
	}
}

func writeSkillOverview(out io.Writer, overview application.SkillOverview) {
	writeCatalogWarnings(out, overview.Catalogs)
	fmt.Fprintln(out, "installed:")
	if len(overview.Installed) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, skill := range overview.Installed {
		fmt.Fprintf(out, "  %s  bundle %s  installed %s\n", skill.Key(), shortDigest(skill.BundleSHA256), skill.InstalledAt.Format(time.RFC3339))
	}
	for _, state := range overview.Catalogs {
		if state.Index == nil || len(state.Index.Skills) == 0 {
			continue
		}
		fmt.Fprintf(out, "catalog %s (index %s):\n", state.Name, state.Index.GeneratedAt)
		for _, skill := range state.Index.Skills {
			fmt.Fprintf(out, "  %s %s  %s\n", skill.Name, skill.Version, skill.Summary)
		}
	}
	for _, update := range overview.Updates {
		fmt.Fprintf(out, "update: %s %s → %s (catalog %s)\n", update.Name, update.Installed, update.Available, update.Catalog)
	}
}

// writeSkillCard is the install card in text: origin, provenance, the
// tools the skill expects and every file as it will be installed.
func writeSkillCard(out io.Writer, staged application.StagedSkill) {
	fmt.Fprintf(out, "skill %s %s · catalog %s · verified against the signed index of %s\n", staged.Entry.Name, staged.Entry.Version, staged.Catalog, staged.IndexGeneratedAt)
	if staged.Entry.Publisher != "" {
		fmt.Fprintf(out, "publisher %s (key %s) · reviewer %s at %s · bundle %s\n", staged.Entry.Publisher, staged.Entry.PublisherKeyID, staged.Entry.Reviewer, staged.Entry.ReviewedAt, shortDigest(staged.Entry.BundleSHA256))
	}
	fmt.Fprintf(out, "description: %s\n", staged.Description)
	if len(staged.Tools) > 0 {
		fmt.Fprintf(out, "tools it expects: %s\n", strings.Join(staged.Tools, ", "))
	}
	if len(staged.Missing) > 0 {
		fmt.Fprintf(out, "not offered here: %s (an MCP server may provide them)\n", strings.Join(staged.Missing, ", "))
	}
	for _, warning := range staged.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
	for _, file := range staged.Files {
		fmt.Fprintf(out, "──── %s (%d bytes, sha256 %s)\n%s", file.Path, len(file.Content), shortDigest(staged.Entry.Files[file.Path]), file.Content)
		if !strings.HasSuffix(string(file.Content), "\n") {
			fmt.Fprintln(out)
		}
	}
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// catalogSkillsUseCase is /skills' use case: the card marks the tools a
// skill expects that neither the console nor the workspace offers.
func catalogSkillsUseCase(skills compose.CatalogSkills, workspace domain.Workspace) application.SkillCatalogUseCase {
	use := skills.UseCase
	state := filepath.Dir(skills.Store.Dir)
	use.Offered = func(ctx context.Context) []string {
		return skillsEnvironment{state: state}.offered(ctx, string(workspace))
	}
	return use
}
