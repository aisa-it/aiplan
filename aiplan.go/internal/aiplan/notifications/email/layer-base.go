package email

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type BaseProcessor struct {
	SubjectTemplate string
	HeadTemplate    string
}

func (bp *BaseProcessor) FullLoad(_ *gorm.DB, entity dao.IDaoAct) dao.IDaoAct {
	return entity
}

func (bp *BaseProcessor) BuildSubject(entity dao.IDaoAct) string {
	if bp.SubjectTemplate != "" {
		return fmt.Sprintf(bp.SubjectTemplate, entity.GetString())
	}
	return "Обновление"
}

func (bp *BaseProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	if bp.HeadTemplate != "" {
		// Можно реализовать рендеринг, но пока заглушка
		return bp.HeadTemplate
	}
	return "Заголовок"
}

type ProcessorOption func(*BaseProcessor)

func NewBaseProcessor(opts ...ProcessorOption) *BaseProcessor {
	bp := &BaseProcessor{}
	for _, opt := range opts {
		opt(bp)
	}
	return bp
}
