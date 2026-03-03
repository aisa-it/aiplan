package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofrs/uuid"
)

type StatesFlowGraph struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
}

type FlowNode struct {
	Id       uuid.UUID `json:"id"`
	Type     string    `json:"type,omitempty"`
	Label    string    `json:"label,omitempty"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

type FlowEdge struct {
	Id       string    `json:"id"`
	Source   uuid.UUID `json:"source"`
	Target   uuid.UUID `json:"target"`
	Animated bool      `json:"animated"`
}

func (fn StatesFlowGraph) Value() (driver.Value, error) {
	b, err := json.Marshal(fn)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (fn *StatesFlowGraph) Scan(value any) error {
	if value == nil {
		*fn = StatesFlowGraph{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	if err := json.Unmarshal(bytes, fn); err != nil {
		return err
	}
	return nil
}
