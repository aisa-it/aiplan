// Счетчик для отслеживания прогресса импорта задач и связанных ресурсов.
// Содержит информацию о количестве задач, импортированных задач,  прикрепленных файлов, пользователей и текущем состоянии базы данных.
//
// Основные возможности:
//   - Отслеживание общего количества задач для импорта.
//   - Отслеживание количества импортированных задач.
//   - Отслеживание количества прикрепленных файлов.
//   - Отслеживание количества пользователей.
//   - Отслеживание текущего этапа обработки базы данных.
package counters

import "sync/atomic"

type ImportCounters struct {
	// Всего задач для импорта
	TotalIssues int
	// Импортированных задач
	MappedIssues  atomic.Int32
	FetchedIssues atomic.Int32

	TotalAttachments    int
	ImportedAttachments atomic.Int32

	TotalUsers    int
	ImportedUsers atomic.Int32

	TotalDBStages  int
	CurrentDBStage int
}

func (ic *ImportCounters) GetFetchProgress() int {
	return int(float64(ic.FetchedIssues.Load()) / float64(ic.TotalIssues) * 100)
}

func (ic *ImportCounters) GetMappingProgress() int {
	return int(float64(ic.MappedIssues.Load()) / float64(ic.TotalIssues) * 100)
}

func (ic *ImportCounters) GetAttachmentsProgress() int {
	return int(float64(ic.ImportedAttachments.Load()) / float64(ic.TotalAttachments) * 100)
}

func (ic *ImportCounters) GetUsersProgress() int {
	return int(float64(ic.ImportedUsers.Load()) / float64(ic.TotalUsers) * 100)
}

func (ic *ImportCounters) GetDBProgress() int {
	return int(float64(ic.CurrentDBStage) / float64(ic.TotalDBStages) * 100)
}
