// Инициализация файлового хранилища (S3/minio или локальная директория).
package server

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
)

// initStorage создаёт файловое хранилище: minio, если задан AWS_ENDPOINT,
// иначе локальная директория. При недоступном minio — фолбэк на локальное.
func initStorage(c *config.Config) (filestorage.FileStorage, error) {
	if c.AssetsPath == "" {
		slog.Warn("ASSETS_PATH param empty, fallback to ./assets dir")
		c.AssetsPath = "assets"
	}

	if c.AWSEndpoint != "" {
		storage, err := filestorage.NewMinioStorage(c.AWSEndpoint, c.AWSAccessKey, c.AWSSecretKey, false, c.AWSBucketName)
		if err == nil {
			return storage, nil
		}
		slog.Warn("Fail init Minio connection, fallback to local file storage", "err", err)
	}

	storage, err := filestorage.NewLocalStorage(c.AssetsPath)
	if err != nil {
		return nil, fmt.Errorf("init local file storage (%s): %w", c.AssetsPath, err)
	}
	return storage, nil
}
