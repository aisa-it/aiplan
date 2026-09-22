// Вспомогательные функции для работы с Jira API.
//
// Основные возможности:
//   - Разбор ключей Jira-задач.
//   - Получение типов ссылок Jira.
//   - Преобразование URL Jira-задачи в формат для удобного использования.
package utils

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/andygrunwald/go-jira"
)

func ParseKey(i jira.Issue) (string, string) {
	return ParseRawKey(i.Key)
}

func ParseRawKey(key string) (string, string) {
	arr := strings.Split(strings.TrimSpace(key), "-")
	if len(arr) < 2 {
		return "", ""
	}
	return arr[0], arr[1]
}

func GetNotNil[T any](a *T, b *T) *T {
	if a != nil {
		return a
	} else if b != nil {
		return b
	}
	return nil
}

func GetLinkTypes(client *jira.Client) ([]jira.IssueLinkType, error) {
	var linkTypes struct {
		Types []jira.IssueLinkType `json:"issueLinkTypes"`
	}
	req, _ := client.NewRequest("GET", "rest/api/2/issueLinkType", nil)
	_, err := client.Do(req, &linkTypes)
	if err != nil {
		return nil, err
	}

	return linkTypes.Types, nil
}

func GetJiraIssueURL(issue *jira.Issue) *url.URL {
	u, err := url.Parse(issue.Self)
	if err != nil {
		slog.Error("Parse jira self url", "url", issue.Self)
		return nil
	}

	u.Path = ""
	u.RawFragment = ""
	u.RawQuery = ""

	return u.JoinPath("browse", issue.Key)
}
