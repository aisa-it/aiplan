// Пакет предоставляет интерфейсы и реализации для интеграций с различными сервисами.
//
//	Он включает в себя регистрацию веб-хуков, получение информации об интеграциях и управление данными пользователей.
//
// Основные возможности:
//   - Регистрация веб-хуков для различных интеграций.
//   - Получение информации об интеграциях, включая имя, описание и пользователя.
//   - Управление данными пользователей, связанными с интеграциями (например, создание и обновление).
//   - Загрузка логотипов интеграций.
//   - Проверка подключения интеграции к рабочему пространству.
package integrations

import (
	"fmt"
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type IntegrationInterface interface {
	RegisterWebhook(g *echo.Group)
	GetInfo() Integration
}

type Integration struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	User      *dao.User `json:"-"`
	AvatarSVG string    `json:"avatar"`
	Added     bool      `json:"added,omitempty"`

	db              *gorm.DB
	telegramService *notifications.TelegramService
	fileStorage     filestorage.FileStorage
	tracker         *tracker.ActivitiesTracker
	bl              *business.Business
}

func (i Integration) GetInfo() Integration {
	return i
}

func (i *Integration) FetchUser() error {
	if err := i.db.Where("username = ?", i.User.Username).First(i.User).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			i.User.ID = dao.GenID()

			if err := i.db.Create(i.User).Error; err != nil {
				return err
			}
			return i.UploadIntegrationLogo()
		}
	}
	return nil
}

func (i *Integration) UploadIntegrationLogo() error {
	assetName := fmt.Sprintf("%s_logo.svg", *i.User.Username)

	var asset dao.FileAsset
	if err := i.db.Where("name = ?", assetName).First(&asset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			asset = dao.FileAsset{
				Id:          dao.GenUUID(),
				Name:        assetName,
				CreatedById: &i.User.ID,
				FileSize:    len(i.AvatarSVG),
				ContentType: "image/svg+xml",
			}

			if err := i.fileStorage.Save([]byte(i.AvatarSVG), asset.Id, asset.ContentType, nil); err != nil {
				return err
			}

			if err = i.db.Create(&asset).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}

	i.User.AvatarId = uuid.NullUUID{Valid: true, UUID: asset.Id}
	return i.db.Model(i.User).Update("avatar_id", asset.Id).Error
}

type IntegrationsService struct {
	integrations []IntegrationInterface
	db           *gorm.DB
}

func NewIntegrationService(g *echo.Group, db *gorm.DB, tS *notifications.TelegramService, fs filestorage.FileStorage, tr *tracker.ActivitiesTracker, bl *business.Business) *IntegrationsService {
	integrations := []IntegrationInterface{
		NewGitlabIntegration(db, tS, fs, tr, bl),
		NewGithubIntegration(db, tS, fs, tr, bl),
	}

	webhooksGroup := g.Group("integrations/webhooks/")
	for _, integration := range integrations {
		integration.RegisterWebhook(webhooksGroup)
	}
	return &IntegrationsService{db: db, integrations: integrations}
}

func (is *IntegrationsService) GetIntegrations(workspaceId string) []Integration {
	integrations := make([]Integration, len(is.integrations))
	for i, integration := range is.integrations {
		integrations[i] = integration.GetInfo()
		if err := is.db.Select("count(*) > 0").
			Where("member_id = ?", integration.GetInfo().User.ID).
			Where("workspace_id = ?", workspaceId).
			Model(&dao.WorkspaceMember{}).Find(&integrations[i].Added).Error; err != nil {
			slog.Error("Check if integration connected to workspace", "integration", integrations[i].Name, "workspaceId", workspaceId, "err", err)
		}
	}
	return integrations
}

func (is *IntegrationsService) GetIntegrationUser(name string) *dao.User {
	var user *dao.User
	for _, i := range is.integrations {
		if i.GetInfo().Name == name {
			user = i.GetInfo().User
		}
	}
	return user
}
