// Пакет предоставляет функциональность для управления бэкапами рабочих пространств в приложении AiPlan.
//
// Основные возможности:
//   - Экспорт бэкапов рабочих пространств в сжатом формате.
//   - Импорт бэкапов рабочих пространств из MinIO и локальных файлов.
//   - Обеспечение целостности и восстановления данных рабочих пространств.
//   - Хранение метаданных бэкапов (дата создания, пользователь, идентификатор).
package aiplan

import (
	"archive/zip"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BackupContext struct {
	AuthContext
	Backup dao.WorkspaceBackup
}

func (s *Services) BackupMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		//user := c.(AuthContext).User
		backupId := c.Param("backupId")

		var backup dao.WorkspaceBackup

		if err := s.db.
			Where("id = ?", backupId).
			First(&backup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrBackupNotFound)
			}
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		return next(BackupContext{c.(AuthContext), backup})
	}
}

func (s *Services) AddBackupServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug",
		s.WorkspaceMiddleware,
		s.LastVisitedWorkspaceMiddleware,
		s.WorkspacePermissionMiddleware,
	)

	backupGroup := g.Group("workspaces/backups/:backupId",
		s.BackupMiddleware,
		s.BackupPermissionMiddleware,
	)

	workspaceGroup.POST("/export/", s.exportWorkspace)
	workspaceGroup.GET("/backups/", s.getWorkspaceBackupList)

	backupGroup.GET("/", s.getWorkspaceBackup)
	backupGroup.DELETE("/", s.deleteWorkspaceBackup)

	backupGroup.POST("/restore/", s.restoreBackup)
	//g.POST("workspaces/importMinio/", s.importWorkspaceMinio)
	//g.POST("workspaces/import/", s.importWorkspaceFile)
	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})
	gob.Register(types.TimeZone{})
	gob.Register(types.TargetDateTimeZ{})
	gob.Register(types.TargetDate{})
}

func (s *Services) restoreBackup(c echo.Context) error {
	workspaceBackup := c.(BackupContext).Backup

	var workspace *dao.Workspace
	if err := s.db.Where("id = ?", workspaceBackup.WorkspaceId).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			workspace = nil
		} else {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
	}

	if workspace == nil {
		if err := s.db.Delete(&workspace).Error; err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
	}

	go func() {
		backup, _, err := unpackFromZip(s.storage, workspaceBackup.Asset)
		if err != nil {
			slog.Error("Unpack backup", "err", err)
		}
		if err := importWorkspaceBackup(s.db, *backup); err != nil {
			slog.Error("Import backup", "err", err)
		}
	}()

	return nil
}

func importWorkspaceBackup(db *gorm.DB, backup WorkspaceBackup) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//var originalWorkspace dao.Workspace
		//if err := tx.Where("id = ?", backup.Workspace.ID).First(&originalWorkspace).Error; err != nil {
		//	if err != gorm.ErrRecordNotFound {
		//		return err
		//	}
		//} else {
		//	//originalWorkspace.BackupWork = true
		//	if err := originalWorkspace.BeforeDelete(tx); err != nil {
		//		return err
		//	}
		//}
		//workspaceLogoId := backup.Workspace.LogoId
		//
		//if err := tx.Omit("LogoId").Save(&backup.Workspace).Error; err != nil {
		//	return err
		//}

		v := reflect.ValueOf(&backup).Elem()
		vType := reflect.ValueOf(WorkspaceBackup{}).Type()
		for i := 0; i < vType.NumField(); i++ {
			_, export := vType.Field(i).Tag.Lookup("importOrder")
			if !export {
				continue
			}

			if vType.Field(i).Name == "Workspace" || vType.Field(i).Name == "ProjectMembers" {
				continue
			}

			// skip empty slices
			if v.Field(i).Kind() == reflect.Slice && v.Field(i).Len() == 0 {
				continue
			}
			slog.Debug("Create workspace entity", "table", vType.Field(i).Name)
			if err := tx.Omit(clause.Associations).Save(v.Field(i).Interface()).Error; err != nil {
				return err
			}
		}
		//if workspaceLogoId.Valid {
		//	if err := tx.Select("LogoId").Save(&backup.Workspace).Error; err != nil {
		//		return err
		//	}
		//}
		return nil
	})
}

func unpackFromZip(storage filestorage.FileStorage, name uuid.UUID) (*WorkspaceBackup, bool, error) {
	info, err := storage.GetFileInfo(name)
	if err != nil {
		return nil, false, err
	}

	if info.ContentType != "application/zip" {
		return nil, false, fmt.Errorf("not a zip file")
	}

	tempDir, err := os.MkdirTemp("", "zip_extract_*")
	if err != nil {
		return nil, false, fmt.Errorf("create temp dir: %s", err.Error())
	}
	defer os.RemoveAll(tempDir)

	zipFilePath, err := downloadZipToTemp(storage, name, tempDir)
	if err != nil {
		return nil, false, fmt.Errorf("download zip: %s", err.Error())
	}

	extractedFiles, err := extractZipTemp(zipFilePath, tempDir)
	if err != nil {
		return nil, false, fmt.Errorf("extract zip: %s", err.Error())
	}

	backup, err := processExtractedFiles(storage, extractedFiles)
	if err != nil {
		return nil, false, err
	}
	return backup, true, nil
}

func downloadZipToTemp(storage filestorage.FileStorage, name uuid.UUID, tempDir string) (string, error) {
	zipFilePath := filepath.Join(tempDir, "archive.zip")

	reader, err := storage.LoadReader(name)
	if err != nil {
		return "", fmt.Errorf("load zip reader: %s", err.Error())
	}
	defer reader.(io.Closer).Close()

	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return "", fmt.Errorf("create temp zip file: %s", err.Error())
	}
	defer zipFile.Close()

	_, err = io.Copy(zipFile, reader)
	if err != nil {
		return "", fmt.Errorf("copy zip to temp: %s", err.Error())
	}

	return zipFilePath, nil
}

func extractZipTemp(zipFilePath, extractDir string) (map[string]string, error) {
	zipReader, err := zip.OpenReader(zipFilePath)
	if err != nil {
		return nil, fmt.Errorf("open zip file: %s", err.Error())
	}
	defer zipReader.Close()

	extractedFiles := make(map[string]string)

	for _, file := range zipReader.File {
		extractedPath := filepath.Join(extractDir, file.Name)

		if err := os.MkdirAll(filepath.Dir(extractedPath), 0755); err != nil {
			return nil, fmt.Errorf("create directory: %s", err.Error())
		}

		if err := extractZip(file, extractedPath); err != nil {
			return nil, fmt.Errorf("extract file %s: %s", file.Name, err.Error())
		}

		extractedFiles[file.Name] = extractedPath
		slog.Debug("File extracted", "name", file.Name, "path", extractedPath)
	}

	return extractedFiles, nil
}

func extractZip(zipFile *zip.File, outputPath string) error {
	rc, err := zipFile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, rc)
	return err
}

func processExtractedFiles(storage filestorage.FileStorage, extractedFiles map[string]string) (*WorkspaceBackup, error) {
	infoFiles := make(map[string]string) // baseName -> filePath
	dataFiles := make(map[string]string) // baseName -> filePath

	for fileName, filePath := range extractedFiles {
		if strings.HasSuffix(fileName, ".info") {
			baseName := strings.TrimSuffix(fileName, ".info")
			infoFiles[baseName] = filePath
		} else {
			baseName := fileName
			dataFiles[baseName] = filePath
		}
	}

	for baseName, infoFilePath := range infoFiles {
		dataFilePath, exists := dataFiles[baseName]
		if !exists {
			continue
		}

		fileUUID, err := uuid.FromString(baseName)
		if err != nil {
			continue
		}

		err = processFilePair(storage, fileUUID, infoFilePath, dataFilePath)
		if err != nil {
			return nil, fmt.Errorf("file %s: %s", fileUUID, err.Error())
		}
	}

	file, err := os.Open(extractedFiles["backup"])
	if err != nil {
		return nil, fmt.Errorf("open file: %s", err.Error())
	}
	defer file.Close()

	var backup WorkspaceBackup
	if err := gob.NewDecoder(file).Decode(&backup); err != nil {
		return nil, fmt.Errorf("decode backup: %s", err.Error())
	}

	return &backup, nil
}

func processFilePair(storage filestorage.FileStorage, fileUUID uuid.UUID, infoFilePath, dataFilePath string) error {
	data, err := os.ReadFile(infoFilePath)
	if err != nil {
		return fmt.Errorf("read info file: %s", err.Error())
	}

	var fileInfo filestorage.FileInfo
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&fileInfo); err != nil {
		return fmt.Errorf("decode gob: %s", err.Error())
	}

	file, err := os.Open(dataFilePath)
	if err != nil {
		return fmt.Errorf("open file: %s", err.Error())
	}
	defer file.Close()

	fileStat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("get file stat: %s", err.Error())
	}

	metadata := filestorage.ConvertToMetadata(fileInfo.UserTags)
	err = storage.SaveReader(file, fileStat.Size(), fileUUID, fileInfo.ContentType, metadata, fileInfo.UserMetadata)
	if err != nil {
		return fmt.Errorf("save to storage: %s", err.Error())
	}
	return nil
}

func (s *Services) exportWorkspace(c echo.Context) error {
	slug := c.Param("workspaceSlug")
	user := *c.(WorkspaceContext).User
	var workspace dao.Workspace
	if err := s.db.Preload("Owner").Where("slug = ?", slug).First(&workspace).Error; err != nil {
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

		if vType.Field(i).Name == "ProjectMembers" {
			query = query.Preload("Member").Preload("Workspace")
		}

		if vType.Field(i).Name == "EntityActivity" {
			query = query.Preload("Workspace").Preload("Issue").Preload("Project").Preload("Actor")
		}

		if vType.Field(i).Name == "LinkedIssues" {
			idIssues := s.db.Model(&dao.Issue{}).Select("id").Where("workspace_id = ?", workspace.ID)
			query = s.db.Where("id1 IN (?) OR id2 IN (?)", idIssues, idIssues)
		}

		if vType.Field(i).Name == "CommentReaction" {
			idComment := s.db.Model(&dao.IssueComment{}).Select("id").Where("workspace_id = ?", workspace.ID)
			query = s.db.Where("comment_id IN (?)", idComment)
		}

		if vType.Field(i).Name == "DocCommentReactions" {
			idComment := s.db.Model(&dao.DocComment{}).Select("id").Where("workspace_id = ?", workspace.ID)
			query = s.db.Where("comment_id IN (?)", idComment)
		}
		if vType.Field(i).Name == "FileAsset" {
			query = query.Where("id::text <> name")

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

	name := dao.GenUUID()
	var backupResp dao.WorkspaceBackup

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		backupResp = dao.WorkspaceBackup{
			ID:          dao.GenUUID(),
			CreatedAt:   backup.CreatedAt,
			CreatedBy:   backup.CreatedBy.ID,
			Asset:       name,
			Author:      &backup.CreatedBy,
			WorkspaceId: &workspace.ID,
			MetaData:    backup.GetMetadata(),
			InProgress:  true,
		}

		if err := tx.Omit(clause.Associations).Create(&backupResp).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		fmt.Println(err)
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	fileAsset := dao.FileAsset{
		Id:          name,
		CreatedAt:   time.Now(),
		CreatedById: &user.ID,
		Name:        name.String(),
		WorkspaceId: nil,
		ContentType: "application/zip",
	}

	go CreateBackup(s.db, s.storage, backup, fileAsset, backupResp.ID, name)

	return c.JSON(http.StatusOK, backupResp)
}

func CreateBackup(db *gorm.DB, storage filestorage.FileStorage, backup WorkspaceBackup, backupAsset dao.FileAsset, backupId uuid.UUID, name uuid.UUID) {
	info, err := CreateZip(storage, backup.FileAsset, backup, name)
	if err != nil {
		fmt.Println(err)
		return
	}

	backupAsset.FileSize = int(info.Size)

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&backupAsset).Error; err != nil {
			return err
		}

		if err := tx.Model(&dao.WorkspaceBackup{}).Where("id = ?", backupId).Update("in_progress", false).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return
	}
}

func CreateZip(storage filestorage.FileStorage, assets []dao.FileAsset, backup WorkspaceBackup, name uuid.UUID) (*filestorage.FileInfo, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		z := zip.NewWriter(pw)

		for _, asset := range assets {
			if err := addAssetToZip(storage, z, asset); err != nil {
				slog.Error("Error adding asset to zip", "asset", asset.Id, "err", err)
			}
		}

		if err := addBackupToZip(z, backup); err != nil {
			slog.Error("Error adding backup", "err", err)
		}

		if err := z.Close(); err != nil {
			slog.Error("Error closing zip writer", "err", err)
		}
	}()

	err := storage.SaveReader(pr, -1, name, "application/zip", &filestorage.Metadata{}, nil)
	if err != nil {
		pr.Close()
		return nil, fmt.Errorf("save to storage: %w", err)
	}

	info, err := storage.GetFileInfo(name)
	if err != nil {
		return nil, fmt.Errorf("get file info: %w", err)
	}

	return info, nil
}

func addAssetToZip(storage filestorage.FileStorage, z *zip.Writer, asset dao.FileAsset) error {
	info, err := storage.GetFileInfo(asset.Id)
	if err != nil {
		return fmt.Errorf("get file info: %w", err)
	}

	reader, err := storage.LoadReader(asset.Id)
	if err != nil {
		return fmt.Errorf("load reader: %w", err)
	}
	defer func() {
		if closer, ok := reader.(io.Closer); ok {
			closer.Close()
		}
	}()

	data, err := z.Create(asset.Id.String())
	if err != nil {
		return fmt.Errorf("create data entry: %w", err)
	}

	if _, err := io.Copy(data, reader); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	var metaBuf bytes.Buffer
	if err := gob.NewEncoder(&metaBuf).Encode(info); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	metaEntry, err := z.Create(asset.Id.String() + ".info")
	if err != nil {
		return fmt.Errorf("create meta entry: %w", err)
	}

	if _, err := io.Copy(metaEntry, &metaBuf); err != nil {
		return fmt.Errorf("copy metadata: %w", err)
	}

	return nil
}

func addBackupToZip(z *zip.Writer, backup WorkspaceBackup) error {
	backupData, err := z.Create("backup")
	if err != nil {
		return fmt.Errorf("create backup entry: %w", err)
	}

	if err := gob.NewEncoder(backupData).Encode(backup); err != nil {
		return fmt.Errorf("encode backup: %w", err)
	}

	return nil
}

func (s *Services) getWorkspaceBackupList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace

	var backups []dao.WorkspaceBackup
	if err := s.db.
		Where("workspace_backups.workspace_id = ?", workspace.ID).
		Where("in_progress = ?", false).
		Find(&backups).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, backups)
}

func (s *Services) getWorkspaceBackup(c echo.Context) error {
	backup := c.(BackupContext).Backup
	return c.JSON(http.StatusOK, backup)
}

func (s *Services) deleteWorkspaceBackup(c echo.Context) error {
	backup := c.(BackupContext).Backup

	fmt.Println("deleting backup", backup.Asset)

	if err := s.db.Delete(&backup).Error; err != nil {
		fmt.Println(err)
	}
	return c.NoContent(http.StatusOK)
}

// =====

func (wb WorkspaceBackup) GetMetadata() types.BackupMetadata {
	var fSize int64

	for _, asset := range wb.FileAsset {
		fSize += int64(asset.FileSize)
	}
	return types.BackupMetadata{
		WorkspaceId:         wb.Workspace.ID,
		WorkspaceSlug:       wb.Workspace.Slug,
		WorkspaceName:       wb.Workspace.Name,
		Users:               len(wb.Users),
		WorkspaceMembers:    len(wb.WorkspaceMembers),
		Forms:               len(wb.Forms),
		FormAnswer:          len(wb.FormAnswer),
		Docs:                len(wb.Docs),
		DocFavorites:        len(wb.DocFavorites),
		DocEditors:          len(wb.DocEditors),
		DocWatchers:         len(wb.DocWatchers),
		DocReaders:          len(wb.DocReaders),
		DocComments:         len(wb.DocComments),
		DocCommentReactions: len(wb.DocCommentReactions),
		Projects:            len(wb.Projects),
		ProjectFavorites:    len(wb.ProjectFavorites),
		ProjectMembers:      len(wb.ProjectMembers),
		States:              len(wb.States),
		Labels:              len(wb.Labels),
		RulesLog:            len(wb.RulesLog),
		Issues:              len(wb.Issues),
		IssueTemplates:      len(wb.IssueTemplates),
		IssueAssignees:      len(wb.IssueAssignees),
		IssueWatchers:       len(wb.IssueWatchers),
		IssueBlockers:       len(wb.IssueBlockers),
		IssueLabels:         len(wb.IssueLabels),
		IssueLinks:          len(wb.IssueLinks),
		IssueProperties:     len(wb.IssueProperties),
		IssueComments:       len(wb.IssueComments),
		CommentReaction:     len(wb.CommentReaction),
		LinkedIssues:        len(wb.LinkedIssues),
		Sprints:             len(wb.Sprints),
		SprintWatchers:      len(wb.SprintWatchers),
		SprintIssues:        len(wb.SprintIssues),
		FileAsset:           len(wb.FileAsset),
		IssueAttachment:     len(wb.IssueAttachment),
		FormAttachment:      len(wb.FormAttachment),
		DocAttachment:       len(wb.DocAttachment),
		WorkspaceActivities: len(wb.WorkspaceActivities),
		FormActivities:      len(wb.FormActivities),
		DocActivities:       len(wb.DocActivities),
		ProjectActivities:   len(wb.ProjectActivities),
		IssueActivities:     len(wb.IssueActivities),
		SprintActivities:    len(wb.SprintActivities),
		FileAssetSize:       fSize,
	}
}

// Content of workspace backup. Don't change order of fields(import will insert in this order)!!!
type WorkspaceBackup struct {
	CreatedAt time.Time
	CreatedBy dao.User

	Users            []dao.User            `importOrder:"1"`
	Workspace        dao.Workspace         `importOrder:"2"`
	WorkspaceMembers []dao.WorkspaceMember `importOrder:"4"`

	Forms      []dao.Form       `importOrder:"3"`
	FormAnswer []dao.FormAnswer `importOrder:"4"`

	Docs                []dao.Doc                `importOrder:"3"`
	DocFavorites        []dao.DocFavorites       `importOrder:"4"`
	DocEditors          []dao.DocEditor          `importOrder:"4"`
	DocWatchers         []dao.DocWatcher         `importOrder:"4"`
	DocReaders          []dao.DocReader          `importOrder:"4"`
	DocComments         []dao.DocComment         `importOrder:"4"`
	DocCommentReactions []dao.DocCommentReaction `importOrder:"4"`

	Projects         []dao.Project          `importOrder:"3"`
	ProjectFavorites []dao.ProjectFavorites `importOrder:"4"`
	ProjectMembers   []dao.ProjectMember    `importOrder:"4"`
	States           []dao.State            `importOrder:"4"`
	Labels           []dao.Label            `importOrder:"4"`
	RulesLog         []dao.RulesLog         `importOrder:"4"`

	Issues          []dao.Issue           `importOrder:"4"`
	IssueTemplates  []dao.IssueTemplate   `importOrder:"5"`
	IssueAssignees  []dao.IssueAssignee   `importOrder:"5"`
	IssueWatchers   []dao.IssueWatcher    `importOrder:"5"`
	IssueBlockers   []dao.IssueBlocker    `importOrder:"5"`
	IssueLabels     []dao.IssueLabel      `importOrder:"5"`
	IssueLinks      []dao.IssueLink       `importOrder:"5"`
	IssueProperties []dao.IssueProperty   `importOrder:"5"`
	IssueComments   []dao.IssueComment    `importOrder:"5"`
	CommentReaction []dao.CommentReaction `importOrder:"5"`
	LinkedIssues    []dao.LinkedIssues    `importOrder:"5"`

	Sprints        []dao.Sprint        `importOrder:"7"`
	SprintWatchers []dao.SprintWatcher `importOrder:"8"`
	SprintIssues   []dao.SprintIssue   `importOrder:"8"`

	FileAsset       []dao.FileAsset       `importOrder:"10"`
	IssueAttachment []dao.IssueAttachment `importOrder:"11"`
	FormAttachment  []dao.FormAttachment  `importOrder:"11"`
	DocAttachment   []dao.DocAttachment   `importOrder:"11"`

	WorkspaceActivities []dao.WorkspaceActivity `importOrder:"12"`
	FormActivities      []dao.FormActivity      `importOrder:"12"`
	DocActivities       []dao.DocActivity       `importOrder:"12"`
	ProjectActivities   []dao.ProjectActivity   `importOrder:"12"`
	IssueActivities     []dao.IssueActivity     `importOrder:"12"`
	SprintActivities    []dao.SprintActivity    `importOrder:"12"`

	//WorkspaceBackup []dao.WorkspaceBackup `importOrder:"5"`
}
