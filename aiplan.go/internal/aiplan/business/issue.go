package business

import (
	"errors"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
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

	data := make(map[string]interface{})
	if fromTg && user.TelegramId != nil {
		data["tg_sender"] = *user.TelegramId
	}

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](b.tracker, activities.EntityCreateActivity, data, nil, comment, &user)
	if err != nil {
		errStack.GetError(nil, err)
	}

	return err
}
