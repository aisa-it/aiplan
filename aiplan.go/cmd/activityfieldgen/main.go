// tools/codegen/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"text/template"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm/schema"
)

type StructField struct {
	Name     string
	Type     string
	FieldTag actField.ActivityField
}

type StructInfo struct {
	Name   string
	Fields []StructField
}

func main() {
	structs := generateStructs()
	filePath := "internal/aiplan/dao/activity-extend-fields.go"

	generateOutput(structs, filePath)

	if err := formatFile(filePath); err != nil {
		log.Fatal("Failed to format file:", err)
	}
}

func getTypePtr(t interface{}) string {
	typeStr := reflect.TypeOf(t).String()
	parts := strings.Split(typeStr, ".")
	if len(parts) > 1 && parts[0] == "dao" {
		return "*" + parts[1]
	}
	return "*" + typeStr
}

func withTable(field actField.ActivityField, table schema.Tabler) actField.ActivityField {
	return actField.ActivityField(fmt.Sprintf("%s::%s", field.String(), table.TableName()))
}

func generateStructs() []StructInfo {
	return []StructInfo{
		DocMemberExtendFields,
		DocAttachmentExtendFields,
		DocCommentExtendFields,
		DocExtendFields,
	}
}

func generateOutput(structs []StructInfo, outputPath string) {
	tmpl := `package dao


{{range .}}
// {{.Name}}
// -migration
type {{.Name}} struct {
    {{- range .Fields}}
    {{.Name}} {{.Type}} ` + "`json:\"-\" gorm:\"-\" field:\"{{.FieldTag}}\" extensions:\"x-nullable\"`" + `
    {{- end}}
}

{{end}}`

	f, err := os.Create(outputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	t := template.Must(template.New("models").Parse(tmpl))
	if err := t.Execute(f, structs); err != nil {
		log.Fatal(err)
	}

}

func formatFile(filePath string) error {
	cmd := exec.Command("go", "fmt", filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
