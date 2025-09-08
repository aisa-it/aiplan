// Генерация документации об ошибках API в формате Markdown.
// Анализирует файлы с определениями ошибок и создает Markdown-документ с таблицей, содержащей коды ошибок, HTTP-коды, сообщения и переводы на русский язык.
//
// Основные возможности:
//   - Чтение файлов Go с определениями ошибок.
//   - Извлечение информации об ошибках из определения.
//   - Генерация Markdown-таблицы с информацией об ошибках.
//   - Создание Markdown-документа с таблицей ошибок.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"strings"

	md "github.com/nao1215/markdown"
)

// main - главная функция программы.  Считывает определения ошибок из указанного файла, генерирует Markdown-таблицу с информацией об ошибках и сохраняет её в указанный файл.
//
// Параметры:
//   - src: путь к файлу с определениями ошибок (например, internal/aiplan/errors.go).
//   - out: путь к файлу, куда будет сохранена Markdown-таблица с ошибками.
//
// Возвращает:
//   - void: функция ничего не возвращает.
func main() {
	errorsFile := flag.String("src", "internal/aiplan/apierrors/errors.go", "Path of errors.go")
	outputMd := flag.String("out", "api_error.md", "Path to output md")
	flag.Parse()

	slog.Info("Generate api errors docs", "src", *errorsFile, "out", *outputMd)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, *errorsFile, nil, 0)
	if err != nil {
		panic(err)
	}

	rows := getRows(f)

	ff, _ := os.Create(*outputMd)
	if err := md.NewMarkdown(ff).
		H1("Перечень кодов ошибок").
		PlainText("Данный раздел посвящен описанию возможных ошибок от сервера.").
		CustomTable(md.TableSet{
			Header: []string{"Код", "HTTP код", "Сообщение", "Сообщение на русском"},
			Rows:   rows,
		}, md.TableOptions{
			AutoWrapText: false,
		}).Build(); err != nil {
		slog.Error("Generate docs fail", "err", err)
	} else {
		slog.Info("Docs generated")
	}
}

// Функция парсит определения ошибок из файла AST и возвращает таблицу строк, представляющую собой строки для таблицы Markdown.
func getRows(f *ast.File) [][]string {
	var rows [][]string
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, id := range spec.Names {
						row := make([]string, 4)
						definedError, ok := id.Obj.Decl.(*ast.ValueSpec).Values[0].(*ast.CompositeLit)
						if !ok {
							continue
						}
						for _, v := range definedError.Elts {
							if param, ok := v.(*ast.KeyValueExpr); ok {
								switch fmt.Sprint(param.Key) {
								case "Code":
									row[0] = md.Bold(param.Value.(*ast.BasicLit).Value)
								case "StatusCode":
									statusName := param.Value.(*ast.SelectorExpr).Sel.Name
									row[1] = fmt.Sprintf("%s %s", getStatusCode(statusName), md.Italic(statusName))
								case "Err":
									if binaryExpr, ok := param.Value.(*ast.BinaryExpr); ok {
										row[2] = md.Code(getBinaryExprString(binaryExpr))
									} else {
										row[2] = md.Code(param.Value.(*ast.BasicLit).Value)
									}
								case "RuErr":
									if binaryExpr, ok := param.Value.(*ast.BinaryExpr); ok {
										row[3] = md.Code(getBinaryExprString(binaryExpr))
									} else {
										row[3] = md.Code(param.Value.(*ast.BasicLit).Value)
									}
								}

								if row[1] == "" {
									statusName := "StatusBadRequest"
									row[1] = fmt.Sprintf("%s %s", getStatusCode(statusName), md.Italic(statusName))
								}
							}
						}
						rows = append(rows, row)
					}
				}
			}
		}
	}
	return rows
}

// getBinaryExprString Преобразует бинарное выражение AST в строку.  Используется для получения представления бинарного выражения в виде строки, пригодной для отображения в таблице ошибок.
//
// Параметры:
//   - expr: бинарное выражение AST.
//
// Возвращает:
//   - строка: строка, представляющая бинарное выражение.
func getBinaryExprString(expr *ast.BinaryExpr) string {
	str := ""
	switch x := expr.X.(type) {
	case *ast.BasicLit:
		str += strings.Trim(x.Value, "\"")
	case *ast.BinaryExpr:
		str += getBinaryExprString(x)
	case *ast.CallExpr:
		fmt.Println(x)
	}

	switch y := expr.Y.(type) {
	case *ast.BasicLit:
		str += strings.Trim(y.Value, "\"")
	case *ast.BinaryExpr:
		str += getBinaryExprString(y)
	case *ast.CallExpr:
		str += y.Args[0].(*ast.Ident).Obj.Decl.(*ast.ValueSpec).Values[0].(*ast.BasicLit).Value
	}

	return str
}

// getStatusCode Преобразует строку статуса в HTTP код.
//
// Парамметры:
//   - status: Строка, представляющая статус.
//
// Возвращает:
//   - HTTP код, соответствующий статусу.
func getStatusCode(status string) string {
	switch status {
	case "StatusContinue":
		return "100"
	case "StatusSwitchingProtocols":
		return "101"
	case "StatusProcessing":
		return "102"
	case "StatusEarlyHints":
		return "103" // RFC 8297

	case "StatusOK":
		return "200"
	case "StatusCreated":
		return "201"
	case "StatusAccepted":
		return "202"
	case "StatusNonAuthoritativeInfo":
		return "203"
	case "StatusNoContent":
		return "204"
	case "StatusResetContent":
		return "205"
	case "StatusPartialContent":
		return "206"
	case "StatusMultiStatus":
		return "207"
	case "StatusAlreadyReported":
		return "208"
	case "StatusIMUsed":
		return "226"

	case "StatusMultipleChoices":
		return "300"
	case "StatusMovedPermanently":
		return "301"
	case "StatusFound":
		return "302"
	case "StatusSeeOther":
		return "303"
	case "StatusNotModified":
		return "304"
	case "StatusUseProxy":
		return "305"
	case "StatusTemporaryRedirect":
		return "307"
	case "StatusPermanentRedirect":
		return "308"

	case "StatusBadRequest":
		return "400"
	case "StatusUnauthorized":
		return "401"
	case "StatusPaymentRequired":
		return "402"
	case "StatusForbidden":
		return "403"
	case "StatusNotFound":
		return "404"
	case "StatusMethodNotAllowed":
		return "405"
	case "StatusNotAcceptable":
		return "406"
	case "StatusProxyAuthRequired":
		return "407"
	case "StatusRequestTimeout":
		return "408"
	case "StatusConflict":
		return "409"
	case "StatusGone":
		return "410"
	case "StatusLengthRequired":
		return "411"
	case "StatusPreconditionFailed":
		return "412"
	case "StatusRequestEntityTooLarge":
		return "413"
	case "StatusRequestURITooLong":
		return "414"
	case "StatusUnsupportedMediaType":
		return "415"
	case "StatusRequestedRangeNotSatisfiable":
		return "416"
	case "StatusExpectationFailed":
		return "417"
	case "StatusTeapot":
		return "418"
	case "StatusMisdirectedRequest":
		return "421"
	case "StatusUnprocessableEntity":
		return "422"
	case "StatusLocked":
		return "423"
	case "StatusFailedDependency":
		return "424"
	case "StatusTooEarly":
		return "425"
	case "StatusUpgradeRequired":
		return "426"
	case "StatusPreconditionRequired":
		return "428"
	case "StatusTooManyRequests":
		return "429"
	case "StatusRequestHeaderFieldsTooLarge":
		return "431"
	case "StatusUnavailableForLegalReasons":
		return "451"

	case "StatusInternalServerError":
		return "500"
	case "StatusNotImplemented":
		return "501"
	case "StatusBadGateway":
		return "502"
	case "StatusServiceUnavailable":
		return "503"
	case "StatusGatewayTimeout":
		return "504"
	case "StatusHTTPVersionNotSupported":
		return "505"
	case "StatusVariantAlsoNegotiates":
		return "506"
	case "StatusInsufficientStorage":
		return "507"
	case "StatusLoopDetected":
		return "508"
	case "StatusNotExtended":
		return "510"
	case "StatusNetworkAuthenticationRequired":
		return "511"
	}
	return ""
}
