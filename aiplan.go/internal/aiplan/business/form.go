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

func GenBodyAnswer(answer *dao.FormAnswer, user *dao.User) (string, error) {
	fileName := make(map[string]string, len(answer.Attachments))
	for _, attachment := range answer.Attachments {
		fileName[attachment.Id.String()] = attachment.Asset.Name
	}

	t, err := template.New("AnswerIssue").Funcs(template.FuncMap{
		"getValString": func(t string, val interface{}) template.HTML {
			switch t {
			case "checkbox":
				if val == nil {
					return template.HTML("Нет")
				} else {
					if v := val.(bool); v {
						return template.HTML("Да")
					} else {
						return template.HTML("Нет")
					}
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
			case "attachment":
				if v, ok := fileName[fmt.Sprint(val)]; ok {
					return template.HTML(v)
				} else {
					return template.HTML(fmt.Sprint(val))
				}
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
