package axlr

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	root "github.com/underpass-ai/AXLR/domain"
)

const maxCompiledToolSchemas = 256
const maxToolSchemaBytes = 128 * 1024

// ToolArgumentValidator checks the frozen plugin schema locally, without
// network access. Compiled schemas are immutable and retained in a bounded
// cache. Unsupported assertion semantics fail closed rather than being ignored.
type ToolArgumentValidator struct {
	mu      sync.Mutex
	schemas map[[32]byte]*compiledToolSchema
	order   [][32]byte
}

func NewToolArgumentValidator() *ToolArgumentValidator { return &ToolArgumentValidator{} }

func (v *ToolArgumentValidator) Validate(definition root.ToolDefinition, arguments root.JSONValue) error {
	if v == nil {
		return errors.New("plugin argument validator is unavailable")
	}
	if _, err := root.NewJSONObject(arguments.Bytes()); err != nil {
		return errors.New("plugin arguments must be an object")
	}
	instance, err := strictToolJSON(arguments.Bytes())
	if err != nil {
		return fmt.Errorf("invalid plugin arguments: %w", err)
	}
	raw := definition.Parameters.Bytes()
	if len(raw) == 0 || len(raw) > maxToolSchemaBytes {
		return errors.New("plugin schema is missing or exceeds 128 KiB")
	}
	key := sha256.Sum256(raw)
	v.mu.Lock()
	resolved := v.schemas[key]
	if resolved == nil {
		resolved, err = compileToolSchema(raw)
		if err == nil {
			if v.schemas == nil {
				v.schemas = make(map[[32]byte]*compiledToolSchema)
			}
			if len(v.order) == maxCompiledToolSchemas {
				delete(v.schemas, v.order[0])
				v.order = v.order[1:]
			}
			v.schemas[key] = resolved
			v.order = append(v.order, key)
		}
	}
	v.mu.Unlock()
	if err != nil {
		return fmt.Errorf("unsupported or invalid plugin schema: %w", err)
	}
	instance, err = exactToolNumbers(instance, resolved.bounds, false)
	if err != nil {
		return fmt.Errorf("unsupported argument precision: %w", err)
	}
	if err := resolved.resolved.Validate(instance); err != nil {
		return fmt.Errorf("plugin arguments do not match the registered schema: %w", err)
	}
	return nil
}

func compileToolSchema(raw []byte) (*compiledToolSchema, error) {
	value, err := strictToolJSON(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("plugin schema must be an object")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	bounds := map[float64]bool{}
	if err := normalizeToolSchema(&schema, value, bounds); err != nil {
		return nil, err
	}
	if err := checkToolReferences(&schema); err != nil {
		return nil, err
	}
	// A nil Loader explicitly prevents external reference resolution. Internal
	// fragment references remain supported after the strict compatibility check.
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	return &compiledToolSchema{resolved: resolved, bounds: bounds}, nil
}
