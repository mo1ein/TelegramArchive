package taskui

import "encoding/json"

type ChatEvent struct {
	Kind  string          `json:"kind"`
	Stage string          `json:"stage"`
	Items json.RawMessage `json:"items,omitempty"`
}
