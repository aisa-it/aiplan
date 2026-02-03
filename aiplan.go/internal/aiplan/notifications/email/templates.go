package email

import (
	"embed"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

//go:embed templates/*
var defaultTemplates embed.FS

func (*EmailService) CreateNewTemplates(tx *gorm.DB) {
	dir, err := defaultTemplates.ReadDir("templates")
	if err == nil {
		for _, file := range dir {
			var exist bool
			name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
			if err := tx.Model(&dao.Template{}).
				Select("EXISTS(?)",
					tx.Model(&dao.Template{}).
						Select("1").
						Where("name = ?", name),
				).
				Find(&exist).Error; err != nil {
				slog.Warn("Error check template in db", slog.String("name", name), "err", err)
				continue
			}
			if exist {
				continue
			}

			data, err := defaultTemplates.ReadFile(filepath.Join("templates", file.Name()))
			if err != nil {
				slog.Warn("Read embed template", slog.String("name", filepath.Join("templates", file.Name())), "err", err)
				continue
			}

			data, err = minifier.Bytes("text/html", data)
			if err != nil {
				slog.Warn("Error minify embed template", slog.String("name", filepath.Join("templates", file.Name())), "err", err)
			}

			if err := tx.Create(&dao.Template{
				Id:       dao.GenUUID(),
				Name:     name,
				Template: string(data),
			}).Error; err != nil {
				slog.Warn("Error insert default template", slog.String("name", name), "err", err)
			}
		}
	}
}
