package manifest

import (
	"bytes"
	"encoding/json"
)

// Optional distinguishes an omitted hand-written field from explicit null.
// Omitted means "do not modify" during import; null means "clear".
type Optional[T any] struct {
	Present bool
	Null    bool
	Value   T
}

func (value *Optional[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Null = true
		var zero T
		value.Value = zero
		return nil
	}
	value.Null = false
	return json.Unmarshal(data, &value.Value)
}
