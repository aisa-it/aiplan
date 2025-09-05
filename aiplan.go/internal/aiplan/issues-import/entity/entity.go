// Содержит структуры данных, представляющие сущности, используемые в системе для работы с Jira и связанными с ней данными.
//
// Основные возможности:
//   - Обработка связанных задач Jira.
//   - Сопоставление типов ссылок Jira.
//   - Представление информации о Jira (проекты, типы ссылок, приоритеты).
//   - Описание вложений Jira и связанных с ними ресурсов.
package entity

import (
	"fmt"
	"net/url"
	"strings"

	"slices"

	"github.com/andygrunwald/go-jira"
	"github.com/gofrs/uuid"
	"sheff.online/aiplan/internal/aiplan/dao"
)

type RawLinkedIssues struct {
	Key1 string
	Key2 string
}

func (l RawLinkedIssues) String() string {
	return fmt.Sprintf("%s:%s", l.Key1, l.Key2)
}

type LinkMapper struct {
	types []string
}

func (lm LinkMapper) Match(id string) bool {
	return slices.Contains(lm.types, strings.TrimSpace(id))
}

func NewLinkMapper(types ...string) LinkMapper {
	return LinkMapper{types}
}

// JiraInfo - общая информация о жире для предоставления вариантов маппинга
type JiraInfo struct {
	Projects   jira.ProjectList     `json:"projects"`
	LinkTypes  []jira.IssueLinkType `json:"link_types"`
	Priorities []jira.Priority      `json:"priorities"`
}

type PrioritiesMapping struct {
	UrgentID string `json:"urgent_id"`
	HighID   string `json:"high_id"`
	MediumID string `json:"medium_id"`
	LowID    string `json:"low_id"`
	NullID   string `json:"null_id"`
}

type Attachment struct {
	JiraKey         string
	DstAssetID      uuid.UUID
	IssueAttachment *dao.IssueAttachment
	InlineAsset     *dao.FileAsset
	JiraAttachment  *jira.Attachment
	FullURL         *url.URL
}

type InlineAsset struct {
	Asset        dao.FileAsset
	AttachmentID string
}
