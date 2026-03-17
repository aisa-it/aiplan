package statesflow

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func ParseGraph(db *gorm.DB, projectId uuid.UUID, graph types.StatesFlowGraph) error {
	var states []dao.State
	if err := db.Where("project_id = ?", projectId).Find(&states).Error; err != nil {
		return err
	}

	statesMap := make(map[uuid.UUID]dao.State, len(states))
	for _, state := range states {
		state.FromStates = types.UUIDArray{}
		statesMap[state.ID] = state
	}

	for _, edge := range graph.Edges {
		targetState, ok := statesMap[edge.Target]
		if !ok {
			continue
		}
		targetState.FromStates.Array = append(targetState.FromStates.Array, edge.Source)
		statesMap[edge.Target] = targetState
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, state := range statesMap {
			if err := tx.Model(&state).Select("from_states").Updates(&state).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
