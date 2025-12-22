// Содержит бизнес-логику для получения статистики проектов.
// Функции предназначены для переиспользования в HTTP handlers и MCP tools.
package business

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
)

// maxStatsLimit максимальное количество записей в опциональных секциях статистики
const maxStatsLimit = 50

// GetProjectStats возвращает агрегированную статистику проекта.
// Функция выполняет агрегацию на стороне БД для оптимальной производительности.
//
// Параметры:
//   - projectID: UUID проекта
//   - opts: параметры запроса статистики (опциональные секции)
//
// Возвращает:
//   - *dto.ProjectStats: статистика проекта
//   - error: ошибка, если произошла при выполнении запросов
func (b *Business) GetProjectStats(projectID uuid.UUID, opts dto.ProjectStatsRequest) (*dto.ProjectStats, error) {
	stats := &dto.ProjectStats{}

	// 1. Получаем базовую информацию о проекте
	if err := b.getProjectBaseInfo(projectID, stats); err != nil {
		return nil, err
	}

	// 2. Получаем общие счётчики задач и распределение по группам статусов
	if err := b.getIssueCounters(projectID, stats); err != nil {
		return nil, err
	}

	// 3. Получаем распределение по статусам
	if err := b.getStatsByState(projectID, stats); err != nil {
		return nil, err
	}

	// 4. Получаем распределение по приоритетам
	if err := b.getStatsByPriority(projectID, stats); err != nil {
		return nil, err
	}

	// 5. Получаем статистику просроченных задач
	if err := b.getOverdueStats(projectID, stats); err != nil {
		return nil, err
	}

	// Опциональные секции
	if opts.IncludeAssigneeStats {
		if err := b.getAssigneeStats(projectID, stats); err != nil {
			return nil, err
		}
	}

	if opts.IncludeLabelStats {
		if err := b.getLabelStats(projectID, stats); err != nil {
			return nil, err
		}
	}

	if opts.IncludeSprintStats {
		if err := b.getSprintStats(projectID, stats); err != nil {
			return nil, err
		}
	}

	if opts.IncludeTimeline {
		if err := b.getTimelineStats(projectID, stats); err != nil {
			return nil, err
		}
	}

	return stats, nil
}

// getProjectBaseInfo получает базовую информацию о проекте.
func (b *Business) getProjectBaseInfo(projectID uuid.UUID, stats *dto.ProjectStats) error {
	type projectInfo struct {
		ID           uuid.UUID
		Name         string
		Identifier   string
		TotalMembers int
	}

	var info projectInfo
	err := b.db.Model(&dao.Project{}).
		Select("projects.id, projects.name, projects.identifier, (?) as total_members",
			b.db.Model(&dao.ProjectMember{}).Select("count(*)").Where("project_id = projects.id"),
		).
		Where("projects.id = ?", projectID).
		Scan(&info).Error

	if err != nil {
		return err
	}

	stats.Project = dto.ProjectStatsProject{
		ID:           info.ID,
		Name:         info.Name,
		Identifier:   info.Identifier,
		TotalMembers: info.TotalMembers,
	}
	return nil
}

// getIssueCounters получает общие счётчики задач и распределение по группам статусов.
func (b *Business) getIssueCounters(projectID uuid.UUID, stats *dto.ProjectStats) error {
	// Получаем счётчики по группам статусов одним запросом
	var groups []struct {
		Group string
		Count int
	}

	err := b.db.Model(&dao.Issue{}).
		Select("s.group as \"group\", COUNT(*) as count").
		Joins("JOIN states s ON s.id = issues.state_id").
		Where("issues.project_id = ?", projectID).
		Group("s.group").
		Scan(&groups).Error

	if err != nil {
		return err
	}

	// Заполняем счётчики
	var total, active, completed, cancelled int
	for _, g := range groups {
		switch g.Group {
		case "backlog":
			stats.ByStateGroup.Backlog = g.Count
			active += g.Count
		case "unstarted":
			stats.ByStateGroup.Unstarted = g.Count
			active += g.Count
		case "started":
			stats.ByStateGroup.Started = g.Count
			active += g.Count
		case "completed":
			stats.ByStateGroup.Completed = g.Count
			completed = g.Count
		case "cancelled":
			stats.ByStateGroup.Cancelled = g.Count
			cancelled = g.Count
		}
		total += g.Count
	}

	stats.Issues = dto.IssueCounters{
		Total:     total,
		Active:    active,
		Completed: completed,
		Cancelled: cancelled,
	}

	return nil
}

// getStatsByState получает распределение задач по статусам.
func (b *Business) getStatsByState(projectID uuid.UUID, stats *dto.ProjectStats) error {
	var result []struct {
		StateID uuid.UUID
		Name    string
		Group   string
		Color   string
		Count   int
	}

	err := b.db.Model(&dao.Issue{}).
		Select("s.id as state_id, s.name, s.group as \"group\", s.color, COUNT(*) as count").
		Joins("JOIN states s ON s.id = issues.state_id").
		Where("issues.project_id = ?", projectID).
		Group("s.id, s.name, s.group, s.color, s.sequence").
		Order("s.sequence").
		Scan(&result).Error

	if err != nil {
		return err
	}

	stats.ByState = make([]dto.StateStatItem, len(result))
	for i, r := range result {
		stats.ByState[i] = dto.StateStatItem{
			StateID: r.StateID,
			Name:    r.Name,
			Group:   r.Group,
			Color:   r.Color,
			Count:   r.Count,
		}
	}
	return nil
}

// getStatsByPriority получает распределение задач по приоритетам.
func (b *Business) getStatsByPriority(projectID uuid.UUID, stats *dto.ProjectStats) error {
	var result []struct {
		Priority *string
		Count    int
	}

	err := b.db.Model(&dao.Issue{}).
		Select("priority, COUNT(*) as count").
		Where("project_id = ?", projectID).
		Group("priority").
		Scan(&result).Error

	if err != nil {
		return err
	}

	for _, r := range result {
		if r.Priority == nil {
			stats.ByPriority.None = r.Count
		} else {
			switch *r.Priority {
			case "urgent":
				stats.ByPriority.Urgent = r.Count
			case "high":
				stats.ByPriority.High = r.Count
			case "medium":
				stats.ByPriority.Medium = r.Count
			case "low":
				stats.ByPriority.Low = r.Count
			}
		}
	}
	return nil
}

// getOverdueStats получает статистику просроченных задач.
func (b *Business) getOverdueStats(projectID uuid.UUID, stats *dto.ProjectStats) error {
	type overdueResult struct {
		Count      int
		OldestDate *time.Time
	}

	var result overdueResult
	now := time.Now()

	err := b.db.Model(&dao.Issue{}).
		Select("COUNT(*) as count, MIN(target_date) as oldest_date").
		Joins("JOIN states s ON s.id = issues.state_id").
		Where("issues.project_id = ?", projectID).
		Where("s.group NOT IN ('completed', 'cancelled')").
		Where("issues.target_date < ?", now).
		Where("issues.target_date IS NOT NULL").
		Scan(&result).Error

	if err != nil {
		return err
	}

	stats.Overdue = dto.OverdueStats{
		Count:      result.Count,
		OldestDate: result.OldestDate,
	}
	return nil
}

// getAssigneeStats получает статистику по исполнителям (топ-50).
func (b *Business) getAssigneeStats(projectID uuid.UUID, stats *dto.ProjectStats) error {
	var result []struct {
		UserID      uuid.UUID
		DisplayName string
		Active      int
		Completed   int
	}

	err := b.db.Model(&dao.IssueAssignee{}).
		Select(`
			u.id as user_id,
			CONCAT(u.first_name, ' ', u.last_name) as display_name,
			SUM(CASE WHEN s.group NOT IN ('completed', 'cancelled') THEN 1 ELSE 0 END) as active,
			SUM(CASE WHEN s.group = 'completed' THEN 1 ELSE 0 END) as completed
		`).
		Joins("JOIN users u ON u.id = issue_assignees.assignee_id").
		Joins("JOIN issues i ON i.id = issue_assignees.issue_id").
		Joins("JOIN states s ON s.id = i.state_id").
		Where("issue_assignees.project_id = ?", projectID).
		Group("u.id, u.first_name, u.last_name").
		Order("active DESC, completed DESC").
		Limit(maxStatsLimit).
		Scan(&result).Error

	if err != nil {
		return err
	}

	stats.AssigneeStats = make([]dto.AssigneeStatItem, len(result))
	for i, r := range result {
		stats.AssigneeStats[i] = dto.AssigneeStatItem{
			UserID:      r.UserID,
			DisplayName: r.DisplayName,
			Active:      r.Active,
			Completed:   r.Completed,
		}
	}
	return nil
}

// getLabelStats получает статистику по меткам (топ-50).
func (b *Business) getLabelStats(projectID uuid.UUID, stats *dto.ProjectStats) error {
	var result []struct {
		LabelID uuid.UUID
		Name    string
		Color   string
		Count   int
	}

	err := b.db.Model(&dao.IssueLabel{}).
		Select("l.id as label_id, l.name, l.color, COUNT(*) as count").
		Joins("JOIN labels l ON l.id = issue_labels.label_id").
		Where("issue_labels.project_id = ?", projectID).
		Group("l.id, l.name, l.color").
		Order("count DESC").
		Limit(maxStatsLimit).
		Scan(&result).Error

	if err != nil {
		return err
	}

	stats.LabelStats = make([]dto.LabelStatItem, len(result))
	for i, r := range result {
		stats.LabelStats[i] = dto.LabelStatItem{
			LabelID: r.LabelID,
			Name:    r.Name,
			Color:   r.Color,
			Count:   r.Count,
		}
	}
	return nil
}

// getSprintStats получает статистику по спринтам (последние 50).
func (b *Business) getSprintStats(projectID uuid.UUID, stats *dto.ProjectStats) error {
	var result []struct {
		SprintID  uuid.UUID
		Name      string
		Total     int
		Completed int
	}

	err := b.db.Model(&dao.SprintIssue{}).
		Select(`
			sp.id as sprint_id,
			sp.name,
			COUNT(*) as total,
			SUM(CASE WHEN s.group = 'completed' THEN 1 ELSE 0 END) as completed
		`).
		Joins("JOIN sprints sp ON sp.id = sprint_issues.sprint_id AND sp.deleted_at IS NULL").
		Joins("JOIN issues i ON i.id = sprint_issues.issue_id").
		Joins("JOIN states s ON s.id = i.state_id").
		Where("sprint_issues.project_id = ?", projectID).
		Group("sp.id, sp.name, sp.sequence_id").
		Order("sp.sequence_id DESC").
		Limit(maxStatsLimit).
		Scan(&result).Error

	if err != nil {
		return err
	}

	stats.SprintStats = make([]dto.SprintStatItem, len(result))
	for i, r := range result {
		stats.SprintStats[i] = dto.SprintStatItem{
			SprintID:  r.SprintID,
			Name:      r.Name,
			Total:     r.Total,
			Completed: r.Completed,
		}
	}
	return nil
}

// getTimelineStats получает временную статистику создания/завершения задач за последние 12 месяцев.
func (b *Business) getTimelineStats(projectID uuid.UUID, stats *dto.ProjectStats) error {
	timeline := &dto.TimelineStats{}

	// Созданные по месяцам (за последние 12 месяцев)
	var createdByMonth []struct {
		Month string
		Count int
	}

	err := b.db.Model(&dao.Issue{}).
		Select("TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*) as count").
		Where("project_id = ?", projectID).
		Where("created_at >= NOW() - INTERVAL '12 months'").
		Group("TO_CHAR(created_at, 'YYYY-MM')").
		Order("month").
		Scan(&createdByMonth).Error

	if err != nil {
		return err
	}

	timeline.CreatedByMonth = make([]dto.MonthlyCount, len(createdByMonth))
	for i, r := range createdByMonth {
		timeline.CreatedByMonth[i] = dto.MonthlyCount{
			Month: r.Month,
			Count: r.Count,
		}
	}

	// Завершённые по месяцам (за последние 12 месяцев)
	var completedByMonth []struct {
		Month string
		Count int
	}

	err = b.db.Model(&dao.Issue{}).
		Select("TO_CHAR(completed_at, 'YYYY-MM') as month, COUNT(*) as count").
		Where("project_id = ?", projectID).
		Where("completed_at IS NOT NULL").
		Where("completed_at >= NOW() - INTERVAL '12 months'").
		Group("TO_CHAR(completed_at, 'YYYY-MM')").
		Order("month").
		Scan(&completedByMonth).Error

	if err != nil {
		return err
	}

	timeline.CompletedByMonth = make([]dto.MonthlyCount, len(completedByMonth))
	for i, r := range completedByMonth {
		timeline.CompletedByMonth[i] = dto.MonthlyCount{
			Month: r.Month,
			Count: r.Count,
		}
	}

	stats.Timeline = timeline
	return nil
}
