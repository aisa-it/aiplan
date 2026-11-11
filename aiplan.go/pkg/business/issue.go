package business

import (
	"errors"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	errStack "github.com/aisa-it/aiplan/aiplan.go/pkg/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// CreateIssueComment создает новый комментарий к задаче. Метод принимает задачу, пользователя, текст комментария, комментарий для базы данных, ID для ответа на комментарий и дополнительные метаданные. Возвращает ошибку, если произошла ошибка.
func (b *Business) CreateIssueComment(issue dao.Issue, user dao.User, text string, replyToId uuid.UUID, fromTg bool, meta ...string) error {
	// Check rights
	var permitted bool
	if err := b.db.Model(&dao.ProjectMember{}).
		Select("EXISTS(?)",
			b.db.Model(&dao.ProjectMember{}).
				Select("1").
				Where("project_id = ?", issue.ProjectId).
				Where("member_id = ?", user.ID).
				Where("role > ?", types.GuestRole),
		).
		Find(&permitted).Error; err != nil {
		return err
	}
	if !user.IsSuperuser && !permitted {
		return errors.New("create comment forbidden")
	}

	actorId := uuid.NullUUID{UUID: user.ID, Valid: true}
	comment := dao.IssueComment{
		Id:          dao.GenUUID(),
		WorkspaceId: issue.WorkspaceId,
		ProjectId:   issue.ProjectId,
		IssueId:     issue.ID,
		ActorId:     actorId,
		CommentHtml: types.RedactorHTML{Body: text},
	}
	if len(meta) > 0 {
		comment.IntegrationMeta = strings.Join(meta, ",")
	}
	if !replyToId.IsNil() {
		comment.ReplyToCommentId = uuid.NullUUID{UUID: replyToId, Valid: true}
	}
	if err := b.db.Create(&comment).Error; err != nil {
		return err
	}
	issue.UpdatedAt = time.Now()
	if err := b.db.Select("updated_at").Updates(&issue).Error; err != nil {
		return err
	}

	trackOpts := []tracker.TrackOption{
		tracker.WithNewVal(comment.CommentHtml.String()),
		tracker.WithNewID(comment.Id),
	}
	if fromTg && user.TelegramId != nil {
		trackOpts = append(trackOpts, tracker.WithTgSender(*user.TelegramId))
	}

	err := b.st.TrackVerb(types.LayerIssue, activities.VerbCreated, &issue, &user, trackOpts...)
	if err != nil {
		errStack.GetError(nil, err)
	}

	//newSnapshot := tracker.CommentToSnapshot(&comment, trackOpts...)

	//err := s.snapshotTracker.TrackChanges(types.LayerIssue, nil, newSnapshot, &issue, user)
	//if err != nil {
	//	errStack.GetError(nil, err)
	//}

	return err
}

// SetIssueState переводит задачу в статус вне HTTP-запроса — для движков,
// которые меняют статус сами (по таймеру, по внешнему событию). Смена
// попадает в историю задачи так же, как сделанная пользователем: с датами
// начала и завершения и записью активности от имени actor.
//
// Права и правила переходов здесь не проверяются — это дело вызывающего.
// tx — транзакция вызывающего; nil — метод работает на своём соединении.
// Запись активности идёт на соединении трекера, вне tx.
func (b *Business) SetIssueState(tx *gorm.DB, issueID, stateID uuid.UUID, actor *dao.User) error {
	if actor == nil {
		return errors.New("set issue state: actor is required")
	}
	if tx == nil {
		tx = b.db
	}
	old, err := loadIssueForState(tx, issueID)
	if err != nil {
		return err
	}
	if old.StateId == stateID {
		return nil
	}
	var state dao.State
	if err := tx.Where("id = ? AND project_id = ?", stateID, old.ProjectId).First(&state).Error; err != nil {
		return err
	}
	oldGroup := ""
	if old.State != nil {
		oldGroup = old.State.Group
	}
	updates := stateDates(oldGroup, state.Group, time.Now())
	updates["state_id"] = stateID
	updates["updated_by_id"] = actor.ID
	if err := tx.Model(&dao.Issue{}).Where("id = ?", issueID).Updates(updates).Error; err != nil {
		return err
	}
	updated, err := loadIssueForState(tx, issueID)
	if err != nil {
		return err
	}
	return b.st.TrackChanges(types.LayerIssue, tracker.IssueToSnapshot(*old), tracker.IssueToSnapshot(*updated), updated, actor)
}

// loadIssueForState читает задачу с тем, что нужно снимку для истории.
func loadIssueForState(tx *gorm.DB, issueID uuid.UUID) (*dao.Issue, error) {
	var issue dao.Issue
	err := tx.
		Preload("State").Preload("Project").Preload("Workspace").
		Preload("Assignees").Preload("Watchers").Preload("IssueType").
		Where("issues.id = ?", issueID).First(&issue).Error
	if err != nil {
		return nil, err
	}
	return &issue, nil
}

// stateDates — даты начала и завершения при переходе между группами
// статусов: вход в работу ставит дату начала, возврат в бэклог её снимает,
// завершение и отмена ставят дату завершения, выход из них — снимает.
func stateDates(oldGroup, newGroup string, now time.Time) map[string]any {
	final := func(group string) bool { return group == "completed" || group == "cancelled" }
	updates := map[string]any{}
	switch {
	case final(newGroup):
		if !final(oldGroup) {
			updates["completed_at"] = &types.TargetDateTimeZ{Time: now}
		}
	default:
		updates["completed_at"] = nil
	}
	switch newGroup {
	case "started":
		if oldGroup != "started" {
			updates["start_date"] = &types.TargetDateTimeZ{Time: now}
		}
	case "backlog", "unstarted":
		updates["start_date"] = nil
	}
	return updates
}
