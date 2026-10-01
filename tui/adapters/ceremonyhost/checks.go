package ceremonyhost

import (
	"context"
	"encoding/json"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	checkTimeoutMS = 600000
	checkOutput    = 16 << 10
	checkTail      = 4 << 10
)

// Checks is application.CheckRunnerPort over AXLR's local exec runtime.
type Checks struct{ Tools application.ToolExecutionPort }

var _ application.CheckRunnerPort = Checks{}

func (c Checks) Run(ctx context.Context, command domain.CheckCommand) (application.CheckResult, error) {
	id, err := domain.NewLocalToolIdentity("exec")
	if err != nil {
		return application.CheckResult{}, err
	}
	args := command.Args
	if args == nil {
		args = []string{}
	}
	encoded, err := json.Marshal(map[string]any{"program": command.Program, "args": args, "timeout_ms": checkTimeoutMS, "max_output_bytes": checkOutput})
	if err != nil {
		return application.CheckResult{}, err
	}
	value, err := root.NewJSONObject(encoded)
	if err != nil {
		return application.CheckResult{}, err
	}
	outcome, err := c.Tools.Execute(ctx, id, value)
	if err != nil {
		return application.CheckResult{}, err
	}
	var envelope struct {
		Status string `json:"status"`
		Output *struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &envelope); err != nil {
		return application.CheckResult{}, fmt.Errorf("unreadable check result")
	}
	if envelope.Output == nil {
		// The program did not run (not found, timed out, refused): a failed check.
		message := envelope.Status
		if envelope.Error != nil {
			message = envelope.Error.Message
		}
		return application.CheckResult{ExitCode: -1, Output: tail(message)}, nil
	}
	return application.CheckResult{ExitCode: envelope.Output.ExitCode, Output: tail(envelope.Output.Stdout + envelope.Output.Stderr)}, nil
}

func tail(text string) string {
	if len(text) <= checkTail {
		return text
	}
	cut := len(text) - checkTail
	for cut < len(text) && text[cut]&0xC0 == 0x80 {
		cut++
	}
	return "…" + text[cut:]
}
