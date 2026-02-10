package email

import (
	"embed"
	"log/slog"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

//go:embed templates/*
var defaultTemplates embed.FS

const (
	templateCollectAll     = "v2_collect_all"
	templateCollectOne     = "v2_collect_one"
	templateBody           = "v2_body"
	templateActivity       = "v2_activity"
	templateAuthorActivity = "v2_author_activity"
)

type EmailTemplates struct {
	ReplaceTxtToSvg func(string) string
	CollectAll      *template.Template
	CollectOne      *template.Template
	Body            *template.Template
	Activity        *template.Template
	AuthorActivity  *template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		templateCollectAll,
		templateCollectOne,
		templateBody,
		templateActivity,
		templateAuthorActivity,
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	res.ReplaceTxtToSvg = templates[0].ReplaceTxtToSvg
	for _, t := range templates {
		switch t.Name {
		case templateCollectAll:
			res.CollectAll = t.ParsedTemplate
		case templateCollectOne:
			res.CollectOne = t.ParsedTemplate
		case templateBody:
			res.Body = t.ParsedTemplate
		case templateActivity:
			res.Activity = t.ParsedTemplate
		case templateAuthorActivity:
			res.AuthorActivity = t.ParsedTemplate
		}
	}
	return res
}

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
