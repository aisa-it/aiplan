// Package policy реализует HTML-санитайзер на базе bluemonday для защиты от XSS-атак.
//
// Пакет настраивает whitelist разрешённых HTML-тегов, атрибутов и CSS-стилей
// для контента из TipTap редактора. Всё, что не в whitelist, удаляется.
//
// Предоставляет две политики:
//   - StripTagsPolicy — удаляет все HTML-теги, оставляя только текст
//   - UgcPolicy — разрешает безопасное подмножество HTML для пользовательского контента
//
// Функция ProcessCustomHtmlTag преобразует кастомные span-элементы TipTap
// (ссылки на задачи) в текстовое представление для экспорта.
package policy

import (
	"container/list"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/microcosm-cc/bluemonday"
)

var StripTagsPolicy *bluemonday.Policy = bluemonday.StrictPolicy()
var UgcPolicy *bluemonday.Policy = bluemonday.UGCPolicy()

func init() {
	dataSpanAttrs := []string{
		"style",
		"class",
		"data-id",
		"data-type",
		"data-date",
		"data-slug",
		"data-title",
		"data-label",
		"data-doc-id",
		"contenteditable",
		"data-comment-id",
		// data-anchor-id намеренно НЕ здесь: он разрешается ниже отдельным
		// правилом с проверкой формата слага. Правила атрибутов складываются
		// по ИЛИ, поэтому попадание в этот список сделало бы проверку
		// бесполезной — любое значение проходило бы по правилу без Matching.
		"data-anchor-title",
		"data-original-url",
		"data-current-issue-id",
		"data-project-identifier",
	}

	dataDivAttrs := []string{
		"data-spoiler",
		"data-title",
		"data-collapsed",
		"data-info-block",
		"data-icon-color",
	}

	colorRegexp := regexp.MustCompile(`^(#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})|rgb\((\d+),\s*(\d+),\s*(\d+)\)|inherit)$`)
	sizeRegexp := regexp.MustCompile(`^(\d+(px|em|rem|ex|rex|pt|in|pc|mm|cm|Q|vh|vw|vmax|vmin|vb|vi)?|auto|max-content|min-content|fit-content|inherit|initial|revert(-layer)?|unset)$`)
	fontRegexp := regexp.MustCompile(`^(serif|sans-serif|monospace|cursive|fantasy|system-ui|ui-serif|ui-sans-serif|ui-monospace|ui-rounded|emoji|math|fangsong|inherit|initial|revert|revert-layer|unset)$`)
	alignRegexp := regexp.MustCompile(`^(right|left)$`)
	displayRegexp := regexp.MustCompile(`^(block|inline-block)$`)
	classRegexp := regexp.MustCompile(`^(date-node|special-link-mention|doc-anchor)$`)
	indentClassRegexp := regexp.MustCompile(`^tt-indent-[1-9]$`)
	dataIconRegexp := regexp.MustCompile(`^(alertIcon|closeIconBorder|checkStatusIcon|infoIcon)$`)
	colorNamesRegexp := regexp.MustCompile(`^(transparent|blue|cyan|green|red|orange|yellow|magenta)$`)
	dataLinksRegexp := regexp.MustCompile(`^\[\s*\{[^{}]*}\s*(,\s*\{[^{}]*}\s*)*]$`)
	// Слаг якоря документа АИДока: ASCII, первый символ буквенно-цифровой, до 64 символов.
	// Тот же формат, что фронт кладёт в data-anchor-id и в id заголовка.
	anchorIdRegexp := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

	// Правило по факту декларативное: ниже "class" разрешается на span без
	// Matching в составе dataSpanAttrs, а политики атрибутов складываются по ИЛИ,
	// так что на span проходит любой класс. Список ведём как описание контракта фронта.
	UgcPolicy.AllowAttrs("class").Matching(classRegexp).OnElements("span")
	UgcPolicy.AllowAttrs("data-issue-table-params", "style", "class").OnElements("table")
	UgcPolicy.AllowAttrs("contenteditable").OnElements("td", "th")

	UgcPolicy.AllowAttrs("spellcheck", "class").OnElements("pre")
	UgcPolicy.AllowAttrs("data-color", "style").OnElements("mark")
	UgcPolicy.AllowAttrs(dataSpanAttrs...).OnElements("span")
	// В отличие от id (см. ниже), это правило действительно ограничивает формат:
	// другого правила для data-anchor-id в политике нет.
	//
	// Заголовки в списке не случайно: якорь заголовка живёт в data-anchor-id,
	// а НЕ в id. Атрибут id на заголовках занят расширением оглавления
	// (@tiptap/extension-table-of-contents пишет туда свой UUID), поэтому
	// фронт хранит слаг отдельным атрибутом. См. headingAnchor.ts.
	UgcPolicy.AllowAttrs("data-anchor-id").Matching(anchorIdRegexp).
		OnElements("span", "h1", "h2", "h3", "h4", "h5", "h6")
	UgcPolicy.AllowAttrs(dataDivAttrs...).OnElements("div")
	UgcPolicy.AllowAttrs("class").Matching(indentClassRegexp).OnElements("p")

	UgcPolicy.AllowAttrs("data-icon").Matching(dataIconRegexp).Globally()
	UgcPolicy.AllowAttrs("data-bgcolor").Matching(colorNamesRegexp).Globally()
	UgcPolicy.AllowAttrs("colwidth").Globally()

	UgcPolicy.AllowStyles("color", "background-color").Matching(colorRegexp).Globally()
	UgcPolicy.AllowStyles("width", "height", "font-size").Matching(sizeRegexp).Globally()
	UgcPolicy.AllowStyles("text-align").Matching(bluemonday.CellAlign).Globally()
	UgcPolicy.AllowStyles("font-family").Matching(fontRegexp).Globally()

	UgcPolicy.AllowStyles("margin").Matching(sizeRegexp).OnElements("img")
	UgcPolicy.AllowStyles("float").Matching(alignRegexp).OnElements("img")
	UgcPolicy.AllowStyles("display").Matching(displayRegexp).OnElements("img")
	UgcPolicy.AllowStyles("drawio").OnElements("img")
	UgcPolicy.AllowAttrs("data-drawio").OnElements("img")

	UgcPolicy.AllowAttrs("data-type").Matching(regexp.MustCompile("^taskList$")).OnElements("ul")

	UgcPolicy.AllowAttrs("data-checked").Matching(regexp.MustCompile("^(true|false)$")).OnElements("li")
	UgcPolicy.AllowAttrs("data-type").Matching(regexp.MustCompile("^taskItem$")).OnElements("li")
	UgcPolicy.AllowAttrs("start").Matching(regexp.MustCompile(`^\d+$`)).OnElements("ol")

	UgcPolicy.AllowAttrs("data-heading-links").OnElements("div")
	UgcPolicy.AllowAttrs("data-links").Matching(dataLinksRegexp).OnElements("div")

	// Якоря документов АИДока: id на заголовках и на span.doc-anchor.
	//
	// ВНИМАНИЕ: правило НЕ ужесточает существующее. bluemonday складывает
	// политики атрибутов по ИЛИ: сначала проверяются правила элемента
	// (elsAndAttrs), затем глобальные (globalAttrs), и достаточно совпадения
	// с любым (sanitize.go:512 и :527). Порядок объявления значения не имеет.
	// UGCPolicy через AllowStandardAttributes уже разрешает id глобально по
	// НЕЯКОРНОМУ `[a-zA-Z0-9\:\-_\.]+` (bluemonday использует MatchString,
	// а не полное совпадение), поэтому мусорные значения вроде "foo bar"
	// или "javascript:alert(1)" всё равно проходят по глобальному правилу.
	// Правило ниже фиксирует ожидаемый формат якоря; ужесточение требует
	// переопределения глобальной политики id и делается отдельно.
	UgcPolicy.AllowAttrs("id").Matching(anchorIdRegexp).OnElements("span", "h1", "h2", "h3", "h4", "h5", "h6")
}

func ProcessCustomHtmlTag(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	queue := list.New()
	queue.PushBack(doc)

	for queue.Len() > 0 {
		element := queue.Front()
		queue.Remove(element)
		node := element.Value.(*html.Node)

		var next *html.Node

		for child := node.FirstChild; child != nil; child = next {
			next = child.NextSibling
			if child.Type == html.ElementNode && child.Data == "span" && isIssueLinkMention(child) {
				processIssueLinkNode(child)
			} else {
				if child.FirstChild != nil {
					queue.PushBack(child)
				}
			}
		}
	}

	var result strings.Builder
	html.Render(&result, doc)

	return result.String()
}

func isIssueLinkMention(node *html.Node) bool {
	for _, attr := range node.Attr {
		if attr.Key == "data-type" && attr.Val == "issueLinkMention" {
			return true
		}
	}
	return false
}

func processIssueLinkNode(node *html.Node) {
	var slug, projectID, issueID string

	for _, attr := range node.Attr {
		switch attr.Key {
		case "data-slug":
			slug = attr.Val
		case "data-project-identifier":
			projectID = attr.Val
		case "data-current-issue-id":
			issueID = attr.Val
		}
	}

	if projectID != "" && issueID != "" {
		replacementText := fmt.Sprintf("<ссылка на задачу %s/%s-%s>", slug, projectID, issueID)
		textNode := &html.Node{
			Type: html.TextNode,
			Data: replacementText,
		}

		node.Parent.InsertBefore(textNode, node)
		node.Parent.RemoveChild(node)
	}
}
