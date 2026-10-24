package domain

import (
	"path"
	"path/filepath"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ExecutionPolicy is what one execution may do with tools, decided by the
// program that runs it (docs/runtime-contract.md, "Policy per execution"):
// an autonomy level, the tools it may use and the ones it may not. With
// an allow list, every tool it does not name is forbidden; a deny rule
// wins over an allow rule. The policy only narrows: a call it allows
// still passes the run's approval level, the settings' rules, the work
// mode and the destructive list.
type ExecutionPolicy struct {
	level PolicyLevel
	allow []PolicyRule
	deny  []PolicyRule
}

func NewExecutionPolicy(level PolicyLevel, allow, deny []PolicyRule) (ExecutionPolicy, error) {
	policy := ExecutionPolicy{level: level}
	for _, rule := range allow {
		policy.allow = append(policy.allow, rule.clone())
	}
	for _, rule := range deny {
		policy.deny = append(policy.deny, rule.clone())
	}
	return policy, policy.Validate()
}

func (p ExecutionPolicy) Validate() error {
	if err := p.level.Validate(); err != nil {
		return err
	}
	for _, rule := range append(append([]PolicyRule(nil), p.allow...), p.deny...) {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p ExecutionPolicy) Level() PolicyLevel { return p.level }

// Allow and Deny are the rules as given, copied.
func (p ExecutionPolicy) Allow() []PolicyRule { return cloneRules(p.allow) }
func (p ExecutionPolicy) Deny() []PolicyRule  { return cloneRules(p.deny) }

func (p ExecutionPolicy) clone() ExecutionPolicy {
	return ExecutionPolicy{level: p.level, allow: cloneRules(p.allow), deny: cloneRules(p.deny)}
}

func cloneRules(rules []PolicyRule) []PolicyRule {
	if rules == nil {
		return nil
	}
	out := make([]PolicyRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.clone())
	}
	return out
}

// Judge is the policy's verdict on one resolved call, with the reason
// the model reads when the call is forbidden. The bridge axlr_call_tool
// never arrives here: the call resolves to the MCP tool it names.
func (p ExecutionPolicy) Judge(tool AvailableTool, arguments axlr.JSONValue) (PolicyVerdict, string) {
	names, values := PolicyNames(tool), policyArguments(arguments)
	label := string(tool.Definition.Name)
	for _, rule := range p.deny {
		if rule.matches(names, values) {
			return PolicyForbid, label + " is denied by the execution's policy (rule " + rule.Name + ")"
		}
	}
	if isHost(tool, HostOperationTools) {
		// The listing is filtered by this policy; the model may always
		// read what it is left.
		return PolicyAllow, ""
	}
	listed := false
	if len(p.allow) > 0 {
		named := false
		for _, rule := range p.allow {
			if rule.matches(names, values) {
				listed = true
				break
			}
			named = named || rule.matchesName(names)
		}
		if !listed {
			if named {
				return PolicyForbid, "the arguments of " + label + " match no allow rule of the execution's policy"
			}
			return PolicyForbid, label + " is not in the execution's allow list (" + policyRuleNames(p.allow) + ")"
		}
	}
	if ReadsOnly(tool.Identity) {
		return PolicyAllow, ""
	}
	plugin := tool.Identity.Kind == ToolKindPlugin
	switch p.level {
	case PolicyObserve:
		if plugin && listed {
			return PolicyAllow, ""
		}
		return PolicyForbid, "the execution's policy observes: " + label + " is not a read"
	case PolicyPropose:
		if plugin && listed || isLocal(tool, "write", "edit") && proposalPath(arguments) {
			return PolicyAllow, ""
		}
		return PolicyForbid, "the execution's policy proposes: " + label + " may only write a plan or a diff (.md, .mdx, .txt, .rst, a file under docs/, .diff or .patch)"
	case PolicyActWithApproval:
		return PolicyAsk, ""
	}
	return PolicyAllow, ""
}

// AsksForEachWrite reports the level that puts every allowed write under a
// decision.
func (p ExecutionPolicy) AsksForEachWrite() bool { return p.level == PolicyActWithApproval }

// OfferedTools is the snapshot without the tools the policy forbids by
// name, so the model is not offered what it may not call: a tool a deny
// rule names as a whole, one an allow list leaves out, a write under
// observe, a command or an MCP tool the allow list does not vouch for
// under propose. axlr_tools stays, and axlr_call_tool while an MCP tool
// is offered.
func (p ExecutionPolicy) OfferedTools(snapshot []AvailableTool) []AvailableTool {
	plugins := false
	for _, tool := range snapshot {
		if tool.Identity.Kind == ToolKindPlugin && p.offers(tool) {
			plugins = true
			break
		}
	}
	out := make([]AvailableTool, 0, len(snapshot))
	for _, tool := range snapshot {
		switch {
		case isHost(tool, HostOperationTools), isHost(tool, HostOperationCallTool):
			if p.deniedByName(tool) || (isHost(tool, HostOperationCallTool) && !plugins) {
				continue
			}
		case !p.offers(tool):
			continue
		}
		out = append(out, tool)
	}
	return out
}

func (p ExecutionPolicy) deniedByName(tool AvailableTool) bool {
	names := PolicyNames(tool)
	for _, rule := range p.deny {
		if rule.byName() && rule.matchesName(names) {
			return true
		}
	}
	return false
}

// offers reports a tool the policy may allow with the right arguments.
func (p ExecutionPolicy) offers(tool AvailableTool) bool {
	if p.deniedByName(tool) {
		return false
	}
	names := PolicyNames(tool)
	listed := false
	if len(p.allow) > 0 {
		for _, rule := range p.allow {
			if rule.matchesName(names) {
				listed = true
				break
			}
		}
		if !listed {
			return false
		}
	}
	if ReadsOnly(tool.Identity) {
		return true
	}
	plugin := tool.Identity.Kind == ToolKindPlugin
	switch p.level {
	case PolicyObserve:
		return plugin && listed
	case PolicyPropose:
		return plugin && listed || isLocal(tool, "write", "edit")
	}
	return true
}

func isHost(tool AvailableTool, operation string) bool {
	return tool.Identity.Kind == ToolKindHost && tool.Identity.LocalOperation == operation
}

func isLocal(tool AvailableTool, operations ...string) bool {
	if tool.Identity.Kind != ToolKindLocal {
		return false
	}
	for _, operation := range operations {
		if tool.Identity.LocalOperation == operation {
			return true
		}
	}
	return false
}

// proposalPath reports a write a proposing execution may make: a document
// (as the writer modes allow) or a .diff or .patch file in the workspace.
func proposalPath(arguments axlr.JSONValue) bool {
	target := policyArguments(arguments)["path"]
	if documentPath(target) {
		return true
	}
	relative, err := axlr.NewRelativePath(target)
	if err != nil {
		return false
	}
	clean := path.Clean(filepath.ToSlash(string(relative)))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	ext := strings.ToLower(path.Ext(clean))
	return ext == ".diff" || ext == ".patch"
}
