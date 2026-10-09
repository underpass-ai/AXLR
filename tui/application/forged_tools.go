package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Forged tools are the model's own workspace tools: it writes a small program
// with axlr_forge_tool and calls it at once with axlr_run_tool, in the same
// session and without restarting the console. Both host tools are fixed, so
// a forged tool changes neither the request's tools nor the system prompt;
// axlr_tools lists the forged tools beside the plugin ones.
const (
	HostForgeToolName root.ToolName = "axlr_forge_tool"
	HostRunToolName   root.ToolName = "axlr_run_tool"
	// ForgedToolsDir is where a workspace keeps its forged tools' files, one
	// directory per tool; the registry, which only axlr_forge_tool writes,
	// lives in the console's state.
	ForgedToolsDir = ".axlr/tools"
)

// Bounds of one forged tool.
const (
	maxForgedFiles          = 8
	maxForgedFileBytes      = 64 * 1024
	maxForgedBytes          = 256 * 1024
	maxForgedArgs           = 32
	maxForgedArgBytes       = 1024
	maxForgedDescription    = 1000
	maxForgedSchemaBytes    = 16 * 1024
	maxForgedArgumentsBytes = 64 * 1024
)

var (
	forgedToolName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	forgedProgram  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
)

// ForgedTool is one registered tool of a workspace: what the model sees, the
// command the console runs from the workspace root with the arguments as JSON
// on stdin, and the SHA-256 of each file it was forged with.
type ForgedTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema json.RawMessage   `json:"input_schema"`
	Program     string            `json:"program"`
	Args        []string          `json:"args"`
	Files       map[string]string `json:"files"`
	ForgedAt    string            `json:"forged_at,omitempty"`
	Session     string            `json:"session,omitempty"`
}

// ForgedFile is one file of a tool, its path relative to the tool's directory.
type ForgedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ForgedToolsPort keeps a workspace's forged tools. Only Forge registers a
// tool: a file written another way into ForgedToolsDir, or brought by a
// checkout, is never one, and Verify refuses a tool whose files changed
// since it was forged.
type ForgedToolsPort interface {
	List(ctx context.Context, workspace string) ([]ForgedTool, error)
	// Forge replaces the tool's directory with files, fills Files and
	// ForgedAt and registers the tool; replaced reports a previous version.
	Forge(ctx context.Context, workspace string, tool ForgedTool, files []ForgedFile) (stored ForgedTool, replaced bool, err error)
	Verify(ctx context.Context, workspace string, tool ForgedTool) error
}

func forgeSchema() string {
	return `{"type":"object","properties":{` +
		`"name":{"type":"string","pattern":"^[a-z][a-z0-9_]{1,47}$"},` +
		`"description":{"type":"string","minLength":1,"maxLength":1000},` +
		`"input_schema":{"type":"object"},` +
		`"program":{"type":"string","minLength":1,"maxLength":64},` +
		`"args":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":1024}},` +
		`"files":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"object","properties":{"path":{"type":"string","minLength":1,"maxLength":256},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}}},` +
		`"required":["name","description","input_schema","program","args","files"],"additionalProperties":false}`
}

// ForgeTool is axlr_forge_tool.
func ForgeTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationForgeTool)
	schema, _ := root.NewJSONObject([]byte(forgeSchema()))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostForgeToolName,
		Description: "Create or replace a tool of this workspace when no local, host, plugin or forged tool does the job (search axlr_tools first; it lists forged tools). Write it as small files under " + ForgedToolsDir + "/<name>/; the console runs program (a name on PATH, such as python3, bash or node) with args from the workspace root, passes the arguments as one JSON object on stdin and returns stdout, stderr and the exit code. args must name one of the tool's files by its workspace path, " + ForgedToolsDir + "/<name>/<file>. input_schema is the JSON Schema of the arguments; description says what it does and returns. Keep it focused, deterministic and free of secrets; print the result on stdout and fail with a nonzero exit and a message on stderr. To fix a tool, forge it again under the same name. Approved like local_write; the tool is callable at once with axlr_run_tool.",
		Parameters:  schema,
	}}
}

// RunTool is axlr_run_tool.
func RunTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationRunTool)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"name":{"type":"string","minLength":1},"arguments":{"type":"object"}},"required":["name","arguments"],"additionalProperties":false}`))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostRunToolName,
		Description: "Run a tool forged with axlr_forge_tool in this workspace by its exact name, with arguments that match its input_schema (axlr_tools with name returns it). The result is its exit code, stdout and stderr. A tool whose files changed since it was forged is refused until it is forged again. Approved like local_exec.",
		Parameters:  schema,
	}}
}

// offersForge reports whether a request offers the forged-tool host tools:
// the console keeps forged tools and the session is an ordinary one, not a
// ceremony or a plan task, whose steps have their own tools.
func (u ContinueTurnUseCase) offersForge(s domain.Session) bool {
	if u.Forge == nil {
		return false
	}
	_, live := s.Ceremony()
	return !live && !s.Mode().StartsCeremony() && s.Mode() != domain.ModeTask
}

// hostForge validates axlr_forge_tool's arguments and registers the tool.
func (u HostToolUseCase) hostForge(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Forge == nil {
		return nil, errors.New("forged tools are not enabled in this console")
	}
	var args struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
		Program     string          `json:"program"`
		Args        []string        `json:"args"`
		Files       []ForgedFile    `json:"files"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, errors.New("axlr_forge_tool arguments must match its schema: name, description, input_schema, program, args, files")
	}
	files, err := validateForge(args.Name, args.Description, args.InputSchema, args.Program, args.Args, args.Files)
	if err != nil {
		return nil, err
	}
	state := session.Export()
	tool := ForgedTool{Name: args.Name, Description: strings.TrimSpace(args.Description), InputSchema: args.InputSchema, Program: args.Program, Args: args.Args, Session: string(state.ID)}
	stored, replaced, err := u.Forge.Forge(ctx, string(state.Workspace), tool, files)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, ForgedToolsDir+"/"+stored.Name+"/"+file.Path)
	}
	return map[string]any{"forged": stored.Name, "replaced": replaced, "files": paths, "command": append([]string{stored.Program}, stored.Args...), "call_with": string(HostRunToolName)}, nil
}

// validateForge checks a tool before anything is written and returns its
// files with clean paths.
func validateForge(name, description string, schema json.RawMessage, program string, args []string, files []ForgedFile) ([]ForgedFile, error) {
	if !forgedToolName.MatchString(name) {
		return nil, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	if strings.TrimSpace(description) == "" || len(description) > maxForgedDescription {
		return nil, fmt.Errorf("description must be 1-%d bytes", maxForgedDescription)
	}
	if len(schema) > maxForgedSchemaBytes {
		return nil, fmt.Errorf("input_schema exceeds %d bytes", maxForgedSchemaBytes)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(schema, &object) != nil || object == nil {
		return nil, errors.New("input_schema must be a JSON Schema object")
	}
	var kind string
	if raw, ok := object["type"]; !ok || json.Unmarshal(raw, &kind) != nil || kind != "object" {
		return nil, errors.New(`input_schema must have "type": "object"; the arguments are one JSON object`)
	}
	if !forgedProgram.MatchString(program) {
		return nil, errors.New("program must be a program name on PATH, such as python3, bash or node, not a path")
	}
	if len(args) > maxForgedArgs {
		return nil, fmt.Errorf("args exceeds %d items", maxForgedArgs)
	}
	if len(files) == 0 || len(files) > maxForgedFiles {
		return nil, fmt.Errorf("files must list 1-%d files", maxForgedFiles)
	}
	clean := make([]ForgedFile, 0, len(files))
	seen := map[string]bool{}
	total := 0
	own := ForgedToolsDir + "/" + name + "/"
	for _, file := range files {
		// Models name a file by its workspace path as often as by its path
		// in the tool's directory (seen with claude-haiku-5.5 on 9 Oct
		// 2026); both mean the same file.
		relative := path.Clean(strings.TrimPrefix(path.Clean(file.Path), own))
		if file.Path == "" || strings.Contains(file.Path, "\\") || path.IsAbs(file.Path) || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || strings.HasPrefix(relative, ForgedToolsDir+"/") {
			return nil, fmt.Errorf("file path %q must be inside the tool's directory: main.py or %smain.py", file.Path, own)
		}
		if seen[relative] {
			return nil, fmt.Errorf("file path %q is listed twice", file.Path)
		}
		seen[relative] = true
		if len(file.Content) > maxForgedFileBytes {
			return nil, fmt.Errorf("file %q exceeds %d bytes", file.Path, maxForgedFileBytes)
		}
		total += len(file.Content)
		clean = append(clean, ForgedFile{Path: relative, Content: file.Content})
	}
	if total > maxForgedBytes {
		return nil, fmt.Errorf("files exceed %d bytes in all", maxForgedBytes)
	}
	names := false
	for _, arg := range args {
		if len(arg) > maxForgedArgBytes {
			return nil, fmt.Errorf("an args item exceeds %d bytes", maxForgedArgBytes)
		}
		if relative, ok := strings.CutPrefix(path.Clean(arg), own); ok && seen[relative] {
			names = true
		}
	}
	if !names {
		return nil, fmt.Errorf("args must name one of the tool's files by its workspace path, such as %s%s", own, clean[0].Path)
	}
	return clean, nil
}

// hostRun runs a forged tool through local_exec: the runtime's confinement
// and sandbox, timeout and output limit apply as to any program.
func (u HostToolUseCase) hostRun(ctx context.Context, session domain.Session, arguments root.JSONValue) (domain.ToolOutcome, error) {
	if u.Forge == nil {
		return hostFailure(errors.New("forged tools are not enabled in this console")), nil
	}
	if u.Tools == nil {
		return hostFailure(errors.New("forged tools need the local runtime")), nil
	}
	var args struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil || args.Name == "" {
		return hostFailure(errors.New("axlr_run_tool arguments must be name and arguments")), nil
	}
	input, err := root.NewJSONObject(args.Arguments)
	if err != nil {
		return hostFailure(errors.New("arguments must be a JSON object")), nil
	}
	if len(input.Bytes()) > maxForgedArgumentsBytes {
		return hostFailure(fmt.Errorf("arguments exceed %d bytes", maxForgedArgumentsBytes)), nil
	}
	workspace := string(session.Export().Workspace)
	tools, err := u.Forge.List(ctx, workspace)
	if err != nil {
		return hostFailure(err), nil
	}
	tool, found := findForged(tools, args.Name)
	if !found {
		return hostFailure(fmt.Errorf("unknown forged tool %q; search axlr_tools or forge it with axlr_forge_tool", args.Name)), nil
	}
	if err := u.Forge.Verify(ctx, workspace, tool); err != nil {
		return hostFailure(err), nil
	}
	if u.Validation != nil {
		schema, err := root.NewJSONObject(tool.InputSchema)
		if err != nil {
			return hostFailure(fmt.Errorf("forged tool %q has no valid input_schema; forge it again", tool.Name)), nil
		}
		if err := u.Validation.Validate(root.ToolDefinition{Name: root.ToolName(forgedAlias(tool.Name)), Description: root.Text(tool.Description), Parameters: schema}, input); err != nil {
			return hostFailure(fmt.Errorf("arguments do not match %s's input_schema: %v", tool.Name, err)), nil
		}
	}
	command, err := json.Marshal(map[string]any{"program": tool.Program, "args": tool.Args, "stdin": string(input.Bytes())})
	if err != nil {
		return hostFailure(err), nil
	}
	value, err := root.NewJSONObject(command)
	if err != nil {
		return hostFailure(err), nil
	}
	return u.Tools.Execute(ctx, localIdentity("exec"), value)
}

func localIdentity(operation string) domain.ToolIdentity {
	identity, _ := domain.NewLocalToolIdentity(operation)
	return identity
}

func findForged(tools []ForgedTool, name string) (ForgedTool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return ForgedTool{}, false
}

// forgedAlias is a valid tool-definition name for validating a forged tool's
// arguments; it is never offered to the model.
func forgedAlias(name string) string { return "forged_" + name }
