package aiplan

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func (s *Services) getSwaggerJSON(c echo.Context) error {
	f, err := os.Open("docs/swagger.json")
	if err != nil {
		return EError(c, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	data := make(map[string]interface{}, 0)
	if err := dec.Decode(&data); err != nil {
		return EError(c, err)
	}
	data["host"] = cfg.WebURL
	return c.JSON(http.StatusOK, data)
}

func (s *Services) uploadAssetForm(tx *gorm.DB, file *multipart.FileHeader, dstAsset *dao.FileAsset, metadata filestorage.Metadata) error {
	assetSrc, err := file.Open()
	if err != nil {
		return err
	}
	defer assetSrc.Close()

	if dstAsset.Id.IsNil() {
		dstAsset.Id = dao.GenUUID()
	}

	dstAsset.Name = file.Filename
	dstAsset.FileSize = int(file.Size)
	dstAsset.ContentType = file.Header.Get("Content-Type")

	if err := s.storage.SaveReader(
		assetSrc,
		file.Size,
		dstAsset.Id,
		dstAsset.ContentType,
		&metadata,
	); err != nil {
		return err
	}

	return tx.Create(&dstAsset).Error
}

func (s *Services) uploadAvatarForm(tx *gorm.DB, file *multipart.FileHeader, dstAsset *dao.FileAsset) error {
	assetSrc, err := file.Open()
	if err != nil {
		return err
	}
	defer assetSrc.Close()

	if dstAsset.Id.IsNil() {
		dstAsset.Id = dao.GenUUID()
	}

	dataType := file.Header.Get("Content-Type")

	dstAsset.Name = file.Filename
	dstAsset.FileSize = int(file.Size)
	dstAsset.ContentType = dataType

	dataSize := 0
	var data io.Reader

	switch dataType {
	case "image/gif", "image/jpeg", "image/png":
		data, dataSize, dataType, err = imageThumbnail(assetSrc, dataType)
		if err != nil {
			return err
		}
	default:
		return apierrors.ErrUnsupportedAvatarType
	}

	if err := s.storage.SaveReader(
		data,
		int64(dataSize),
		dstAsset.Id,
		dataType,
		&filestorage.Metadata{},
	); err != nil {
		return err
	}

	return tx.Create(&dstAsset).Error
}

func hasRecentFieldUpdate(tx *gorm.DB, userId uuid.UUID, fields ...string) bool {
	err := tx.Model(&dao.ActivityEvent{}).
		Where("actor_id = ?", userId).
		Where("created_at > NOW() - INTERVAL '2 seconds'").
		Where("field IN (?)", fields).
		Take(&dao.ActivityEvent{}).Error

	return err == nil
}
