package tracker

import (
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/opt"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffMasksSecretField(t *testing.T) {
	workspaceID := uuid.Must(uuid.NewV4())

	oldSnapshot := WorkspaceSnapshot{
		ID:    workspaceID,
		Name:  opt.Some("Workspace"),
		Token: opt.Some("old-secret-token"),
	}

	newSnapshot := WorkspaceSnapshot{
		ID:    workspaceID,
		Name:  opt.Some("Workspace Updated"),
		Token: opt.Some("new-secret-token"),
	}

	changes := Diff(oldSnapshot, newSnapshot, workspaceID, "Workspace")

	var tokenChange *FieldChange
	for i := range changes {
		if changes[i].Field == "integration_token" {
			tokenChange = &changes[i]
			break
		}
	}

	require.NotNil(t, tokenChange, "not nil")

	assert.NotEqual(t, "old-secret-token", tokenChange.OldVal, "токен должен быть замаскирован")
	assert.NotEqual(t, "new-secret-token", tokenChange.NewVal, " токен должен быть замаскирован")

	assert.Equal(t, "ol***en", tokenChange.OldVal, "старый токен должен быть в формате 'ol***en'")
	assert.Equal(t, "ne***en", tokenChange.NewVal, "новый токен должен быть в формате 'ne***en'")
}
