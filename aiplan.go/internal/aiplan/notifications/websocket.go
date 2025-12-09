// Управление вебсокетными уведомлениями для пользователей.
// Отправка уведомлений, поддержание активных сессий и автоматическое закрытие неактивных.
//
// Основные возможности:
//   - Поддержка множественных активных вебсокетных сессий для каждого пользователя.
//   - Отправка уведомлений через вебсокеты с использованием JSON.
//   - Автоматическое закрытие неактивных вебсокетных сессий для экономии ресурсов.
//   - Пинг для поддержания активных соединений.
package notifications

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gofrs/uuid"
)

const (
	pingPeriod = time.Second * 20
	timeout    = time.Minute
)

type WebsocketMsg struct {
	NotificationResponse
	CountNotify int `json:"count_notify"`
}

type Mention struct {
	dao.IssueComment
}

type WebsocketNotificationService struct {
	sessions map[string]map[uuid.UUID]*websocket.Conn
	mutex    sync.RWMutex
}

func NewWebsocketNotificationService() *WebsocketNotificationService {
	return &WebsocketNotificationService{
		sessions: make(map[string]map[uuid.UUID]*websocket.Conn),
	}
}

func (wns *WebsocketNotificationService) Handle(userId string, w http.ResponseWriter, req *http.Request) {
	c, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		// TODO remove pattern "*"
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		slog.Error("Open websocket connection", "err", err)
		return
	}
	defer c.CloseNow()

	conId := uuid.Must(uuid.NewV4())

	wns.mutex.Lock()
	cons, ok := wns.sessions[userId]
	if !ok {
		cons = make(map[uuid.UUID]*websocket.Conn)
	}
	cons[conId] = c
	wns.sessions[userId] = cons
	wns.mutex.Unlock()

	go wns.pingLoop(userId, conId, c)

	// Start read until close
	ctx := context.Background()
	ctx = c.CloseRead(ctx)
	<-ctx.Done()

	wns.mutex.Lock()
	delete(wns.sessions[userId], conId)
	if len(wns.sessions[userId]) == 0 {
		delete(wns.sessions, userId)
	}
	wns.mutex.Unlock()

	c.Close(websocket.StatusNormalClosure, "")
}

func (wns *WebsocketNotificationService) CloseUserSessions(userId string) {
	wns.mutex.Lock()
	defer wns.mutex.Unlock()
	cons, ok := wns.sessions[userId]
	if !ok {
		return
	}
	for _, con := range cons {
		con.Close(websocket.StatusNormalClosure, "user reset all sessions")
	}
}

func (wns *WebsocketNotificationService) Send(userId, notifyId string, data interface{}, countNotify int) error {
	msg := WebsocketMsg{}
	msg.Id = notifyId
	msg.CountNotify = countNotify
	msg.CreatedAt = time.Now().UTC()
	switch v := data.(type) {
	case dao.IssueActivity:
		if v.Verb == "deleted" && *v.Field != actField.Linked.Field.String() {
			return nil
		}
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Issue:     v.Issue.ToLightDTO(),
			Project:   v.Issue.Project.ToLightDTO(),
			Workspace: v.Issue.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()
	case dao.DocActivity:
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Doc:       v.Doc.ToLightDTO(),
			Workspace: v.Doc.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()
	case dao.ProjectActivity:
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Project:   v.Project.ToLightDTO(),
			Workspace: v.Project.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()
	case dao.WorkspaceActivity:
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Workspace: v.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()
	case dao.EntityActivity:
		if v.EntityType == "issue" && v.Verb == "deleted" && *v.Field != actField.Linked.Field.String() {
			return nil
		}
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Issue:     v.Issue.ToLightDTO(),
			Project:   v.Issue.Project.ToLightDTO(),
			Workspace: v.Issue.Workspace.ToLightDTO(),
		}

		tmp := v.ToLightDTO()
		//tmp.NewEntity = v.AffectedUser.ToLightDTO()
		msg.Data = tmp
	case dao.FullActivity:
		if v.EntityType == "issue" && v.Verb == "deleted" && *v.Field != actField.Linked.Field.String() {
			return nil
		}
		msg.Type = "activity"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Issue:     v.Issue.ToLightDTO(),
			Project:   v.Issue.Project.ToLightDTO(),
			Workspace: v.Issue.Workspace.ToLightDTO(),
			Doc:       v.Doc.ToLightDTO(),
			Form:      v.Form.ToLightDTO(),
		}

	case dao.IssueComment:
		msg.Type = "comment"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Issue:     v.Issue.ToLightDTO(),
			Project:   v.Project.ToLightDTO(),
			Workspace: v.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()

	case Mention:
		msg.Type = "mention"
		msg.Detail = NotificationDetailResponse{
			User:      v.Actor.ToLightDTO(),
			Issue:     v.Issue.ToLightDTO(),
			Project:   v.Project.ToLightDTO(),
			Workspace: v.Workspace.ToLightDTO(),
		}
		msg.Data = v.ToLightDTO()

	case dao.UserNotifications:
		msg.Type = v.Type
		var project *dto.ProjectLight

		if v.Issue != nil {
			project = v.Issue.Project.ToLightDTO()
		}
		msg.Detail = NotificationDetailResponse{
			User:         v.Author.ToLightDTO(),
			IssueComment: v.Comment.ToLightDTO(),
			Issue:        v.Issue.ToLightDTO(),
			Project:      project,
			Workspace:    v.Workspace.ToLightDTO(),
		}

		msg.Data = NotificationResponseMessage{
			Title: v.Title,
			Msg:   v.Msg,
		}

	default:
		return errors.New("unsupported notification data type")
	}

	wns.mutex.RLock()
	cons, ok := wns.sessions[userId]
	wns.mutex.RUnlock()
	if !ok {
		return nil
	}
	for _, session := range cons {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		if err := wsjson.Write(ctx, session, msg); err != nil {
			slog.Error("Write notification to websocket", "userId", userId, "err", err)
		}
		cancel()
	}
	return nil
}

func (wns *WebsocketNotificationService) pingLoop(userId string, sessionId uuid.UUID, conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := conn.Ping(ctx)
		cancel()
		if err != nil {
			slog.Debug("Ping to websocket failed", "userId", userId, "err", err)
			wns.mutex.Lock()
			delete(wns.sessions[userId], sessionId)
			if len(wns.sessions[userId]) == 0 {
				delete(wns.sessions, userId)
			}
			wns.mutex.Unlock()
			conn.Close(websocket.StatusNormalClosure, "Ping failed, connection closed")
			return
		}
	}
}
