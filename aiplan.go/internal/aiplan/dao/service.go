// Пакет dao содержит интерфейсы и структуры данных для доступа к данным приложения.  Он предоставляет абстракции для взаимодействия с базой данных, позволяя изолировать бизнес-логику от деталей реализации доступа к данным.
//
// Основные возможности:
//   - Определяет интерфейсы для работы с различными сущностями (User, Workspace, Project и т.д.).
//   - Предоставляет структуры данных для представления изменений сущностей (TemplateActivity, FullActivity).
//   - Содержит вспомогательные функции для преобразования данных в DTO (DTO).
//   - Обеспечивает гибкий механизм для работы с различными типами сущностей и активностей.
package dao

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type IDaoAct interface {
	GetId() uuid.UUID
	GetString() string
	GetEntityType() actField.ActivityField
}

type IRedactorHTML interface {
	GetRedactorHtml() types.RedactorHTML
}

// ActivitySender
// -migration
type ActivitySender struct {
	SenderTg int64 `json:"-" gorm:"-"`
}

func GetActionEntity(a ActivityEvent, pref string) any {
	val := reflect.ValueOf(a)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	typ := val.Type()
	return getEntityRecursive(val, typ, pref)
}

func getEntityRecursive(val reflect.Value, typ reflect.Type, pref string) any {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		if !fieldVal.IsValid() || !fieldVal.CanInterface() {
			continue
		}

		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			if res := getEntityRecursive(fieldVal, fieldVal.Type(), pref); res != nil {
				return res
			}
			continue
		}

		tag := field.Tag.Get("field")
		if tag == "" || !strings.HasPrefix(field.Name, pref) {
			continue
		}

		if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
			method := fieldVal.MethodByName("ToLightDTO")
			if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
				return method.Call(nil)[0].Interface()
			} else {
				errMsg := fmt.Sprintf("Err: %s not have method 'ToLightDTO'", fieldVal.Type().String())
				slog.Info(errMsg)
			}
		}
	}
	return nil
}
