// Вспомогательные функции для работы с данными, часто используемые в различных частях приложения.  Включает в себя преобразования между слайсами и множествами, а также полезные утилиты для обработки данных.
//
// Основные возможности:
//   - Преобразование слайсов в множества (map[T]struct{}).
//   - Проверка наличия элементов множества в другом множестве или слайсе.
//   - Преобразование слайсов в слайсы другого типа с применением функции.
//   - Преобразование множеств в слайсы.
//   - Преобразование слайсов значений в map, где ключ - результат функции над значением, а значение - само значение.
package utils

import (
  "database/sql"
  "encoding/base64"
  "errors"
  "fmt"
  "iter"
  "net/http"
  "net/url"
  "time"

  "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

  "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
  "github.com/gofrs/uuid"
)

var (
  ValidStatusEmoji = map[string]string{
    "🔕":  "Не беспокоить",
    "🍽️": "Обед",
    "🎧":  "На звонке",
    "🏖️": "Отпуск",
    "🤒":  "Больничный",
    "💬":  "",
    "":   "",
  }

  ValidIssueStatusGroup = map[string]struct{}{
    "unstarted": struct{}{},
    "completed": struct{}{},
    "started":   struct{}{},
    "backlog":   struct{}{},
    "cancelled": struct{}{},
  }

  PrioritiesSortValues = map[string]int{
    "urgent": 0,
    "high":   1,
    "medium": 2,
    "low":    3,
    "":       4,
  }
)

type IDChangeSet struct {
  AddIds      []uuid.UUID
  DelIds      []uuid.UUID
  InvolvedIds []uuid.UUID
}

func CalculateIDChanges(reqIDs, curIDs []any) *IDChangeSet {
  result := IDChangeSet{}
  oldMap := make(map[uuid.UUID]struct{})
  newMap := make(map[uuid.UUID]struct{})

  for _, v := range reqIDs {
    switch vv := v.(type) {
    case string:
      newMap[uuid.Must(uuid.FromString(vv))] = struct{}{}
    case uuid.UUID:
      newMap[vv] = struct{}{}
    }
  }

  // Find deleted ids
  for _, v := range curIDs {
    var key uuid.UUID
    switch vv := v.(type) {
    case string:
      key = uuid.Must(uuid.FromString(vv))
    case uuid.UUID:
      key = vv
    }
    oldMap[key] = struct{}{}

    if _, ok := newMap[key]; !ok {
      result.DelIds = append(result.DelIds, key)
    }
  }

  // Find added ids
  for key, _ := range newMap {
    if _, ok := oldMap[key]; !ok {
      result.AddIds = append(result.AddIds, key)
    }
  }

  result.InvolvedIds = append(result.AddIds, result.DelIds...)

  return &result
}

func SliceToSet[T comparable](ids []T) map[T]struct{} {
  res := make(map[T]struct{})
  for _, id := range ids {
    res[id] = struct{}{}
  }
  return res
}

func CheckInSet[T comparable](set map[T]struct{}, all ...T) bool {
  for _, el := range all {
    if _, ok := set[el]; ok {
      return true
    }
  }
  return false
}

func CheckInSlice[T comparable](in []T, all ...T) bool {
  set := SliceToSet(in)
  return CheckInSet(set, all...)
}

func SliceToSlice[T any, U any](in *[]T, f func(*T) U) []U {
  if in == nil {
    return make([]U, 0)
  }
  out := make([]U, len(*in))
  for i, v := range *in {
    out[i] = f(&v)
  }
  return out
}

func SetToSlice[T comparable](in map[T]struct{}) []T {
  var out []T
  for k, _ := range in {
    out = append(out, k)
  }
  return out
}

func SliceToMap[K comparable, V any](in *[]V, f func(*V) K) map[K]V {
  out := make(map[K]V, 0)
  if in == nil {
    return out
  }
  for _, v := range *in {
    out[f(&v)] = v
  }
  return out
}

func MergeSlices[T any](slices ...[]T) []T {
  lenSl := 0
  for _, s := range slices {
    lenSl += len(s)
  }

  result := make([]T, 0, lenSl)
  for _, s := range slices {
    result = append(result, s...)
  }
  return result
}

func ToPtr[T any](b T) *T {
  return &b
}

func MergeUniqueSlices[T comparable](slices ...[]T) []T {
  seen := make(map[T]bool)
  result := make([]T, 0)

  for _, slice := range slices {
    for _, item := range slice {
      if !seen[item] {
        result = append(result, item)
        seen[item] = true
      }
    }
  }

  return result
}

func Filter[T any](seq iter.Seq[T], by func(T) bool) iter.Seq[T] {
  return func(yield func(T) bool) {
    for i := range seq {
      if by(i) {
        if !yield(i) {
          return
        }
      }
    }
  }
}

func All[T any](res []T) iter.Seq[T] {
  return func(yield func(T) bool) {
    for i := range res {
      if !yield(res[i]) {
        return
      }
    }
  }
}

func Collect[T any](seq iter.Seq[T]) []T {
  var out []T
  seq(func(val T) bool {
    out = append(out, val)
    return true
  })
  return out
}

func CheckHttps(u *url.URL) *url.URL {
  u.Scheme = "https"
  resp, err := http.Get(u.String())
  if errors.Is(err, http.ErrSchemeMismatch) {
    u.Scheme = "http"
    return u
  }
  if resp != nil {
    resp.Body.Close()
  }
  return u
}

func MergeMaps[K comparable, V any](maps ...map[K]V) map[K]V {
  merged := make(map[K]V)
  for _, m := range maps {
    for k, v := range m {
      merged[k] = v
    }
  }
  return merged
}

func CompareUsers(a *dto.UserLight, b *dto.UserLight) int {
  if a == b {
    return 0
  }
  if b == nil || (a != nil && a.GetName() < b.GetName()) {
    return -1
  }
  if a == nil || a.GetName() > b.GetName() {
    return 1
  }
  return 0
}

func UUIDToBase64(u uuid.UUID) string {
  return base64.RawURLEncoding.EncodeToString(u[:])
}

func Base64ToUUID(encoded string) (uuid.UUID, error) {
  data, err := base64.RawURLEncoding.DecodeString(encoded)
  if err != nil {
    return uuid.Nil, err
  }

  return uuid.FromBytes(data)
}

func FormatDateStr(dateStr, outFormat string, tz *types.TimeZone) (string, error) {
  date, err := FormatDate(dateStr)
  if err != nil {
    return "", err
  }

  if tz != nil {
    date = date.In((*time.Location)(tz))
  }
  sss := date.Format(outFormat)
  return sss, nil

}

func FormatDate(dateStr string) (time.Time, error) {
  if dateStr == "" {
    return time.Time{}, fmt.Errorf("empty date string")
  }

  layouts := []string{
    time.RFC3339,
    "2006-01-02T15:04:05",
    "2006-01-02T15:04:05.000Z",
    "2006-01-02T15:04:05Z07:00",
    "2006-01-02 15:04:05Z07:00",
    "2006-01-02 15:04:05",
    "2006-01-02 15:04:05 -0700 MST",
    "2006-01-02",
    "02.01.2006 15:04 MST",
    "02.01.2006 15:04 -0700",
    "02.01.2006",
    "2006-01-02 15:04:05-07",
    "2006-01-02 15:04:05 -0700",
    "2006-01-02T15:04:05+07:00",
  }

  var t time.Time
  var err error
  for _, layout := range layouts {
    t, err = time.Parse(layout, dateStr)
    if err == nil {
      return t, nil
    }
  }
  return time.Time{}, fmt.Errorf("unsuported date format")
}

func FormatDateToSqlNullTime(dateStr string) sql.NullTime {
  if dateStr == "" {
    return sql.NullTime{}
  }
  date, err := FormatDate(dateStr)
  if err != nil {
    return sql.NullTime{}
  }
  return sql.NullTime{Valid: true, Time: date}
}
