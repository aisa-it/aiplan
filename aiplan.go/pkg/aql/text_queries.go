package aql

import (
	"regexp"
	"unicode/utf8"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// textCmp — ~ ищет слова полнотекстовым поиском, !~ — задачи, где их нет.
func textCmp(match func(v *Visitor, text string) *gorm.DB) cmpFunc {
	return func(v *Visitor, op int, value any) (*gorm.DB, error) {
		if op != parser.AQLParserLIKE && op != parser.AQLParserNOT_LIKE {
			return nil, ErrTextOperator
		}
		text, err := lowerStrings([]any{value})
		if err != nil {
			return nil, err
		}
		if utf8.RuneCountInString(text[0]) < minLikeLen {
			return nil, ErrLikeLength
		}
		cond := match(v, text[0])
		if op == parser.AQLParserNOT_LIKE {
			return v.newQuery().Not(cond), nil
		}
		return cond, nil
	}
}

// websearchSyntax — фраза в кавычках или -слово.
var websearchSyntax = regexp.MustCompile(`"|(^|\s)-\pL`)

// tokensMatch: в запросе с фразой или исключением префиксы не ищутся — префиксный запрос требует
// все слова в любом порядке и отменил бы и фразу, и исключение.
func tokensMatch(column, text string) clause.Expr {
	prefix := ""
	if !websearchSyntax.MatchString(text) {
		prefix = dao.AllWordsTSQuery(text, minLikeLen)
	}
	return dao.TokensMatch(column, text, prefix)
}

// titleMatch: issues.tokens собран из названия и описания и покрыт индексом, по нему отбор;
// вектор одного названия считается только для отобранных задач.
func titleMatch(v *Visitor, text string) *gorm.DB {
	return v.newQuery().Where(tokensMatch("issues.tokens", text)).
		Where(tokensMatch("to_tsvector_multilang(issues.name, '')", text))
}

// descriptionMatch — отбор как в titleMatch.
func descriptionMatch(v *Visitor, text string) *gorm.DB {
	return v.newQuery().Where(tokensMatch("issues.tokens", text)).
		Where(tokensMatch("to_tsvector_multilang('', issues.description_stripped)", text))
}

// commentMatch — комментарии видны вместе с задачей: задачи ограничивает scope внешнего запроса.
func commentMatch(v *Visitor, text string) *gorm.DB {
	rows := v.newQuery().Model(&dao.IssueComment{}).Select("1").
		Where("issue_comments.issue_id = issues.id").Where(tokensMatch("issue_comments.tokens", text))
	return v.newQuery().Where("EXISTS (?)", rows)
}

// textMatch — название, описание или комментарии.
func textMatch(v *Visitor, text string) *gorm.DB {
	return v.newQuery().Where(tokensMatch("issues.tokens", text)).Or(commentMatch(v, text))
}
