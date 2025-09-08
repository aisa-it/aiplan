// Пакет предоставляет атомарные структуры данных и инструменты для безопасной работы с данными в многопоточных средах.
//
// Основные возможности:
//   - `SyncMap`: Атомарная карта для хранения данных с защитой от гонок.
//   - `ImportMap`: Атомарная карта с возможностью преобразования ключей.
//   - `AtomicArray`: Атомарный массив для безопасного доступа и изменения элементов.
//   - `SortOrderCounter`: Счетчик для отслеживания порядка сортировки, связанный с UUID родительского элемента.
package atomic

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/gofrs/uuid"
)

type SyncMap[K comparable, V any] struct {
	entities map[K]V
	mutex    sync.RWMutex
}

func NewSyncMap[K comparable, V any]() SyncMap[K, V] {
	return SyncMap[K, V]{entities: make(map[K]V)}
}

func (m *SyncMap[K, V]) Put(key K, value V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entities[key] = value
}

func (m *SyncMap[K, V]) Get(key K) V {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.entities[key]
}

func (m *SyncMap[K, V]) Delete(key K) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.entities, key)
}

func (m *SyncMap[K, V]) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entities = make(map[K]V)
}

func (m *SyncMap[K, V]) Range(fn func(key K, value V)) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for k, v := range m.entities {
		fn(k, v)
	}
}

func (m *SyncMap[K, V]) ValueArray() []V {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	var arr []V
	for _, v := range m.entities {
		arr = append(arr, v)
	}
	return arr
}

func (m *SyncMap[K, V]) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.entities)
}

func (m *SyncMap[K, V]) Dump(name string) error {
	m.mutex.RLock()
	d, err := json.Marshal(m.entities)
	m.mutex.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(name, d, 0644)
}

func (m *SyncMap[K, V]) Load(name string) error {
	d, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return json.Unmarshal(d, &m.entities)
}

func (m *SyncMap[K, V]) Contains(key K) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	_, ok := m.entities[key]
	return ok
}

type ImportMap[T any] struct {
	entities      map[string]T
	mutex         sync.RWMutex
	translateFunc func(string) (T, error)
}

func (m *ImportMap[T]) GetLight(key string) T {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.entities[key]
}

func (m *ImportMap[T]) Get(key string) (T, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.GetNoLock(key)
}

func (m *ImportMap[T]) GetNoLock(key string) (T, error) {
	if e, ok := m.entities[key]; ok {
		return e, nil
	}
	if m.translateFunc != nil {
		entity, err := m.translateFunc(key)
		if err != nil {
			return entity, err
		}
		m.entities[key] = entity
		return entity, nil
	}
	var res T
	return res, nil
}

func (m *ImportMap[T]) Put(key string, entity T) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entities[key] = entity
}

func (m *ImportMap[T]) PutNoLock(key string, entity T) {
	m.entities[key] = entity
}

func (m *ImportMap[T]) Contains(key string) bool {
	m.mutex.RLock()
	_, ok := m.entities[key]
	m.mutex.RUnlock()
	return ok
}

func (m *ImportMap[T]) Array() []T {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	var arr []T
	for _, v := range m.entities {
		arr = append(arr, v)
	}
	return arr
}

func (m *ImportMap[T]) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.entities)
}

func (m *ImportMap[T]) Range(iter func(k string, v T)) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for k, v := range m.entities {
		iter(k, v)
	}
}

func (m *ImportMap[T]) Delete(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.entities, key)
}

func (m *ImportMap[T]) ValueArray() []T {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	res := make([]T, len(m.entities))

	i := 0
	for _, v := range m.entities {
		res[i] = v
		i++
	}
	return res
}

func (m *ImportMap[T]) Dump(name string) error {
	m.mutex.RLock()
	d, err := json.Marshal(m.entities)
	m.mutex.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(name, d, 0644)
}

func (m *ImportMap[T]) Load(name string) error {
	d, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return json.Unmarshal(d, &m.entities)
}

func NewImportMap[T any](translateFunc func(string) (T, error)) ImportMap[T] {
	return ImportMap[T]{translateFunc: translateFunc, entities: make(map[string]T)}
}

type ConvertMap[T any] struct {
	SyncMap[string, T]
	convertFunc func(T) string
}

func (m *ConvertMap[T]) Put(entity T) {
	m.SyncMap.Put(m.convertFunc(entity), entity)
}

func NewConvertMap[T any](convertFunc func(T) string) ConvertMap[T] {
	return ConvertMap[T]{convertFunc: convertFunc, SyncMap: NewSyncMap[string, T]()}
}

type AtomicArray[T any] struct {
	arr []T
	mu  sync.Mutex
}

func (arr *AtomicArray[T]) Index(i int) T {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	return arr.arr[i]
}

func (arr *AtomicArray[T]) Range(iter func(int, T)) {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	for i, el := range arr.arr {
		iter(i, el)
	}
}

func (arr *AtomicArray[T]) Append(el ...T) {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	arr.arr = append(arr.arr, el...)
}

func (arr *AtomicArray[T]) Len(el ...T) int {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	return len(arr.arr)
}

func (arr *AtomicArray[T]) Array() []T {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	return arr.arr
}

func (arr *AtomicArray[T]) Clear() {
	arr.mu.Lock()
	defer arr.mu.Unlock()
	arr.arr = nil
}

type SortOrderCounter struct {
	m     map[uuid.UUID]int
	mutex sync.Mutex
}

func NewSortOrderCounter() SortOrderCounter {
	return SortOrderCounter{m: make(map[uuid.UUID]int)}
}

func (c *SortOrderCounter) GetNext(parentId uuid.NullUUID) int {
	if !parentId.Valid {
		return 0
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	current := c.m[parentId.UUID]
	c.m[parentId.UUID] = current + 1
	return current + 1
}
