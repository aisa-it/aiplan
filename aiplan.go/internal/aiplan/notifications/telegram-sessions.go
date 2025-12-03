// Пакет для управления сессиями и уведомлениями в Telegram боте.
// Содержит логику начала, продолжения и завершения сессий, а также обработку пользовательских команд.
// Поддерживает создание задач и отслеживание активности в системе.
//
// Основные возможности:
//   - Управление сессиями пользователей.
//   - Обработка команд пользователя для создания задач.
//   - Отслеживание активности в системе (создание задач).
//   - Интеграция с базой данных и Telegram ботом.
package notifications

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

type SessionHandler struct {
	sessions map[int64]*Flow
	m        sync.RWMutex
	db       *gorm.DB
	bot      *tgbotapi.BotAPI
}

type Flow struct {
	steps    []Step
	position int

	resultFunc func(dao.User, []interface{})

	Answers []interface{}
}

type Step struct {
	Question    tgbotapi.MessageConfig
	answer      interface{}
	convertFunc func(tgbotapi.Update) (interface{}, error)
}

func NewSessionHandler(db *gorm.DB, bot *tgbotapi.BotAPI) *SessionHandler {
	sh := SessionHandler{db: db, bot: bot}
	sh.sessions = make(map[int64]*Flow)
	return &sh
}

func (sh *SessionHandler) StartSession(user dao.User, flow Flow) (*Step, bool) {
	sh.m.Lock()
	defer sh.m.Unlock()
	sh.sessions[*user.TelegramId] = &flow
	return flow.Next(user)
}

func (sh *SessionHandler) IsBusy(user dao.User) bool {
	sh.m.RLock()
	defer sh.m.RUnlock()
	_, ok := sh.sessions[*user.TelegramId]
	return ok
}

func (sh *SessionHandler) Next(user dao.User) (*Step, bool) {
	if !sh.IsBusy(user) {
		return nil, true
	}
	sh.m.RLock()
	defer sh.m.RUnlock()
	flow := sh.sessions[*user.TelegramId]
	return flow.Next(user)
}

func (sh *SessionHandler) Answer(user dao.User, update tgbotapi.Update) error {
	if !sh.IsBusy(user) {
		return nil
	}
	sh.m.RLock()
	defer sh.m.RUnlock()
	flow := sh.sessions[*user.TelegramId]
	return flow.Answer(update)
}

func (sh *SessionHandler) Answers(user dao.User) []interface{} {
	sh.m.RLock()
	defer sh.m.RUnlock()
	return sh.sessions[*user.TelegramId].Answers
}

func (sh *SessionHandler) Abort(user dao.User) {
	sh.m.Lock()
	defer sh.m.Unlock()
	delete(sh.sessions, *user.TelegramId)
}

func (flow *Flow) Next(user dao.User) (*Step, bool) {
	if flow.position > len(flow.steps)-1 {
		for _, step := range flow.steps {
			flow.Answers = append(flow.Answers, step.answer)
		}
		flow.resultFunc(user, flow.Answers)
		return nil, true
	}
	step := flow.steps[flow.position]
	return &step, false
}

func (flow *Flow) Answer(update tgbotapi.Update) error {
	if err := flow.steps[flow.position].Answer(update); err != nil {
		return err
	}
	flow.position = flow.position + 1
	return nil
}

func (step *Step) Answer(update tgbotapi.Update) error {
	res, err := step.convertFunc(update)
	if err != nil {
		return err
	}
	step.answer = res
	return nil
}

func (sh *SessionHandler) StartIssueCreationFlow(user dao.User, createFunc func(dao.User, []interface{})) (*Step, bool) {
	var projects []dao.Project
	if err := sh.db.Preload("Workspace").
		Where("id in (?)", sh.db.Select("project_id").Where("member_id = ?", user.ID).Where("role > 5").Model(&dao.ProjectMember{})).
		Find(&projects).Error; err != nil {
		return nil, true
	}

	var keys [][]tgbotapi.InlineKeyboardButton
	for _, project := range projects {
		keys = append(keys, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s/%s", project.Workspace.Slug, project.Identifier),
				project.ID,
			),
		))
	}
	chooseProjectMsg := tgbotapi.NewMessage(*user.TelegramId, "Создание задачи")
	chooseProjectMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keys...)
	chooseProjectMsg.ParseMode = "markdown"

	flow := Flow{steps: []Step{
		{
			Question:    chooseProjectMsg,
			convertFunc: sh.convertProjectCallback,
		},
		{
			Question:    tgbotapi.NewMessage(*user.TelegramId, "Пришлите название создаваемой задачи"),
			convertFunc: convertString,
		},
		{
			Question:    tgbotapi.NewMessage(*user.TelegramId, "Пришлите описание создаваемой задачи или `-` для пустого описания"),
			convertFunc: convertStringWithEmpty,
		},
	}, resultFunc: createFunc}
	return sh.StartSession(user, flow)
}

func (sh *SessionHandler) convertProjectCallback(update tgbotapi.Update) (interface{}, error) {
	if update.CallbackQuery == nil {
		return nil, errors.New("нажмите на кнопку выбора")
	}
	projectId := update.CallbackData()

	var project dao.Project
	if err := sh.db.Preload("Workspace").
		Where("id = ?", projectId).
		Find(&project).Error; err != nil {
		slog.Error("Get project from telegram callback", "project_id", projectId, "err", err)
		return nil, errors.New("внутренняя ошибка")
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(
		update.CallbackQuery.Message.Chat.ID,
		update.CallbackQuery.Message.MessageID,
		update.CallbackQuery.Message.Text+fmt.Sprintf("\nПроект: *%s/%s*", project.Workspace.Slug, project.Identifier),
		tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: make([][]tgbotapi.InlineKeyboardButton, 0),
		})
	edit.ParseMode = "markdown"
	sh.bot.Send(edit)

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
	callback.ShowAlert = true
	callback.Text = fmt.Sprintf("Выбран проект: %s/%s", project.Workspace.Slug, project.Identifier)
	sh.bot.Request(callback)

	return project, nil
}

func convertString(update tgbotapi.Update) (interface{}, error) {
	if update.Message == nil {
		return nil, errors.New("пришлите текст")
	}
	return update.Message.Text, nil
}

func convertStringWithEmpty(update tgbotapi.Update) (interface{}, error) {
	if update.Message == nil {
		return nil, errors.New("пришлите текст")
	}
	if update.Message.Text == "-" {
		return "", nil
	}
	return update.Message.Text, nil
}

func (ts *TelegramService) createIssue(user dao.User, args []interface{}) {
	project, pOk := args[0].(dao.Project)
	title, tOk := args[1].(string)
	description, dOk := args[2].(string)

	if !pOk || !tOk || !dOk {
		slog.Error("CreateIssue session wrong args", "args", args)
		return
	}

	issue := dao.Issue{
		ID:              dao.GenUUID(),
		WorkspaceId:     project.WorkspaceId,
		ProjectId:       project.ID,
		CreatedAt:       time.Now(),
		CreatedById:     user.ID,
		UpdatedById:     &user.ID,
		Name:            title,
		DescriptionHtml: description,
	}

	if err := dao.CreateIssue(ts.db, &issue); err != nil {
		slog.Error("Create issue from telegram", "err", err)
		return
	}
	err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](ts.tracker, activities.EntityCreateActivity, nil, nil, issue, &user)
	if err != nil {
		slog.Error("Track new issue from telegram activity", "err", err)
		return
	}

}
