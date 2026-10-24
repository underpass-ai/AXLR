package contract

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// ParseExecutionCorrelation reads the contract's correlation object,
// strictly: an unknown field or an unsafe id refuses it. The object may
// come wrapped as {"correlation": {...}}.
func ParseExecutionCorrelation(raw []byte) (domain.ExecutionCorrelation, error) {
	if len(raw) > MaxExecutionObjectBytes {
		return domain.ExecutionCorrelation{}, errors.New("the correlation exceeds 4 KiB")
	}
	var wrapped struct {
		Correlation json.RawMessage `json:"correlation"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Correlation != nil {
		raw = wrapped.Correlation
	}
	var wire dto.ExecutionCorrelation
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil {
		return domain.ExecutionCorrelation{}, errors.New("the correlation must be the contract's object: bot_id, task_id, execution_id and trace_id, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ExecutionCorrelation{}, errors.New("trailing data after the correlation")
	}
	return DomainExecutionCorrelation(wire)
}

// DomainExecutionCorrelation validates a wire correlation into the domain's.
func DomainExecutionCorrelation(wire dto.ExecutionCorrelation) (domain.ExecutionCorrelation, error) {
	correlation := domain.ExecutionCorrelation{BotID: wire.BotID, TaskID: wire.TaskID, ExecutionID: wire.ExecutionID, TraceID: wire.TraceID}
	return correlation, correlation.Validate()
}

// ExecutionCorrelation is the wire form of a correlation, as every event
// and the result carry it.
func ExecutionCorrelation(correlation domain.ExecutionCorrelation) dto.ExecutionCorrelation {
	return dto.ExecutionCorrelation{BotID: correlation.BotID, TaskID: correlation.TaskID, ExecutionID: correlation.ExecutionID, TraceID: correlation.TraceID}
}
