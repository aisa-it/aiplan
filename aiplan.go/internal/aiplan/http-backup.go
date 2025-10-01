// Пакет предоставляет функциональность для управления бэкапами рабочих пространств в приложении AiPlan.
//
// Основные возможности:
//   - Экспорт бэкапов рабочих пространств в сжатом формате.
//   - Импорт бэкапов рабочих пространств из MinIO и локальных файлов.
//   - Обеспечение целостности и восстановления данных рабочих пространств.
//   - Хранение метаданных бэкапов (дата создания, пользователь, идентификатор).
package aiplan

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Content of workspace backup. Don't change order of fields(import will insert in this order)!!!
type WorkspaceBackup struct {
	CreatedAt time.Time
	CreatedBy dao.User

	Users            []dao.User            `importOrder:"1"`
	Workspace        dao.Workspace         `importOrder:"2"`
	WorkspaceMembers []dao.WorkspaceMember `importOrder:"4"`
	Projects         []dao.Project         `importOrder:"3"`
	States           []dao.State           `importOrder:"4"`
	Issues           []dao.Issue           `importOrder:"4"`

	EstimatePoints []dao.EstimatePoint `importOrder:"4"`

	//FileAssets []dao.FileAsset `importOrder:"4"`

	Labels []dao.Label `importOrder:"4"`

	ProjectFavorites []dao.ProjectFavorites `importOrder:"4"`
	ProjectMembers   []dao.ProjectMember    `importOrder:"4"`

	IssueActivities []dao.EntityActivity `importOrder:"5"`
	IssueAssignees  []dao.IssueAssignee  `importOrder:"5"`
	IssueWatchers   []dao.IssueWatcher   `importOrder:"5"`
	IssueBlockers   []dao.IssueBlocker   `importOrder:"5"`
	IssueComments   []dao.IssueComment   `importOrder:"5"`
	IssueLabels     []dao.IssueLabel     `importOrder:"5"`
	IssueLinks      []dao.IssueLink      `importOrder:"5"`
	IssueProperties []dao.IssueProperty  `importOrder:"5"`

	Estimates []dao.Estimate `importOrder:"5"`
}

func (s *Services) AddBackupServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug",
		s.WorkspaceMiddleware,
		s.LastVisitedWorkspaceMiddleware,
		s.WorkspacePermissionMiddleware,
	)

	workspaceGroup.GET("/backups/", s.getWorkspaceBackupList)
	workspaceGroup.POST("/export/", s.exportWorkspace)
	g.POST("workspaces/importMinio/", s.importWorkspaceMinio)
	g.POST("workspaces/import/", s.importWorkspaceFile)

	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})
}

// getWorkspaceBackupList godoc
// Deprecated
// @ Deprecated
func (s *Services) getWorkspaceBackupList(c echo.Context) error {
	// @id getWorkspaceBackupList
	// @Summary Пространство (бэкапы): получение всех бекапов рабочего пространства
	// @Description Возвращает список всех бекапов указанного рабочего пространства
	// @Tags Workspace
	// @Security ApiKeyAuth
	// @Accept json
	// @Produce json
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Success 200 {array} dao.WorkspaceBackup "Список бекапов рабочего пространства"
	// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
	// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
	// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
	// @Router /api/workspaces/{workspaceSlug}/backups [get]
	slug := c.Param("workspaceSlug")

	var backups []dao.WorkspaceBackup
	if err := s.db.Joins("Workspace").Where("slug = ?", slug).Find(&backups).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, backups)
}

// exportWorkspace godoc
// Deprecated
// @ Deprecated
func (s *Services) exportWorkspace(c echo.Context) error {
	// @id exportWorkspace
	// @Summary Пространство (бэкапы): создание бекапа рабочего пространства
	// @Description Создает бекап указанного рабочего пространства и сохраняет его
	// @Tags Workspace
	// @Security ApiKeyAuth
	// @Accept json
	// @Produce json
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Success 200 {object} dao.WorkspaceBackup "Созданный бекап рабочего пространства"
	// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
	// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
	// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
	// @Router /api/workspaces/{workspaceSlug}/export [post]
	slug := c.Param("workspaceSlug")
	user := *c.(AuthContext).User

	var workspace dao.Workspace
	if err := s.db.Preload("Owner").
		Where("slug = ?", slug).
		First(&workspace).Error; err != nil {
		return EError(c, err)
	}

	backup := WorkspaceBackup{
		CreatedBy: user,
		CreatedAt: time.Now(),
		Workspace: workspace,
	}
	v := reflect.ValueOf(&backup).Elem()
	vType := reflect.ValueOf(WorkspaceBackup{}).Type()
	for i := 0; i < vType.NumField(); i++ {
		_, export := vType.Field(i).Tag.Lookup("importOrder")
		if !export {
			continue
		}

		if vType.Field(i).Name == "Workspace" || vType.Field(i).Name == "Users" {
			continue
		}

		query := s.db.Where("workspace_id = ?", workspace.ID)

		// Preload users
		if vType.Field(i).Name == "WorkspaceMembers" {
			query = query.Preload("Member")
		}

		vv := reflect.New(vType.Field(i).Type)
		if err := query.Find(vv.Interface()).Error; err != nil {
			return EError(c, err)
		}
		v.Field(i).Set(vv.Elem())
	}
	for _, user := range backup.WorkspaceMembers {
		backup.Users = append(backup.Users, *user.Member)
	}

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	if err := gob.NewEncoder(gw).Encode(backup); err != nil {
		return EError(c, err)
	}
	gw.Flush()
	gw.Close()

	name := dao.GenUUID()
	if err := s.storage.SaveReader(buf, int64(buf.Len()), name, "application/x-binary", nil); err != nil {
		return EError(c, err)
	}

	backupResp := dao.WorkspaceBackup{
		ID:          dao.GenID(),
		CreatedAt:   backup.CreatedAt,
		CreatedBy:   backup.CreatedBy.ID,
		Asset:       name,
		Author:      &backup.CreatedBy,
		WorkspaceId: workspace.ID,
	}
	if err := s.db.Omit(clause.Associations).Create(&backupResp).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, backupResp)
}

// importWorkspaceMinio godoc
// Deprecated
// @ Deprecated
func (s *Services) importWorkspaceMinio(c echo.Context) error {
	//@id importWorkspaceMinio
	//@Summary Пространство (бэкапы): импорт рабочего пространства из файла на MinIO
	//@Description Импортирует рабочее пространство из указанного файла, хранящегося на MinIO
	//@Tags Workspace
	//@Security ApiKeyAuth
	//@Accept json
	//@Produce json
	//@Param data body dao.WorkspaceBackup true "Данные бекапа рабочего пространства"
	//@Success 200 "Рабочее пространство успешно импортировано"
	//@Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
	//@Failure 404 {object} apierrors.DefinedError "Файл бекапа не найден"
	//@Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
	//@Router /api/workspaces/importMinio [post]
	var request dao.WorkspaceBackup
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	backupReader, err := s.storage.LoadReader(request.Asset)
	if err != nil {
		return EError(c, err)
	}

	gr, err := gzip.NewReader(backupReader)
	if err != nil {
		return EError(c, err)
	}

	var backup WorkspaceBackup
	if err := gob.NewDecoder(gr).Decode(&backup); err != nil {
		return EError(c, err)
	}

	if err := s.importWorkspaceBackup(backup); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// importWorkspaceFile godoc
// Deprecated
// @ Deprecated
func (s *Services) importWorkspaceFile(c echo.Context) error {
	// @id importWorkspaceFile
	// @Summary Пространство (бэкапы): импорт рабочего пространства из загружаемого файла
	// @Description Импортирует рабочее пространство из загруженного файла бекапа
	// @Tags Workspace
	// @Security ApiKeyAuth
	// @Accept multipart/form-data
	// @Produce json
	// @Param backup formData file true "Файл бекапа рабочего пространства"
	// @Success 200 "Рабочее пространство успешно импортировано"
	// @Failure 400 {object} apierrors.DefinedError "Некорректный файл бекапа"
	// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
	// @Router /api/workspaces/import [post]
	backupData, err := c.FormFile("backup")
	if err != nil {
		return EError(c, err)
	}

	backupReader, err := backupData.Open()
	if err != nil {
		return EError(c, err)
	}
	defer backupReader.Close()

	gr, err := gzip.NewReader(backupReader)
	if err != nil {
		return EError(c, err)
	}

	var backup WorkspaceBackup
	if err := gob.NewDecoder(gr).Decode(&backup); err != nil {
		return EError(c, err)
	}

	if err := s.importWorkspaceBackup(backup); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (s *Services) importWorkspaceBackup(backup WorkspaceBackup) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		slog.Debug("Remove old workspace", "id", backup.Workspace.ID)
		var originalWorkspace dao.Workspace
		if err := tx.Where("id = ?", backup.Workspace.ID).First(&originalWorkspace).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
		} else {
			if err := originalWorkspace.BeforeDelete(tx); err != nil {
				return err
			}
		}

		slog.Debug("Create workspace")
		if err := tx.Save(&backup.Workspace).Error; err != nil {
			return err
		}

		v := reflect.ValueOf(&backup).Elem()
		vType := reflect.ValueOf(WorkspaceBackup{}).Type()
		for i := 0; i < vType.NumField(); i++ {
			_, export := vType.Field(i).Tag.Lookup("importOrder")
			if !export {
				continue
			}

			if vType.Field(i).Name == "Workspace" {
				continue
			}

			// skip empty slices
			if v.Field(i).Kind() == reflect.Slice && v.Field(i).Len() == 0 {
				continue
			}

			slog.Debug("Create workspace entity", "table", vType.Field(i).Name)
			if err := tx.Save(v.Field(i).Interface()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
