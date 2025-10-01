// Пакет для очистки нежелательных активов (файлов) в хранилище. Обнаруживает файлы с именем, содержащим "unknown/", или имена, не являющиеся UUID, и перемещает их в директорию "unknown/". Также удаляет дубликаты файлов, зарегистрированные в базе данных, перемещая их в "unknown/".
//
// Основные возможности:
//   - Обнаружение и перемещение файлов с нежелательными именами.
//   - Удаление дубликатов файлов, хранящихся в базе данных.
package maintenance

import (
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/internal/aiplan/file-storage"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type AssetsCleaner struct {
	db *gorm.DB
	si filestorage.FileStorage
}

func NewAssetCleaner(db *gorm.DB, si filestorage.FileStorage) *AssetsCleaner {
	return &AssetsCleaner{db, si}
}

func (ac *AssetsCleaner) CleanAssets() {
	slog.Info("Start assets cleaning")
	var moved int
	if err := ac.si.ListRoot(func(fi filestorage.FileInfo) error {
		if strings.Contains(fi.Name, "unknown/") {
			return nil
		}

		if _, err := uuid.FromString(fi.Name); err != nil {
			if err := ac.si.Move(fi.Name, "unknown/"+fi.Name); err != nil {
				return err
			}
			moved++
			return nil
		}

		var exist bool
		if err := ac.db.
			Where("id = ?", fi.Name).
			Select("count(*) > 0").
			Model(&dao.FileAsset{}).
			Find(&exist).Error; err != nil && !strings.Contains(err.Error(), "invalid input syntax") {
			return err
		}
		if exist {
			return nil
		}
		if err := ac.si.Move(fi.Name, "unknown/"+fi.Name); err != nil {
			return err
		}
		moved++
		return nil
	}); err != nil {
		slog.Error("Clean assets fail", "err", err)
	}
	slog.Info("Finish assets cleaning", "moved", moved)
}
