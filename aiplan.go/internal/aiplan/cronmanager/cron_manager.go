// Пакет для управления cron-задачами.
//
// Основные возможности:
//   - Загрузка задач из реестра.
//   - Добавление задач в cron-расписание.
//   - Удаление задач из cron-расписания.
//   - Запуск и остановка cron-диспетчера.
package cronmanager

import (
	"fmt"
	"sync"

	"log/slog"

	"github.com/robfig/cron/v3"
)

type CronJobFunc func()

type Job struct {
	Func     CronJobFunc
	Schedule string
}

type JobRegistry map[string]Job

type CronManager struct {
	dispatcher  *cron.Cron
	jobs        map[string]cron.EntryID
	mu          sync.Mutex
	jobRegistry JobRegistry
}

// NewCronManager создает новый менеджер для планирования задач.
// Параметры:
//   - jobRegistry: реестр задач
//
// Возвращает:
//   - *CronManager: созданный менеджер для планирования задач
func NewCronManager(jobRegistry JobRegistry) *CronManager {
	dispatcher := cron.New(
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
	)

	return &CronManager{
		dispatcher:  dispatcher,
		jobs:        make(map[string]cron.EntryID),
		jobRegistry: jobRegistry,
	}
}

// LoadJobs загружает задачи.
//
// Параметры:
//   - jobs: список задач
//
// Возвращает:
//   - error: ошибка, если произошла
func (cm *CronManager) LoadJobs() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Clear existing jobs
	for name, entryID := range cm.jobs {
		cm.dispatcher.Remove(entryID)
		delete(cm.jobs, name)
	}

	// Add jobs from the registry
	for name, job := range cm.jobRegistry {
		if err := cm.addJob(name, job.Schedule); err != nil {
			slog.Error("Error adding job", "name", name, "err", err)
		}
	}

	return nil
}

// addJob добавляет новую задачу в расписание.
//
// Параметры:
//   - name: имя задачи
//   - schedule: расписание выполнения задачи
//
// Возвращает:
//   - error: ошибка, если задача не может быть добавлена
func (cm *CronManager) addJob(name, schedule string) error {
	job, exists := cm.jobRegistry[name]
	if !exists {
		return fmt.Errorf("No job function registered for name: %s", name)
	}

	id, err := cm.dispatcher.AddFunc(schedule, job.Func)
	if err != nil {
		slog.Error("Failed to add job", "name", name, "err", err)
		return fmt.Errorf("Failed to add job '%s': %v", name, err)
	}
	cm.jobs[name] = id
	return nil
}

// RemoveJob removes a job by its name.
// Parameters:
//   - name: the name of the job to be removed
//
// Returns:
//   - error: any error encountered during the removal process
func (cm *CronManager) RemoveJob(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if entryID, exists := cm.jobs[name]; exists {
		cm.dispatcher.Remove(entryID)
		delete(cm.jobs, name)
	}
}

// Start запускает процесс обработки данных.
// Параметры:
//   - ctx: контекст для отмены операции
//   - cfg: конфигурация параметров обработки
//
// Возвращает:
//   - error: ошибка, если возникла
func (cm *CronManager) Start() {
	cm.dispatcher.Start()
}

// Stop останавливает выполнение задачи.
//
// Параметры:
//   - taskID: идентификатор задачи для остановки
//   - force: флаг, указывающий на принудительную остановку
//
// Возвращает:
//   - error: ошибка, если таковая возникла
func (cm *CronManager) Stop() {
	ctx := cm.dispatcher.Stop()
	<-ctx.Done()
}
