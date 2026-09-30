package axlr

import (
	"context"
	"encoding/json"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// prepareChange uses AXLR's bounded, rooted read path. A preview failure never
// blocks the requested effect. The resulting digest must prove the snapshot
// matches the write that actually completed before review evidence is attached.
func (r ToolRunner) prepareChange(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) *domain.FileChange {
	if id.Kind != domain.ToolKindLocal || (id.LocalOperation != "write" && id.LocalOperation != "edit") {
		return nil
	}
	var input struct {
		Path, Content, Mode string
		OldText             string `json:"old_text"`
		NewText             string `json:"new_text"`
		ExpectedSHA256      string `json:"expected_sha256"`
	}
	if json.Unmarshal(args.Bytes(), &input) != nil {
		return nil
	}
	path, err := root.NewRelativePath(input.Path)
	if err != nil {
		return nil
	}
	c := &domain.FileChange{Path: path, Created: id.LocalOperation == "write" && input.Mode == "create"}
	if !c.Created {
		readArgs, _ := json.Marshal(dto.ReadArgs{Path: input.Path, MaxBytes: domain.MaxChangePreviewBytes})
		response := r.Executor.Execute(ctx, dto.Request{ProtocolVersion: runtime.ProtocolVersion, RequestID: "tui-change-preview", Tool: "read", Arguments: readArgs})
		before, ok := response.Output.(dto.ReadOutput)
		if response.Status != "completed" || !ok {
			c.Unavailable = "unavailable"
			return c
		}
		if before.Truncated || !domain.ChangePreviewFits(before.Content) {
			c.Unavailable = "too_large"
			return c
		}
		c.Before = root.Text(before.Content)
		if input.ExpectedSHA256 != "" && before.ContentSHA256 != input.ExpectedSHA256 {
			c.Before, c.Unavailable = "", "unavailable"
			return c
		}
	}
	if id.LocalOperation == "write" {
		c.After = root.Text(input.Content)
	} else {
		if input.OldText == "" || strings.Count(string(c.Before), input.OldText) != 1 {
			c.Before = ""
			c.Unavailable = "unavailable"
			return c
		}
		c.After = root.Text(strings.Replace(string(c.Before), input.OldText, input.NewText, 1))
	}
	if !domain.ChangePreviewFits(string(c.After)) {
		c.Before, c.After, c.Unavailable = "", "", "too_large"
	}
	return c
}

func completedChange(c *domain.FileChange, response dto.Response) *domain.FileChange {
	if c == nil || response.Status != "completed" {
		return nil
	}
	output, ok := response.Output.(dto.WriteOutput)
	if !ok {
		return nil
	}
	if c.Unavailable == "" && output.ContentSHA256 != string(root.DigestOf([]byte(c.After))) {
		c.Before, c.After, c.Unavailable = "", "", "unavailable"
	}
	if c.Unavailable == "" && !c.Created && c.Before == c.After {
		return nil
	}
	return c
}
