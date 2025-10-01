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
	"encoding/base64"
	"errors"
	"iter"
	"net/http"
	"net/url"

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
	AddIds      []string
	DelIds      []string
	InvolvedIds []string
}

func CalculateIDChanges(reqIDs, curIDs []interface{}) (*IDChangeSet, error) {
	result := IDChangeSet{}
	oldMap := make(map[string]struct{})
	newMap := make(map[string]struct{})
	uniqueMap := make(map[string]int)
	var involvedIds []string

	for _, v := range reqIDs {
		newMap[v.(string)] = struct{}{}
		uniqueMap[v.(string)] = 0
	}
	for _, v := range curIDs {
		oldMap[v.(string)] = struct{}{}
		uniqueMap[v.(string)] = 0
	}

	for k, _ := range uniqueMap {
		if _, ok := newMap[k]; ok {
			uniqueMap[k] += 1
		}
		if _, ok := oldMap[k]; ok {
			uniqueMap[k] -= 1
		}

		if uniqueMap[k] != 0 {
			involvedIds = append(involvedIds, k)
		}
	}

	for _, id := range involvedIds {
		switch uniqueMap[id] {
		case -1:
			result.DelIds = append(result.DelIds, id)
		case 1:
			result.AddIds = append(result.AddIds, id)
		}
	}
	result.InvolvedIds = involvedIds

	return &result, nil
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
