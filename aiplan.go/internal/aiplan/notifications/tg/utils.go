package tg

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-telegram/bot"
	"golang.org/x/net/html"
)

const (
	targetDateTimeZ = "TargetDateTimeZ"
	userMentioned   = "UserMentioned"
)

func Stelegramf(format string, a ...any) string {
	for i, v := range a {
		switch tv := v.(type) {
		case string:
			a[i] = bot.EscapeMarkdown(tv)
		}
	}

	return fmt.Sprintf(format, a...)
}

func getUserTg(user *dao.User, roles ...role) (userTg, bool) {
	if user.TelegramId == nil {
		return userTg{}, false
	}
	if !user.CanReceiveNotifications() {
		return userTg{}, false
	}
	if user.Settings.TgNotificationMute {
		return userTg{}, false
	}
	return userTg{id: *user.TelegramId, loc: user.UserTimezone, roles: combineRoles(roles...)}, true
}

func strReplace(in string) string {
	out := strings.Split(in, "_")
	return "$$$" + strings.Join(out, "$$$") + "$$$"
}

func msgReplace(user userTg, msg TgMsg) TgMsg {
	for k, v := range msg.replace {
		key := k
		keys := strings.Split(k, "_")
		if len(keys) > 1 {
			key = keys[0]
		}
		switch key {
		case targetDateTimeZ:
			if strNeW, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), "02.01.2006 15:04", &user.loc); err == nil {
				msg.body = strings.ReplaceAll(msg.body, strReplace(k), Stelegramf("%s", strNeW))
			} else {
				return NewTgMsg()
			}
		case userMentioned:
			if user.Has(commentMentioned) {
				msg.title += Stelegramf("\n__%s__", "Вас упомянули в комментарии")
			}
		}
	}
	return msg
}

func getUserName(user *dao.User) string {
	if user == nil {
		return "Новый пользователь"
	}
	if user.LastName == "" {
		return fmt.Sprintf("%s", user.Email)
	}
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func getExistUser(user ...*dao.User) *dao.User {
	for _, u := range user {
		if u != nil {
			return u
		}
	}
	return nil
}

func escapeCharacters(data string) string {
	data = html.UnescapeString(data)
	res := strings.ReplaceAll(data, "\\", "")
	res = strings.ReplaceAll(res, "_", "\\_")
	res = strings.ReplaceAll(res, "*", "\\*")
	res = strings.ReplaceAll(res, "[", "\\[")
	res = strings.ReplaceAll(res, "]", "\\]")
	res = strings.ReplaceAll(res, "(", "\\(")
	res = strings.ReplaceAll(res, ")", "\\)")
	res = strings.ReplaceAll(res, "~", "\\~")
	res = strings.ReplaceAll(res, "`", "\\`")
	res = strings.ReplaceAll(res, ">", "\\>")
	res = strings.ReplaceAll(res, "#", "\\#")
	res = strings.ReplaceAll(res, "+", "\\+")
	res = strings.ReplaceAll(res, "-", "\\-")
	res = strings.ReplaceAll(res, "=", "\\=")
	res = strings.ReplaceAll(res, "|", "\\|")
	res = strings.ReplaceAll(res, "{", "\\{")
	res = strings.ReplaceAll(res, "}", "\\}")
	res = strings.ReplaceAll(res, ".", "\\.")
	res = strings.ReplaceAll(res, "!", "\\!")
	res = strings.ReplaceAll(res, "&lt;", "\\<")
	res = strings.ReplaceAll(res, "&gt;", "\\>")
	res = strings.ReplaceAll(res, "&amp;", "\\&")
	res = strings.ReplaceAll(res, "&#39;", "'")
	return res
}
