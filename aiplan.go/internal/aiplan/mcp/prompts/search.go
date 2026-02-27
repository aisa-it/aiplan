package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

var searchPrompts = []Prompt{
	{
		mcp.NewPrompt("search_issues_guide",
			mcp.WithPromptDescription("Пошаговое руководство по поиску задач в AIPlan. Возвращает список доступных пространств и проектов с их ID, а также инструкции по использованию search_issues. Вызови этот промпт перед поиском задач, если не знаешь UUID проектов или slug пространств."),
		),
		searchIssuesGuide,
	},
}

// GetSearchPrompts возвращает список MCP промптов для поиска.
func GetSearchPrompts(db *gorm.DB) []server.ServerPrompt {
	result := make([]server.ServerPrompt, 0, len(searchPrompts))
	for _, p := range searchPrompts {
		result = append(result, server.ServerPrompt{
			Prompt:  p.Prompt,
			Handler: WrapPrompt(db, p.Handler),
		})
	}
	return result
}

// workspaceWithProjects содержит пространство и его проекты для формирования промпта.
type workspaceWithProjects struct {
	Workspace dao.Workspace
	Projects  []dao.Project
}

func searchIssuesGuide(ctx context.Context, db *gorm.DB, user *dao.User, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	// Получаем пространства пользователя
	var workspaces []dao.Workspace
	err := db.
		Where("id IN (?)",
			db.Model(&dao.WorkspaceMember{}).
				Select("workspace_id").
				Where("member_id = ?", user.ID),
		).
		Order("lower(name)").
		Find(&workspaces).Error
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении пространств: %w", err)
	}

	// Для каждого пространства получаем проекты
	data := make([]workspaceWithProjects, 0, len(workspaces))
	for _, ws := range workspaces {
		var projects []dao.Project
		err := db.
			Where("workspace_id = ?", ws.ID).
			Order("lower(name)").
			Find(&projects).Error
		if err != nil {
			continue
		}
		data = append(data, workspaceWithProjects{
			Workspace: ws,
			Projects:  projects,
		})
	}

	content := buildSearchGuideContent(data)

	return &mcp.GetPromptResult{
		Description: "Руководство по поиску задач с актуальным списком пространств и проектов",
		Messages: []mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(content)),
		},
	}, nil
}

func buildSearchGuideContent(data []workspaceWithProjects) string {
	var sb strings.Builder

	sb.WriteString("# Руководство по поиску задач в AIPlan\n\n")
	sb.WriteString("## Пошаговый алгоритм поиска задач\n\n")
	sb.WriteString("### Шаг 1: Определи пространство и проект\n")
	sb.WriteString("Для поиска задач через `search_issues` тебе нужны:\n")
	sb.WriteString("- **workspace_slugs** — slug пространства (строка)\n")
	sb.WriteString("- **project_ids** — UUID проекта (строка формата `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`)\n\n")
	sb.WriteString("Используй данные ниже, чтобы найти нужные идентификаторы.\n\n")

	writeWorkspacesSection(&sb, data)
	writeSearchExample(&sb)
	writeFiltersReference(&sb)
	writeIssueIdentifiers(&sb)

	return sb.String()
}

func writeWorkspacesSection(sb *strings.Builder, data []workspaceWithProjects) {
	sb.WriteString("## Доступные пространства и проекты\n\n")

	if len(data) == 0 {
		sb.WriteString("У тебя нет доступных пространств.\n\n")
		return
	}

	for _, d := range data {
		fmt.Fprintf(sb, "### Пространство: %s\n", d.Workspace.Name)
		fmt.Fprintf(sb, "- **Slug:** `%s`\n", d.Workspace.Slug)
		fmt.Fprintf(sb, "- **ID:** `%s`\n\n", d.Workspace.ID)

		if len(d.Projects) == 0 {
			sb.WriteString("  Нет проектов в этом пространстве.\n\n")
			continue
		}

		sb.WriteString("| Проект | Идентификатор | UUID |\n")
		sb.WriteString("|--------|--------------|------|\n")
		for _, p := range d.Projects {
			fmt.Fprintf(sb, "| %s | %s | `%s` |\n", p.Name, p.Identifier, p.ID)
		}
		sb.WriteString("\n")
	}
}

func writeSearchExample(sb *strings.Builder) {
	sb.WriteString("## Шаг 2: Вызови search_issues\n\n")
	sb.WriteString("Пример вызова для поиска задач в конкретном проекте:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"workspace_slugs\": [\"<slug пространства>\"],\n")
	sb.WriteString("  \"project_ids\": [\"<UUID проекта>\"],\n")
	sb.WriteString("  \"search_query\": \"текст поиска\",\n")
	sb.WriteString("  \"only_active\": true\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")
}

func writeFiltersReference(sb *strings.Builder) {
	sb.WriteString("## Доступные фильтры search_issues\n\n")
	sb.WriteString("| Параметр | Тип | Описание |\n")
	sb.WriteString("|----------|-----|----------|\n")
	sb.WriteString("| search_query | string | Полнотекстовый поиск по названию и описанию |\n")
	sb.WriteString("| workspace_slugs | string[] | Фильтр по slug пространств |\n")
	sb.WriteString("| project_ids | string[] | Фильтр по UUID проектов |\n")
	sb.WriteString("| priorities | string[] | urgent, high, medium, low |\n")
	sb.WriteString("| state_ids | string[] | UUID статусов (получи через get_state_list) |\n")
	sb.WriteString("| assignee_ids | string[] | UUID исполнителей |\n")
	sb.WriteString("| labels | string[] | UUID меток |\n")
	sb.WriteString("| sprint_ids | string[] | UUID спринтов |\n")
	sb.WriteString("| assigned_to_me | bool | Только мои задачи |\n")
	sb.WriteString("| authored_by_me | bool | Только созданные мной |\n")
	sb.WriteString("| only_active | bool | Только активные (не завершённые/отменённые) |\n")
	sb.WriteString("| order_by | string | sequence_id, created_at, updated_at, name, priority, target_date, search_rank |\n")
	sb.WriteString("| desc | bool | Сортировка по убыванию |\n")
	sb.WriteString("| limit | number | Лимит записей (по умолчанию 10, макс. 100) |\n")
	sb.WriteString("| offset | number | Смещение для пагинации |\n\n")
}

func writeIssueIdentifiers(sb *strings.Builder) {
	sb.WriteString("## Идентификаторы задач\n\n")
	sb.WriteString("Задачи можно получить по:\n")
	sb.WriteString("- **UUID:** `595aaa46-f5ec-423d-8272-eb29b602ee08`\n")
	sb.WriteString("- **Sequence ID:** `{workspace_slug}-{project_identifier}-{sequence}` (например: `myspace-PORTAL-42`)\n")
	sb.WriteString("- **Ссылке:** `https://{host}/i/{workspace_slug}/{project_identifier}/{sequence}`\n")
}
