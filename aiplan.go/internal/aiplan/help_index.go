// Пакет для генерации и отображения иерархического списка страниц справки (help) из директории с Markdown файлами.  Преобразует структуру директории в JSON для использования в веб-интерфейсе.
//
// Основные возможности:
//   - Автоматическое обнаружение иерархии страниц справки на основе структуры директории.
//   - Генерация URL для каждой страницы справки.
//   - Извлечение заголовка страницы из Markdown файла.
//   - Предоставление JSON представления иерархии для отображения в веб-браузере.
package aiplan

import (
	"bufio"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

type HelpPage struct {
	Title    string      `json:"title"`
	FullPath string      `json:"full_path"`
	SubPages []*HelpPage `json:"sub_pages"`
}

func NewHelpIndex(root string) func(echo.Context) error {
	index := readDir(root, 0)
	resolveHelpURL(root, index)

	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, index)
	}
}

func resolveHelpURL(root string, pages []*HelpPage) {
	for i, sub := range pages {
		ref, _ := url.Parse(filepath.Join("/api/docs/", strings.TrimPrefix(sub.FullPath, root)))
		pages[i].FullPath = cfg.WebURL.ResolveReference(ref).String()

		if len(sub.SubPages) > 0 {
			resolveHelpURL(root, sub.SubPages)
		}
	}
}

func readDir(root string, level int) []*HelpPage {
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var res []*HelpPage
	for _, item := range items {
		if strings.HasPrefix(item.Name(), "test") {
			continue
		}

		if item.Name() == "README.md" || item.Name() == "node_modules" {
			continue
		}

		p := getHelpPageNum(item.Name())
		itemPath := filepath.Join(root, item.Name())
		page := &HelpPage{
			FullPath: itemPath,
		}
		if item.IsDir() {
			page.FullPath = filepath.Join(itemPath, item.Name()+".md")
			if _, err := os.Stat(page.FullPath); err != nil {
				page.FullPath = ""
			}
			page.SubPages = readDir(itemPath, level+1)
			if page.FullPath == "" && len(page.FullPath) == 0 {
				continue
			}
		} else if filepath.Ext(item.Name()) != ".md" {
			continue
		}

		page.Title = getHelpHeader(page.FullPath)

		pathArr := strings.Split(root, "/")
		if pathArr[len(pathArr)-1]+".md" == filepath.Base(item.Name()) {
			continue
		}

		if level < len(p) && p[level] != -1 {
			res = insertInIndexSlice(res, p[level], page)
		} else {
			res = append(res, page)
		}

		res = slices.DeleteFunc(res, func(hp *HelpPage) bool {
			return hp == nil
		})
	}
	return res
}

func getHelpPageNum(raw string) []int {
	arr := strings.Split(raw, "_")
	if len(arr) < 2 {
		return []int{-1}
	}

	indexes := strings.Split(arr[0], ".")
	resIndexes := make([]int, len(indexes))
	for i, rawI := range indexes {
		ii, err := strconv.Atoi(rawI)
		if err != nil {
			return []int{-1}
		}
		resIndexes[i] = ii
	}

	return resIndexes
}

func insertInIndexSlice[T any](arr []T, i int, v T) []T {
	if i < len(arr) {
		return slices.Insert(arr, i, v)
	}
	arr = append(arr, make([]T, i-len(arr)+1)...)
	return slices.Insert(arr, i, v)
}

func getHelpHeader(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	r := bufio.NewReader(f)
	header, _ := r.ReadString('\n')
	return strings.TrimSpace(strings.ReplaceAll(header, "#", ""))
}
