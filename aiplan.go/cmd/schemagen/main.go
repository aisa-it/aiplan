// Получает имена моделей DAO, предназначенных для миграций, из указанного файла Go и вставляет их в файл main.go.
package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// GetDAOModelsForMigration извлекает имена моделей DAO, предназначенных для миграций, из указанного файла Go и возвращает их в виде строки для вставки в main.go.
//
// Параметры:
//   - filePath: путь к файлу Go, содержащему определения моделей DAO.
//
// Возвращает:
//   - models: слайс строк, содержащий строки с типами моделей DAO в формате `&dao.ModelName`.
//   - error: ошибка, если произошла ошибка при парсинге или обработке файла.
func GetDAOModelsForMigration(filePath string) (models []string, err error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	for _, pkg := range pkgs {
		d := doc.New(pkg, filePath, doc.AllDecls)

		for _, t := range d.Types {
			if _, ok := t.Decl.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType); !ok {
				continue
			}
			if strings.Contains(t.Doc, "-migration") {
				slog.Warn("Skip struct migration", "name", t.Name)
				continue
			}

			models = append(models, fmt.Sprintf("&dao.%s{}", t.Name))
		}
	}
	return
}

// main - главная функция, которая извлекает имена моделей DAO для миграций из указанного файла Go и вставляет их в файл main.go.  Функция читает main.go, ищет строку, содержащую объявление переменной `models`, и заменяет её на строку, содержащую список моделей DAO в формате `&dao.ModelName`. Затем обновленный файл main.go записывается обратно на диск.  Обрабатывает ошибки при чтении и записи файлов.  Необходимо, чтобы в файле `main.go` уже было объявление переменной `var models = []any{}`.  Также, функция предполагает, что модели DAO определены в каталоге `internal/aiplan/dao/`.
func main() {
	mainFilePath := "../../cmd/aiplan/main.go"

	models, err := GetDAOModelsForMigration("../../internal/aiplan/dao/")
	if err != nil {
		fmt.Println(err)
	}

	cmd := fmt.Sprintf("var models = []any{%s}", strings.Join(models, ", "))
	fmt.Println(cmd)

	mainFile, err := os.ReadFile(mainFilePath)
	if err != nil {
		fmt.Println(err)
		return
	}

	reg := regexp.MustCompile(`var\s*models\s*=\s*\[\]any{.*}`)
	if err := os.WriteFile(mainFilePath, reg.ReplaceAll(mainFile, []byte(cmd)), 0644); err != nil {
		fmt.Println(err)
	}
}
