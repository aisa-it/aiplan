package business

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

const answerIssueTmpl = `{{if .User}}<p>Пользователь: {{.User.GetName}}, {{.User.Email}}</p>{{else}}<p>Анонимный пользователь</p>{{end}}<ol>{{- range .Answers -}}<li><p><span style="font-size: 14px"><strong>{{- .Label -}}</strong></span><br><span style="font-size: 14px">{{- getValString .Type .Val -}}</span></p></li>{{- end -}}</ol>`
const answerTelegramTmpl = `{{if .User}}👤 *Пользователь:* {{.User.GetName}} ({{.User.Email}})
{{else}}👤 *Анонимный пользователь*
{{end}}
📋 *Ответы на форму:*
{{- range $index, $answer := .Answers}}
{{add $index 1}}. *{{$answer.Label}}*
   {{- getValStringTelegram $answer.Type $answer.Val -}}
{{- end}}`

func GenBodyAnswer(answer *dao.FormAnswer, user *dao.User) (string, error) {
	t, err := template.New("AnswerIssue").Funcs(template.FuncMap{
		"getValString": func(t string, val interface{}) template.HTML {
			switch t {
			case "checkbox":
				if v := val.(bool); v {
					return template.HTML("Да")
				} else {
					return template.HTML("Нет")
				}
			case "date":
				return template.HTML(time.UnixMilli(int64(val.(float64))).Format("02.01.2006"))
			case "multiselect":
				if values, ok := val.([]interface{}); ok {
					var stringValues []string
					for _, v := range values {
						stringValues = append(stringValues, fmt.Sprint(v))
					}
					return template.HTML(strings.Join(stringValues, "<br>"))
				}
				return template.HTML(fmt.Sprint(val))
			}
			return template.HTML(fmt.Sprint(val))
		},
	}).Parse(answerIssueTmpl)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, struct {
		User    *dao.User
		Answers types.FormFieldsSlice
	}{
		User:    user,
		Answers: answer.Fields,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func GenTelegramBodyAnswer(answer *dao.FormAnswer, user *dao.User) (string, error) {
	t, err := template.New("AnswerTelegram").Funcs(template.FuncMap{
		"getValStringTelegram": func(t string, val interface{}) string {
			switch t {
			case "checkbox":
				if v := val.(bool); v {
					return " ✅ Да"
				} else {
					return " ❌ Нет"
				}
			case "date":
				return " 📅 " + time.UnixMilli(int64(val.(float64))).Format("02.01.2006")
			case "multiselect":
				if values, ok := val.([]interface{}); ok {
					var stringValues []string
					for _, v := range values {
						stringValues = append(stringValues, fmt.Sprint(v))
					}
					return " 📌 " + strings.Join(stringValues, ", ")
				}
				return " " + fmt.Sprint(val)
			default:
				return " " + fmt.Sprint(val)
			}
		},
		"add": func(a, b int) int {
			return a + b
		},
	}).Parse(answerTelegramTmpl)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, struct {
		User    *dao.User
		Answers types.FormFieldsSlice
	}{
		User:    user,
		Answers: answer.Fields,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
