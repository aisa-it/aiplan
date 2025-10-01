package business

import (
	"errors"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

// CreateDocComment создает новый комментарий к документу. Метод принимает задачу, пользователя, текст комментария, комментарий для базы данных, ID для ответа на комментарий и дополнительные метаданные. Возвращает ошибку, если произошла ошибка.
func (b *Business) CreateDocComment(doc dao.Doc, user dao.User, text string, replyToId uuid.NullUUID, fromTg bool, meta ...string) error {
	// Check rights
	var permited bool
	if err := b.db.Select("count(*) > 0").
		Model(&dao.WorkspaceMember{}).
		Where("workspace_id = ?", doc.WorkspaceId).
		Where("member_id = ? and role > ?", user.ID, types.GuestRole).
		Find(&permited).Error; err != nil {
		return err
	}
	if !user.IsSuperuser && !permited {
		return errors.New("create comment forbidden")
	}

	comment := dao.DocComment{
		Id:          dao.GenUUID(),
		WorkspaceId: doc.WorkspaceId,
		DocId:       doc.ID.String(),
		ActorId:     &user.ID,
		CommentHtml: types.RedactorHTML{Body: text},
	}
	if len(meta) > 0 {
		comment.IntegrationMeta = strings.Join(meta, ",")
	}
	if replyToId.Valid {
		comment.ReplyToCommentId = replyToId
	}
	if err := b.db.Create(&comment).Error; err != nil {
		return err
	}
	doc.UpdatedAt = time.Now()
	if err := b.db.Select("updated_at").Updates(&doc).Error; err != nil {
		return err
	}

	data := make(map[string]interface{})
	if fromTg && user.TelegramId != nil {
		data["tg_sender"] = *user.TelegramId
	}

	err := tracker.TrackActivity[dao.DocComment, dao.DocActivity](b.tracker, tracker.ENTITY_CREATE_ACTIVITY, data, nil, comment, &user)
	if err != nil {
		errStack.GetError(nil, err)
	}

	return err
}
