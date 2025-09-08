// Управление данными о шаблонах, включая загрузку, обработку и замену текстовых ссылок на SVG-изображения и таблицы.
//
// Основные возможности:
//   - Загрузка шаблонов из базы данных и парсинг с использованием template.
//   - Извлечение ссылок на SVG-изображения и таблицы из шаблонов.
//   - Замена текстовых ссылок на SVG-изображения и таблицы на соответствующие SVG-элементы в HTML.
package dao

import (
	"fmt"
	"regexp"
	"text/template"

	"gorm.io/gorm"
)

type Template struct {
	// id uuid IS_NULL:NO
	Id string `json:"id"`
	// name text IS_NULL:NO
	Name string `json:"name" gorm:"index"`
	// template text IS_NULL:NO
	Template       string             `json:"template"`
	Func           template.FuncMap   `gorm:"-"`
	ParsedTemplate *template.Template `gorm:"-" extensions:"x-nullable"`
	Icons          map[string]string  `json:"-" gorm:"-"`
}

func (Template) TableName() string { return "templates" }

func (temp *Template) AfterFind(tx *gorm.DB) error {
	t, err := template.New(temp.Name).Funcs(temp.Func).Parse(temp.Template)
	if err != nil {
		return err
	}
	temp.ParsedTemplate = t
	temp.Icons = make(map[string]string)
	var res []struct {
		Name     string
		Template string
	}
	if err := tx.Table("templates").Where("name IN ?", []string{"icon_pictures", "icon_table"}).Select("name", "template").Scan(&res).Error; err != nil {
		return err
	}
	for _, re := range res {
		temp.Icons[re.Name] = re.Template
	}
	return nil
}

func (temp *Template) ReplaceTxtToSvg(body string) string {
	imgRegex := regexp.MustCompile(`image:\s+\(alt:\s*([^)]*)\)`)
	tableRegex := regexp.MustCompile(`table\s*\(size:\s*(\d+)x(\d+)\)`)
	imgSvg := temp.Icons["icon_pictures"]
	tableSvg := temp.Icons["icon_table"]

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
