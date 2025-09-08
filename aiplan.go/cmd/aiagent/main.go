// This package provides tools for generating Go documentation comments using a large language model.
//
// It analyzes Go source files, extracts information about functions, types, and packages,
// and then uses a language model to generate GoDoc-formatted comments.
//
// The generated comments are added to the source files, improving code readability and maintainability.
//
// Supported features:
//   - Generates GoDoc comments for functions, types (structs and interfaces), and packages.
//   - Uses a language model to generate informative and concise comments.
//   - Adds comments to the original source files.
//   - Handles package comments and function/type comments separately.
//   - Includes error handling and logging for robustness.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
)

type FunctionInfo struct {
	Name    string
	Params  string
	Returns string
	File    string
	Line    int
}

type TypeInfo struct {
	Name    string
	Type    string // "struct" or "interface"
	Fields  string // For structs: field names and types
	Methods string // For interfaces: method signatures
	File    string
	Line    int
}

type FileInfo struct {
	Path    string
	Package string
	Line    int
}

// Это главная функция программы. Она служит точкой входа в приложение и выполняет инициализацию.
//
// Она принимает аргументы командной строки и обрабатывает их.
// Также она инициирует работу других компонентов программы.
//
// Возвращает:
//   - int: код возврата программы. 0 - успешное выполнение, ненулевое значение - ошибка.
func main() {
	if len(os.Args) < 2 {
		log.Fatal("Укажите путь к директории с Go-файлами")
	}

	rootDir := os.Args[1]
	var functions []FunctionInfo
	var types []TypeInfo
	var files []FileInfo

	// Create Ollama client
	client, err := api.ClientFromEnvironment()
	if err != nil {
		log.Fatalf("Failed to create Ollama client: %v", err)
	}

	// Рекурсивно обходим директорию и находим все Go-файлы
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}

			// Save package info
			pos := fset.Position(node.Pos())
			files = append(files, FileInfo{
				Path:    path,
				Package: node.Name.Name,
				Line:    pos.Line,
			})

			// Анализируем AST файла
			ast.Inspect(node, func(n ast.Node) bool {
				switch fn := n.(type) {
				case *ast.FuncDecl:
					log.Printf("func %v\n", fn.Name.Name)

					// Получаем информацию о параметрах и возвращаемых значениях
					params := getParams(fn.Type.Params)
					returns := getReturns(fn.Type.Results)

					// Сохраняем информацию о функции
					pos := fset.Position(fn.Pos())
					functions = append(functions, FunctionInfo{
						Name:    fn.Name.Name,
						Params:  params,
						Returns: returns,
						File:    path,
						Line:    pos.Line,
					})

				case *ast.TypeSpec:
					switch t := fn.Type.(type) {
					case *ast.StructType:
						fields := getFields(t.Fields)
						pos := fset.Position(fn.Pos())
						types = append(types, TypeInfo{
							Name:   fn.Name.Name,
							Type:   "struct",
							Fields: fields,
							File:   path,
							Line:   pos.Line,
						})
					case *ast.InterfaceType:
						methods := getMethods(t.Methods)
						pos := fset.Position(fn.Pos())
						types = append(types, TypeInfo{
							Name:    fn.Name.Name,
							Type:    "interface",
							Methods: methods,
							File:    path,
							Line:    pos.Line,
						})
					}
				}

				return true
			})
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d functions\n", len(functions))
	log.Printf("found %d types\n", len(types))
	log.Printf("found %d files\n", len(files))
	var cnts = make(map[string]int)
	// Process package comments first
	for _, file := range files {

		var cnt = cnts[file.Path]
		if hasGodocComment(file.Path, file.Line) {
			log.Printf("Skipping package %s - already has a comment", file.Package)
			continue
		}

		comment, err := generatePackageComment(client, file)
		if err != nil {
			log.Printf("Error generating package comment for %s: %v", file.Path, err)
			continue
		}

		if err := addCommentToFile(file.Path, file.Line, comment); err != nil {
			log.Printf("Error adding package comment to file %s: %v", file.Path, err)
		}
		cnt += strings.Count(comment, "\n")
		cnts[file.Path] = cnt
	}
	for _, fn := range functions {

		var cnt = cnts[fn.File]
		// Check if function already has a comment
		if hasGodocComment(fn.File, fn.Line+cnt) {
			log.Printf("Skipping %s - already has a comment", fn.Name)
			continue
		}

		comment, err := generateGodocCommentOld(client, fn)
		if err != nil {
			log.Printf("Ошибка генерации комментария для %s: %v", fn.Name, err)
			continue
		}

		if err := addCommentToFile(fn.File, fn.Line+cnt, comment); err != nil {
			log.Printf("Ошибка добавления комментария в файл %s: %v", fn.File, err)
		}
		cnt += strings.Count(comment, "\n")
		cnts[fn.File] = cnt
	}
	// for _, typ := range types {
	// 	var cnt = cnts[typ.File]
	// 	if hasGodocComment(typ.File, typ.Line+cnt) {
	// 		log.Printf("Skipping %s %s - already has a comment", typ.Type, typ.Name)
	// 		continue
	// 	}

	// 	comment, err := generateTypeComment(client, typ)
	// 	if err != nil {
	// 		log.Printf("Error generating comment for %s %s: %v", typ.Type, typ.Name, err)
	// 		continue
	// 	}

	// 	if err := addCommentToFile(typ.File, typ.Line+cnt, comment); err != nil {
	// 		log.Printf("Error adding comment to file %s: %v", typ.File, err)
	// 	}
	// 	cnt += strings.Count(comment, "\n")
	// 	cnts[typ.File] = cnt
	// }

}

// Функция для получения параметров функции.
//
// Параметры:
//   - fl *ast.FieldList: Список полей структуры, содержащей параметры функции.
//
// Возвращает:
//   - string: Строка, содержащая список параметров функции, разделенных запятыми.
func getParams(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}

	var params []string
	for _, field := range fl.List {
		paramType := exprToString(field.Type)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				params = append(params, fmt.Sprintf("%s %s", name.Name, paramType))
			}
		} else {
			params = append(params, paramType)
		}
	}
	return strings.Join(params, ", ")
}

// Функция для получения списка возвращаемых значений из списка полей структуры.
//
// Параметры:
//   - fl *ast.FieldList: Список полей структуры, содержащей параметры функции.
//
// Возвращаемые значения:
//   - string: Строка, содержащая список параметров функции, разделенных запятыми.
func getReturns(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}

	var returns []string
	for _, field := range fl.List {
		returns = append(returns, exprToString(field.Type))
	}
	return strings.Join(returns, ", ")
}

// Функция преобразует выражение Go в строку.
//
// Параметры:
//   - expr: Выражение Go, которое нужно преобразовать в строку.
//
// Возвращает:
//   - Строка, представляющая собой строковое представление выражения Go.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.FuncType:
		return "func(" + getParams(t.Params) + ")" + getReturns(t.Results)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// Функция для получения списка полей структуры.
//
// Параметры:
//   - fl *ast.FieldList: Список полей структуры, содержащей параметры функции.
//
// Возвращаемые значения:
//   - string: Строка, содержащая список параметров функции, разделенных запятыми.
func getFields(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}

	var fields []string
	for _, field := range fl.List {
		fieldType := exprToString(field.Type)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				fields = append(fields, fmt.Sprintf("%s %s", name.Name, fieldType))
			}
		} else {
			fields = append(fields, fieldType)
		}
	}
	return strings.Join(fields, "\n")
}

// Функция для получения списка методов структуры.
//
// Параметры:
//   - fl *ast.FieldList: Список полей структуры, содержащей методы.
//
// Возвращаемые значения:
//   - string: Строка, содержащая список методов структуры, разделенных новой строкой.
func getMethods(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}

	var methods []string
	for _, field := range fl.List {
		if fn, ok := field.Type.(*ast.FuncType); ok {
			methods = append(methods, fmt.Sprintf("%s(%s) %s",
				field.Names[0].Name,
				getParams(fn.Params),
				getReturns(fn.Results)))
		}
	}
	return strings.Join(methods, "\n")
}

// Эта функция генерирует коментарий в формате GoDoc для предоставленной функции, типа или пакета.
//
// Функция принимает на вход путь к файлу, анализирует его содержимое, используя парсер Go, и затем использует языковую модель для генерации соответствующего коментария в формате GoDoc.
//
// Параметры:
//   - client: Клиент для взаимодействия с языковой моделью.
//   - content: Содержимое файла.
//   - prompt: Подсказка для языковой модели, описывающая задачу.
//
// Возвращает:
//   - Строку, содержащую сгенерированный коментарий в формате GoDoc.
func generateComment(client *api.Client, content string, prompt string) (string, error) {
	format := json.RawMessage(`{
        "type": "object",
        "properties": {
            "comment": {
                "type": "string"
            }
        },
        "required": [
            "comment"
        ]
    }`)

	log.Printf("Content length: %d", len(content))
	var fullResponse strings.Builder
	var stream = false
	var req = api.ChatRequest{
		Model: "gemma3n:latest",
		Messages: []api.Message{
			{
				Role:    "user",
				Content: content,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Format: format,
		// Options: map[string]interface{}{
		// 	"temperature": 0.2,
		// },
		Stream: &stream,
	}

	// bts, err := json.Marshal(req)
	// if err != nil {
	// 	return "", err
	// }

	// log.Printf("\n%s\n", string(bts))

	err := client.Chat(context.Background(), &req, func(resp api.ChatResponse) error {
		log.Printf(".")
		if len(resp.Message.Content) > 0 {
			fullResponse.WriteString(resp.Message.Content)
		}
		return nil
	})
	if err != nil {
		log.Printf("!\n")
		return "", err
	}

	var result struct {
		Comment string `json:"comment"`
	}

	if err := json.Unmarshal([]byte(fullResponse.String()), &result); err != nil {
		log.Printf("!\n")
		return "", err
	}
	log.Printf(".\n")
	return result.Comment + "\n", nil

}

// Эта функция генерирует комментарии GoDoc для пакета, анализируя исходный код и используя языковую модель для создания информативных и кратких комментариев.
//
// Она принимает путь к директории с Go-файлами, рекурсивно обходит их, извлекает информацию о функциях, типах (структурах и интерфейсах) и пакетах.
// Затем использует языковую модель для генерации комментариев в формате GoDoc и добавляет их в исходные файлы.
//
// Поддерживаемые функции:
//   - Генерация комментариев для функций, типов (структур и интерфейсов) и пакетов.
//   - Использование языковой модели для создания информативных и кратких комментариев.
//   - Добавление комментариев в исходные файлы.
//   - Обработка комментариев пакетов и функций/типов отдельно.
//   - Обработка ошибок и логирование для надежности.
func generatePackageComment(client *api.Client, file FileInfo) (string, error) {
	log.Printf("generating package comment for %s\n", file.Path)
	prompt := fmt.Sprintf(`Напиши комментарий на русском языке в формате godoc для Go-пакета %s.
Комментарий должен быть кратким, но информативным, описывать назначение пакета.
Формат:
// Краткое описание.
//
// Основные возможности:
//   - Возможность 1
//   - Возможность 2
// `, file.Package)

	content, err := os.ReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	return generateComment(client, string(content), prompt)
}

// Эта функция генерирует комментарий GoDoc для типа (структуры или интерфейса), анализируя исходный код и используя языковую модель для создания информативных и кратких комментариев.
//
// Функция принимает путь к файлу, рекурсивно обходит его, извлекает информацию о функциях, типах (структурах и интерфейсах) и пакетах. Затем использует языковую модель для генерации комментариев GoDoc и добавляет их в исходные файлы.
//
// Поддерживаемые функции:
//   - Генерация комментариев для функций, типов (структур и интерфейсов) и пакетов.
//   - Использование языковой модели для создания информативных и кратких комментариев.
//   - Добавление комментариев в исходные файлы.
//   - Обработка комментариев пакетов и функций/типов отдельно.
//   - Обработка ошибок и логирование для надежности.
func generateTypeComment(client *api.Client, typ TypeInfo) (string, error) {
	log.Printf("generating godoc comment for %s:%s\n", typ.File, typ.Name)
	var t = "Methods"
	var tt = typ.Methods
	if typ.Type == "struct" {
		t = "Fields"
		tt = typ.Fields
	}
	prompt := fmt.Sprintf(`Напиши комментарий на русском языке в формате godoc для Go-%s %s:
    
%s: %s

%s:
%s

Комментарий должен быть кратким, но информативным, описывать назначение типа.
Формат для структур:
// TypeName краткое описание.
//
// Поля:
//   - Field1: описание
//   - Field2: описание
//

Формат для интерфейсов:
// TypeName краткое описание.
//
// Методы:
//   - Method1: описание
//   - Method2: описание
//

`, typ.Type, typ.Name,
		strings.Title(typ.Type), typ.Name,
		strings.Title(t),
		tt)

	content, err := os.ReadFile(typ.File)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	return generateComment(client, string(content), prompt)
}

// Эта функция генерирует GoDoc-комментарии для функций, типов (структур и интерфейсов) и пакетов. Она анализирует исходный код Go-файлов, используя парсер Go, и затем использует языковую модель для генерации соответствующих комментариев в формате GoDoc.  Эти комментарии добавляются обратно в исходные файлы, улучшая читаемость и поддерживаемость кода.  Функция обрабатывает пакетные комментарии и комментарии функций/типов отдельно, обеспечивая надежность и обработку ошибок.
func generateGodocCommentOld(client *api.Client, fn FunctionInfo) (string, error) {
	log.Printf("generating godoc comment for %s:%s\n", fn.File, fn.Name)
	prompt := fmt.Sprintf(`Напиши комментарий на русском языке в формате godoc для Go-функции:
    
Функция: %s
Параметры: %s
Возвращаемые значения: %s

Комментарий должен быть кратким, но информативным, описывать назначение функции,
её параметры и возвращаемые значения. Формат:
// FunctionName краткое описание.
//
// Параметры:
//   - param1: описание
//   - param2: описание
//
// Возвращает:
//   - тип1: описание
// `, fn.Name, fn.Params, fn.Returns)

	content, err := os.ReadFile(fn.File)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	return generateComment(client, string(content), prompt)
}

// Этот пакет предоставляет инструменты для генерации комментариев GoDoc с использованием большой языковой модели.
//
// Он анализирует исходные файлы Go, извлекает информацию о функциях, типах (структурах и интерфейсах) и пакетах,
// а затем использует языковую модель для генерации комментариев GoDoc.
//
// Поддерживаемые функции:
//   - Генерирует комментарии GoDoc для функций, типов (структур и интерфейсов) и пакетов.
//   - Использует языковую модель для генерации информативных и кратких комментариев.
//   - Добавляет комментарии в исходные файлы.
//   - Обрабатывает комментарии пакетов и функций/типов отдельно.
//   - Включает обработку ошибок и логирование для надежности.
func addCommentToFile(filename string, originalLine int, comment string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	// Ensure each line in comment starts with //
	var formattedComment strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(comment))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			line = "// " + line
		}
		formattedComment.WriteString(line + "\n")
	}

	lines := strings.Split(string(content), "\n")

	// Вставляем комментарий
	newLines := make([]string, 0, len(lines)+strings.Count(comment, "\n")+1)
	newLines = append(newLines, lines[:originalLine-1]...)
	newLines = append(newLines, strings.TrimRight(formattedComment.String(), "\n"))
	newLines = append(newLines, lines[originalLine-1:]...)

	return os.WriteFile(filename, []byte(strings.Join(newLines, "\n")), 0644)
}

// Эта функция проверяет, есть ли в файле Go комментарий GoDoc на указанной строке.
//
// Параметры:
//   - filename: Путь к файлу Go.
//   - line: Номер строки, на которой нужно проверить наличие комментария.
//
// Возвращает:
//   - true, если в файле есть комментарий GoDoc на указанной строке, иначе false.
func hasGodocComment(filename string, line int) bool {
	content, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("Error reading file %s: %v", filename, err)
		return false
	}

	lines := strings.Split(string(content), "\n")
	if line <= 1 {
		return false
	}

	prevLine := strings.TrimSpace(lines[line-2])
	return strings.HasPrefix(prevLine, "//")
}
