// Управление процессом удаления данных.  Отслеживает текущие удаления и предоставляет функциональность для начала, завершения и ожидания завершения операций удаления.
package dao

import (
	"sync"
	"time"
)

// DeletionWatcher
// -migration
type DeletionWatcher struct {
	txs map[string]struct{} // Maybe add transaction object for rollback
	mu  sync.RWMutex
}

// NewDeletionWatcher создает новый экземпляр DeletionWatcher. Он инициализирует внутреннее отображение txs для отслеживания текущих операций удаления.
//
// Возвращает:
//   - *DeletionWatcher: новый экземпляр DeletionWatcher.
//
// Принимает:
//   - Нет.
func NewDeletionWatcher() *DeletionWatcher {
	return &DeletionWatcher{txs: make(map[string]struct{})}
}

// StartDeletion запускает процесс удаления с указанным идентификатором.  Добавляет идентификатор в список текущих операций удаления для отслеживания.
//
// Параметры:
//   - id: Уникальный идентификатор объекта, который необходимо удалить.
//
// Возвращает:
//   - Нет (nil).
func (w *DeletionWatcher) StartDeletion(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.txs[id] = struct{}{}
}

// FinishDeletion завершает операцию удаления с указанным идентификатором.  Удаляет идентификатор из списка текущих операций удаления, отслеживаемых DeletionWatcher.  Это сигнализирует о завершении процесса удаления для данного объекта. Функция не возвращает значения.
func (w *DeletionWatcher) FinishDeletion(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.txs, id)
}

// IsDeleting проверяет, находится ли объект с заданным идентификатором в процессе удаления.
//
// Параметры:
//   - id: Уникальный идентификатор объекта.
//
// Возвращает:
//   - bool: true, если объект в процессе удаления, false в противном случае.
func (w *DeletionWatcher) IsDeleting(id string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.txs[id]
	return ok
}

// RuNNingDeletions возвращает список всех объектов, находящихся в процессе удаления.
// Используется для отслеживания текущих операций удаления и может быть полезен для мониторинга или отката операций.
//
// Возвращаемые значения:
//   - []string: Слайс строк, содержащий идентификаторы объектов, находящихся в процессе удаления.
func (w *DeletionWatcher) RunningDeletions() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	res := make([]string, len(w.txs))
	i := 0
	for id := range w.txs {
		res[i] = id
		i++
	}
	return res
}

// :
func (w *DeletionWatcher) WaitAll(ids []string) {
	for {
		count := 0
		for _, id := range ids {
			if w.IsDeleting(id) {
				count++
			}
		}
		if count == 0 {
			return
		}

		time.Sleep(time.Millisecond * 100)
	}
}
