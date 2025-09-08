// Определяет типы данных для работы с датами и временем, а также для представления таблиц активности по дням недели.  Предоставляет удобные средства для сериализации этих типов в JSON и текстовые форматы.
//
// Основные возможности:
//   - Представление дней недели в сокращенном виде (WeekdayShort).
//   - Работа с датами и временем (Day).
//   - Создание таблиц активности, где каждый день недели связан с количеством каких-либо событий (ActivityTable).  Поддержка сериализации таблиц в JSON и текстовый формат для удобного хранения и обмена данными.
package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type WeekdayShort time.Weekday

func (w WeekdayShort) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%.3s\"", strings.ToLower(time.Weekday(w).String()))), nil
}

type Day time.Time

func (d Day) MarshalText() ([]byte, error) {
	return []byte(time.Time(d).Format("02012006")), nil
}

type ActivityTableDay struct {
	Weekday WeekdayShort `json:"weekday"`
	Count   int          `json:"count"`
}

type ActivityTable map[Day]ActivityTableDay

func (a ActivityTable) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[Day]ActivityTableDay(a))
}
