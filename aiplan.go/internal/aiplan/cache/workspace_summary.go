package cache

import (
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type WorkspaceSummaryCache struct {
	db *gorm.DB
	m  sync.RWMutex
	c  map[uuid.UUID]dto.WorkspaceSummary
}
