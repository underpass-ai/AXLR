package axlr

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
)

// jsonschema-go v0.4.3 classifies json.Number as a string. Preserve integers as
// int64/uint64, and permit a decimal projection only when rounding cannot change
// its type or comparison with any numeric assertion. Schema literals require
// exact representation. Execution always receives the original JSON bytes.
func exactToolNumbers(value any, bounds map[float64]bool, literal bool) (any, error) {
	switch value := value.(type) {
	case json.Number:
		rat, ok := new(big.Rat).SetString(value.String())
		if !ok {
			return nil, errors.New("JSON number cannot be represented exactly")
		}
		if rat.IsInt() {
			integer := rat.Num()
			if integer.IsInt64() {
				return integer.Int64(), nil
			}
			if integer.IsUint64() {
				return integer.Uint64(), nil
			}
		}
		f, err := value.Float64()
		converted := new(big.Rat).SetFloat64(f)
		if err != nil || converted == nil {
			return nil, errors.New("JSON number exceeds exact schema-validator precision")
		}
		if converted.Cmp(rat) != 0 && (literal || rat.IsInt() || math.Trunc(f) == f || bounds[f]) {
			return nil, errors.New("JSON number rounds across a schema assertion or integer boundary")
		}
		return f, nil
	case []any:
		for i, item := range value {
			converted, err := exactToolNumbers(item, bounds, literal)
			if err != nil {
				return nil, err
			}
			value[i] = converted
		}
		return value, nil
	case map[string]any:
		for name, item := range value {
			converted, err := exactToolNumbers(item, bounds, literal)
			if err != nil {
				return nil, err
			}
			value[name] = converted
		}
		return value, nil
	default:
		return value, nil
	}
}
