package business

import (
	"errors"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/dao"
	errStack "sheff.online/aiplan/internal/aiplan/stack-error"
	"sheff.online/aiplan/internal/aiplan/types"
	"strings"
	"time"
)

// CreateIssueComment создает новый комментарий к задаче. Метод принимает задачу, пользователя, текст комментария, комментарий для базы данных, ID для ответа на комментарий и дополнительные метаданные. Возвращает ошибку, если произошла ошибка.
func (b *Business) CreateIssueComment(issue dao.Issue, user dao.User, text string, replyToId *string, fromTg bool, meta ...string) error {
	// Check rights
	var permited bool
	if err := b.db.Select("count(*) > 0").
		Model(&dao.ProjectMember{}).
		Where("project_id = ?", issue.ProjectId).
		Where("member_id = ? and role > ?", user.ID, types.GuestRole).
		Find(&permited).Error; err != nil {
		return err
	}
	if !user.IsSuperuser && !permited {
		return errors.New("create comment forbidden")
	}

	issueId := issue.ID.String()
	comment := dao.IssueComment{
		Id:          dao.GenID(),
		WorkspaceId: issue.WorkspaceId,
		ProjectId:   issue.ProjectId,
		IssueId:     issueId,
		ActorId:     &user.ID,
		CommentHtml: types.RedactorHTML{Body: text},
	}
	if len(meta) > 0 {
		comment.IntegrationMeta = strings.Join(meta, ",")
	}
	if replyToId != nil {
		comment.ReplyToCommentId = replyToId
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

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](b.tracker, tracker.ENTITY_CREATE_ACTIVITY, data, nil, comment, &user)
	if err != nil {
		errStack.GetError(nil, err)
	}

	return err
}
