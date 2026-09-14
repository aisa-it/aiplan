package utils

import (
	"testing"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCalculateIDChanges(t *testing.T) {
	t.Run("empty slices", func(t *testing.T) {
		reqIDs := []any{}
		curIDs := []any{}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.Empty(t, result.AddIds)
		assert.Empty(t, result.DelIds)
		assert.Empty(t, result.InvolvedIds)
	})

	t.Run("add new IDs", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id2}
		curIDs := []any{}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, result.AddIds)
		assert.Empty(t, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, result.InvolvedIds)
	})

	t.Run("remove existing IDs", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())

		reqIDs := []any{}
		curIDs := []any{id1, id2}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.Empty(t, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id1, id2}, result.InvolvedIds)
	})

	t.Run("no changes", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id2}
		curIDs := []any{id1, id2}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.Empty(t, result.AddIds)
		assert.Empty(t, result.DelIds)
		assert.Empty(t, result.InvolvedIds)
	})

	t.Run("mixed changes - add and remove", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())
		id3 := uuid.Must(uuid.NewV4())
		id4 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id3, id4}
		curIDs := []any{id1, id2}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id3, id4}, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id2}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2, id3, id4}, result.InvolvedIds)
	})

	t.Run("string UUIDs", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())
		id3 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1.String(), id2.String()}
		curIDs := []any{id1.String(), id3.String()}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id2}, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id3}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2, id3}, result.InvolvedIds)
	})

	t.Run("mixed UUID types - strings and UUID objects", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())
		id3 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id2.String()}
		curIDs := []any{id1.String(), id3}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id2}, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id3}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2, id3}, result.InvolvedIds)
	})

	t.Run("duplicate IDs in request", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id1, id2}
		curIDs := []any{id1}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id2}, result.AddIds)
		assert.Empty(t, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2}, result.InvolvedIds)
	})

	t.Run("duplicate IDs in current", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1}
		curIDs := []any{id1, id1, id2}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.Empty(t, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id2}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2}, result.InvolvedIds)
	})

	t.Run("complex scenario with multiple operations", func(t *testing.T) {
		id1 := uuid.Must(uuid.NewV4())
		id2 := uuid.Must(uuid.NewV4())
		id3 := uuid.Must(uuid.NewV4())
		id4 := uuid.Must(uuid.NewV4())

		id5 := uuid.Must(uuid.NewV4())

		reqIDs := []any{id1, id3, id4, id5}
		curIDs := []any{id1, id2, id3}

		result := CalculateIDChanges(reqIDs, curIDs)

		assert.ElementsMatch(t, []uuid.UUID{id4, id5}, result.AddIds)
		assert.ElementsMatch(t, []uuid.UUID{id2}, result.DelIds)
		assert.ElementsMatch(t, []uuid.UUID{id2, id4, id5}, result.InvolvedIds)
	})
}
