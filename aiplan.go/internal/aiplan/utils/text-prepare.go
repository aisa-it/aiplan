package utils

import (
	"fmt"
	"regexp"
	"strings"

	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/microcosm-cc/bluemonday"
)

func HtmlToTg(text string) string {
	res := replaceTablesToText(text)
	res = replaceImageToText(res)
	res = policy.ProcessCustomHtmlTag(res)
	res = prepareHtmlBody(policy.StripTagsPolicy, res)
	return Substr(ReplaceImgToEmoj(res), 0, 4000)
}

func ReplaceImgToEmoj(body string) string {
	imgRegex := regexp.MustCompile(`image:\s+\(alt:\s*([^)]*)\)`)
	tableRegex := regexp.MustCompile(`table\s*\(size:\s*(\d+)x(\d+)\)`)

	body = imgRegex.ReplaceAllStringFunc(body, func(imgTag string) string {
		matches := imgRegex.FindStringSubmatch(imgTag)
		altText := "image"
		if len(matches) > 1 {
			altText = matches[1]
		}
		return fmt.Sprintf("%s(%s)", "🖼", altText)
	})

	body = tableRegex.ReplaceAllStringFunc(body, func(tableTag string) string {
		matches := tableRegex.FindStringSubmatch(tableTag)
		if len(matches) == 3 {
			rows := matches[1]
			cols := matches[2]
			return fmt.Sprintf("%s(%sx%s)", "📊", rows, cols)
		}
		return tableTag
	})
	return strings.ReplaceAll(body, "&#34;", "\"")
}

func Substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func prepareHtmlBody(stripPolicy *bluemonday.Policy, html string) string {
	res := strings.ReplaceAll(html, "<p>", "\n")
	res = strings.ReplaceAll(res, "<li>", "\n")
	res = stripPolicy.Sanitize(res)
	res = strings.TrimSpace(res)
	return res
}

func replaceImageToText(str string) string {
	imgRegex := regexp.MustCompile(`<img[^>]*alt="([^"]*)"[^>]*>`)

	result := imgRegex.ReplaceAllStringFunc(str, func(imgTag string) string {
		matches := imgRegex.FindStringSubmatch(imgTag)
		altText := "image"
		if len(matches) > 1 {
			altText = matches[1]
		}
		return fmt.Sprintf("%s: (alt: %s)", "image", altText)
	})
	return result
}

func replaceTablesToText(html string) string {
	tableRegex := regexp.MustCompile(`(?s)<table[^>]*>(.*?)</table>`)
	rowRegex := regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	cellRegex := regexp.MustCompile(`(?s)<td[^>]*>|<th[^>]*>`)

	result := tableRegex.ReplaceAllStringFunc(html, func(table string) string {
		rows := rowRegex.FindAllStringSubmatch(table, -1)
		numRows := len(rows)
		numCols := 0

		for _, row := range rows {
			cells := cellRegex.FindAllString(row[1], -1)
			if len(cells) > numCols {
				numCols = len(cells)
			}
		}

		sizeText := fmt.Sprintf("<p>table (size: %dx%d)</p>", numRows, numCols)
		return sizeText
	})

	return result
}
