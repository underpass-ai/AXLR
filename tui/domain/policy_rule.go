package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// PolicyRule names a tool, with optional patterns its arguments must
// match, in the allow or deny list of an ExecutionPolicy. The name is a
// glob over the names PolicyNames gives a tool: "local_*", "kmp/*",
// "observability.query_logs". An argument pattern is a glob over the
// argument's value as text; a rule with patterns matches only a call
// that carries every named argument with a matching value.
type PolicyRule struct {
	Name      string
	Arguments map[string]string
}

const (
	maxPolicyRuleName    = 128
	maxPolicyArgumentKey = 64
	maxPolicyPattern     = 256
)

func NewPolicyRule(name string, arguments map[string]string) (PolicyRule, error) {
	rule := PolicyRule{Name: name}
	if len(arguments) > 0 {
		rule.Arguments = make(map[string]string, len(arguments))
		for key, pattern := range arguments {
			rule.Arguments[key] = pattern
		}
	}
	return rule, rule.Validate()
}

func (r PolicyRule) Validate() error {
	if r.Name == "" || len(r.Name) > maxPolicyRuleName {
		return errors.New("a policy rule names a tool, at most 128 characters")
	}
	for _, c := range r.Name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.:/*?-", c)) {
			return fmt.Errorf("policy rule %q: a tool name holds letters, digits, _ . : / - and the globs * and ?", r.Name)
		}
	}
	for key, pattern := range r.Arguments {
		if key == "" || len(key) > maxPolicyArgumentKey || !printable(key) {
			return fmt.Errorf("policy rule %q: an argument name is 1 to 64 printable characters", r.Name)
		}
		if pattern == "" || len(pattern) > maxPolicyPattern || !printable(pattern) {
			return fmt.Errorf("policy rule %q: the pattern of %s is 1 to 256 printable characters", r.Name, key)
		}
	}
	return nil
}

func printable(text string) bool {
	if !utf8.ValidString(text) {
		return false
	}
	for _, c := range text {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

func (r PolicyRule) clone() PolicyRule {
	out := PolicyRule{Name: r.Name}
	if r.Arguments != nil {
		out.Arguments = make(map[string]string, len(r.Arguments))
		for key, pattern := range r.Arguments {
			out.Arguments[key] = pattern
		}
	}
	return out
}

// matchesName reports a rule that names the tool, whatever its arguments.
func (r PolicyRule) matchesName(names []string) bool {
	for _, name := range names {
		if globMatch(r.Name, name) {
			return true
		}
	}
	return false
}

// matches reports a rule that names the tool and whose argument patterns
// the call satisfies.
func (r PolicyRule) matches(names []string, arguments map[string]string) bool {
	if !r.matchesName(names) {
		return false
	}
	for key, pattern := range r.Arguments {
		value, present := arguments[key]
		if !present || !globMatch(pattern, value) {
			return false
		}
	}
	return true
}

// byName reports a rule without argument patterns: one that forbids or
// allows the tool as a whole.
func (r PolicyRule) byName() bool { return len(r.Arguments) == 0 }

// PolicyNames lists the names a rule may match for one tool: the name the
// model calls it by and, for an MCP tool, "<plugin>/<tool>" and the tool's
// own name, since a name the model cannot use (a dot in it, a collision)
// is replaced by an alias the policy's author cannot know.
func PolicyNames(tool AvailableTool) []string {
	names := []string{string(tool.Definition.Name)}
	if tool.Identity.Kind == ToolKindPlugin {
		plugin, native := tool.Identity.Plugin.PluginID.String(), tool.Identity.Plugin.ToolName.String()
		names = append(names, plugin+"/"+native)
		if native != string(tool.Definition.Name) {
			names = append(names, native)
		}
	}
	return names
}

// policyArguments reads a call's arguments as text a pattern can match: a
// string as it is, anything else as its compact JSON.
func policyArguments(arguments axlr.JSONValue) map[string]string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &fields) != nil {
		return nil
	}
	out := make(map[string]string, len(fields))
	for key, raw := range fields {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			out[key] = text
			continue
		}
		var compact bytes.Buffer
		if json.Compact(&compact, raw) != nil {
			compact.Reset()
			compact.Write(raw)
		}
		out[key] = compact.String()
	}
	return out
}

// policyRuleNames lists the rules' names for a reason the model reads.
func policyRuleNames(rules []PolicyRule) string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		names = append(names, rule.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
