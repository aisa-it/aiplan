package email

import (
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

//go:embed templates/*
var defaultTemplates embed.FS

const (
	templateCollectAll      = "v2_collect_all"
	templateCollectOne      = "v2_collect_one"
	templateCollectComplex  = "v2_collect_complex"
	templateCompositeFields = "v2_composite_fields"
	templateBody            = "v2_body"
	templateActivity        = "v2_activity"
	templateAuthorActivity  = "v2_author_activity"
	templateChangeCounter   = "v2_change_counter"
	templateHeadEntity      = "v2_head_entity"
	templateCollectValues   = "v2_collect_values"
)

var (
	templateNames = []string{
		templateCollectAll,
		templateCollectOne,
		templateCollectComplex,
		templateCollectValues,
		templateCompositeFields,
		templateBody,
		templateActivity,
		templateAuthorActivity,
		templateChangeCounter,
		templateHeadEntity,
	}
)

type EmailTemplates struct {
	ReplaceTxtToSvg        func(string) string
	CollectAll             *template.Template
	CollectOne             *template.Template
	CollectComplex         *template.Template
	CollectValues          *template.Template
	CollectCompositeFields *template.Template
	Body                   *template.Template
	Activity               *template.Template
	AuthorActivity         *template.Template
	ChangeCounter          *template.Template
	HeadEntity             *template.Template
}

// CreateNewTemplates загружает встроенные шаблоны в БД при запуске
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

			data, err = getEmailMinifier().Bytes("text/html", data)
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

type cacheEntry struct {
	daoTemplate dao.Template
	expiry      time.Time
}

type TemplateService struct {
	db    *gorm.DB
	cache map[string]*cacheEntry
	mutex sync.RWMutex
	ttl   time.Duration
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{
		db:    db,
		cache: make(map[string]*cacheEntry),
		ttl:   5 * time.Minute,
	}
}

func (ts *TemplateService) Get(name string) (*dao.Template, error) {
	ts.mutex.RLock()
	if entry, ok := ts.cache[name]; ok && time.Now().Before(entry.expiry) {
		ts.mutex.RUnlock()
		return &entry.daoTemplate, nil
	}
	ts.mutex.RUnlock()

	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	if entry, ok := ts.cache[name]; ok && time.Now().Before(entry.expiry) {
		return &entry.daoTemplate, nil
	}

	var daoTmpl dao.Template
	if err := ts.db.Where("name = ?", name).First(&daoTmpl).Error; err != nil {
		return nil, err
	}

	if daoTmpl.ParsedTemplate == nil {
		tmpl, err := template.New(name).Parse(daoTmpl.Template)
		if err != nil {
			return nil, err
		}
		daoTmpl.ParsedTemplate = tmpl
	}

	ts.cache[name] = &cacheEntry{
		daoTemplate: daoTmpl,
		expiry:      time.Now().Add(ts.ttl),
	}
	return &daoTmpl, nil
}

func (ts *TemplateService) LoadTemplates() EmailTemplates {
	var res EmailTemplates
	var firstTemplate *dao.Template
	for _, name := range templateNames {
		tmpl, err := ts.Get(name)
		if err != nil {
			slog.Error("Failed to load template", "name", name, "error", err)
			continue
		}
		if firstTemplate == nil && res.ReplaceTxtToSvg == nil {
			firstTemplate = tmpl
		}
		switch name {
		case templateCollectAll:
			res.CollectAll = tmpl.ParsedTemplate
		case templateCollectOne:
			res.CollectOne = tmpl.ParsedTemplate
		case templateCollectComplex:
			res.CollectComplex = tmpl.ParsedTemplate
		case templateCollectValues:
			res.CollectValues = tmpl.ParsedTemplate
		case templateCompositeFields:
			res.CollectCompositeFields = tmpl.ParsedTemplate
		case templateBody:
			res.Body = tmpl.ParsedTemplate
		case templateActivity:
			res.Activity = tmpl.ParsedTemplate
		case templateAuthorActivity:
			res.AuthorActivity = tmpl.ParsedTemplate
		case templateChangeCounter:
			res.ChangeCounter = tmpl.ParsedTemplate
		case templateHeadEntity:
			res.HeadEntity = tmpl.ParsedTemplate
		}
	}
	if firstTemplate != nil {
		res.ReplaceTxtToSvg = createReplaceFunc(*firstTemplate)
	}
	return res
}

func createReplaceFunc(t dao.Template) func(string) string {
	imgRegex := regexp.MustCompile(`image:\s+\(alt:\s*([^)]*)\)`)
	tableRegex := regexp.MustCompile(`table\s*\(size:\s*(\d+)x(\d+)\)`)
	imgSvg := t.Icons["icon_pictures"]
	tableSvg := t.Icons["icon_table"]

	return func(body string) string {
		if imgSvg != "" {
			body = imgRegex.ReplaceAllStringFunc(body, func(imgTag string) string {
				matches := imgRegex.FindStringSubmatch(imgTag)
				altText := "image"
				if len(matches) > 1 {
					altText = matches[1]
				}
				return fmt.Sprintf(`<span alt="%s">%s (%s)</span>`, altText, imgSvg, altText)
			})
		}

		if tableSvg != "" {
			body = tableRegex.ReplaceAllStringFunc(body, func(tableTag string) string {
				matches := tableRegex.FindStringSubmatch(tableTag)
				if len(matches) == 3 {
					rows := matches[1]
					cols := matches[2]
					return fmt.Sprintf(`<span alt="table:(%sx%s)">%s (%sx%s)</span>`, rows, cols, tableSvg, rows, cols)
				}
				return tableTag
			})
		}

		return body
	}
}

func toValueCtx(value, body *string) *actValueCtx {
	hasValidValue := value != nil && *value != ""
	hasValidBody := body != nil && *body != ""

	if !hasValidValue && !hasValidBody {
		return nil
	}

	return &actValueCtx{
		Value: value,
		Body:  body,
	}
}
