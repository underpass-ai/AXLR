package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// ExecutionReceipt is the wire form of a receipt. Its arrays are never
// null; the policy's, the budget's, the sandbox's and the routing's sha256 digest their
// compact JSON as the contract writes them.
func ExecutionReceipt(receipt domain.ExecutionReceipt) dto.ExecutionReceipt {
	out := dto.ExecutionReceipt{
		Schema:    dto.ExecutionReceiptSchema,
		Status:    string(receipt.Status),
		Session:   string(receipt.Session),
		Sessions:  make([]string, 0, len(receipt.Sessions)),
		StartedAt: receipt.StartedAt.UTC(),
		EndedAt:   receipt.EndedAt.UTC(),
		Requests:  make([]dto.ReceiptRequest, 0, len(receipt.Requests)),
		Calls:     make([]dto.ReceiptCall, 0, len(receipt.Calls)),
		Total:     executionUsage(receipt.Total),
		Complete:  receipt.Complete,
	}
	for _, id := range receipt.Sessions {
		out.Sessions = append(out.Sessions, string(id))
	}
	if receipt.Policy != nil {
		wire := ExecutionPolicy(*receipt.Policy)
		out.Policy = &dto.ReceiptPolicy{Level: wire.Level, SHA256: jsonDigest(wire)}
	}
	if receipt.Budget != nil {
		wire := ExecutionBudget(*receipt.Budget)
		out.Budget = &dto.ReceiptBudget{SHA256: jsonDigest(wire), Limits: wire}
	}
	if receipt.Sandbox != nil {
		wire := ExecutionSandbox(*receipt.Sandbox)
		out.Sandbox = &dto.ReceiptSandbox{SHA256: jsonDigest(wire), Profile: wire.Profile, Egress: wire.Egress, Allow: wire.Allow}
	}
	if receipt.Routing != nil {
		wire := ExecutionRouting(*receipt.Routing)
		out.Routing = &dto.ReceiptRouting{SHA256: jsonDigest(wire), Objective: wire.Objective, Candidates: wire.Candidates, AllowRemoteProviders: *wire.AllowRemoteProviders}
	}
	if receipt.Correlation != nil {
		correlation := ExecutionCorrelation(*receipt.Correlation)
		out.Correlation = &correlation
	}
	for _, request := range receipt.Requests {
		usage := request.Usage
		wire := dto.ReceiptRequest{Session: string(request.Session), Index: request.Index, StartedAt: request.StartedAt.UTC(), Status: string(request.Status), Model: string(request.Model), Route: string(request.Route), Provider: usage.Provider, PromptTokens: usage.PromptTokens, CachedTokens: usage.CachedTokens, CacheWriteTokens: usage.CacheWriteTokens, CompletionTokens: usage.CompletionTokens, ReasoningTokens: usage.ReasoningTokens, Unmetered: usage.Unmetered}
		if request.Status != domain.ReceiptRequestInterrupted {
			duration := request.Duration.Milliseconds()
			wire.DurationMS = &duration
		}
		if usage.CostKnown {
			cost := usage.Cost
			wire.CostUSD = &cost
		}
		out.Requests = append(out.Requests, wire)
	}
	for _, call := range receipt.Calls {
		wire := dto.ReceiptCall{Session: string(call.Session), ID: string(call.Call.ID), Tool: string(call.Call.Tool), ArgumentsSHA256: call.ArgumentsSHA256, OutcomeSHA256: call.OutcomeSHA256, Decision: string(call.Call.Decision), Status: string(call.Call.Status)}
		if !call.StartedAt.IsZero() {
			started := call.StartedAt.UTC()
			wire.StartedAt = &started
			if call.Ended {
				duration := call.Duration.Milliseconds()
				wire.DurationMS = &duration
			}
		}
		out.Calls = append(out.Calls, wire)
	}
	return out
}

// jsonDigest is the hex sha256 of value's compact JSON.
func jsonDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func executionUsage(usage domain.ExecutionUsage) dto.ExecutionUsage {
	return dto.ExecutionUsage{Requests: usage.Requests, PromptTokens: usage.PromptTokens, CachedTokens: usage.CachedTokens, CacheWriteTokens: usage.CacheWriteTokens, CompletionTokens: usage.CompletionTokens, ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD, CostRequests: usage.CostRequests, Unmetered: usage.Unmetered}
}
