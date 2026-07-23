package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"gorm.io/gorm"
)

type ActivityNotification struct {
	emailService *EmailService
	templateSvc  *TemplateService
	processors   map[types.EntityLayer]EmailProcessor
}

func NewActivityNotificationService(db *gorm.DB, emailSvc *EmailService) *ActivityNotification {
	templateSvc := NewTemplateService(db)
	processors := make(map[types.EntityLayer]EmailProcessor)

	return &ActivityNotification{
		emailService: emailSvc,
		templateSvc:  templateSvc,
		processors:   processors,
	}
}

func (an *ActivityNotification) RegisterProcessor(layerType types.EntityLayer, processor EmailProcessor) {
	an.processors[layerType] = processor
}

func (an *ActivityNotification) ProcessLayer(layerType types.EntityLayer) {
	processor, ok := an.processors[layerType]
	if !ok {
		return
	}

	templates := an.templateSvc.LoadTemplates()
	ProcessLayer(an.emailService, processor, templates)
}

func InitActivityNotificationService(db *gorm.DB, es *EmailService) *ActivityNotification {
	an := NewActivityNotificationService(db, es)

	an.RegisterProcessor(NewIssuePipeline())
	an.RegisterProcessor(NewSprintPipeline())
	an.RegisterProcessor(NewProjectPipeline())
	an.RegisterProcessor(NewWorkspacePipeline())
	an.RegisterProcessor(NewDocPipeline())
	an.RegisterProcessor(NewSkipActivitiesPipeline())
	return an
}

func (es *EmailService) EmailActivity() {
	if es.cfg.EmailActivityDisabled {
		return
	}

	es.an.ProcessLayer(types.LayerIssue)
	es.an.ProcessLayer(types.LayerSprint)
	es.an.ProcessLayer(types.LayerProject)
	es.an.ProcessLayer(types.LayerWorkspace)
	es.an.ProcessLayer(types.LayerDoc)
	es.an.ProcessLayer(layerSkip)
}
