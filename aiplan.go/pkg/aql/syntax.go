package aql

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/antlr4-go/antlr/v4"
)

var tokenNames = map[int]string{
	antlr.TokenEOF:                "конец запроса",
	parser.AQLParserAND:           "AND",
	parser.AQLParserOR:            "OR",
	parser.AQLParserNOT:           "NOT",
	parser.AQLParserIN:            "IN",
	parser.AQLParserIS:            "IS",
	parser.AQLParserEMPTY:         "EMPTY",
	parser.AQLParserORDER:         "ORDER BY",
	parser.AQLParserBY:            "BY",
	parser.AQLParserASC:           "ASC",
	parser.AQLParserDESC:          "DESC",
	parser.AQLParserWAS:           "WAS",
	parser.AQLParserCHANGED:       "CHANGED",
	parser.AQLParserAFTER:         "AFTER",
	parser.AQLParserBEFORE:        "BEFORE",
	parser.AQLParserON:            "ON",
	parser.AQLParserDURING:        "DURING",
	parser.AQLParserFROM:          "FROM",
	parser.AQLParserTO:            "TO",
	parser.AQLParserEQ:            "«=»",
	parser.AQLParserNEQ:           "«!=»",
	parser.AQLParserLIKE:          "«~»",
	parser.AQLParserNOT_LIKE:      "«!~»",
	parser.AQLParserLT:            "«<»",
	parser.AQLParserLTE:           "«<=»",
	parser.AQLParserGT:            "«>»",
	parser.AQLParserGTE:           "«>=»",
	parser.AQLParserLPAREN:        "«(»",
	parser.AQLParserRPAREN:        "«)»",
	parser.AQLParserLBRACKET:      "«[»",
	parser.AQLParserRBRACKET:      "«]»",
	parser.AQLParserCOMMA:         "«,»",
	parser.AQLParserNUMBER:        "значение",
	parser.AQLParserDURATION:      "значение",
	parser.AQLParserQUOTED_STRING: "значение",
	parser.AQLParserSTRING:        "значение",
}

func parseQuery(query string) (parser.IQueryContext, error) {
	lexer := parser.NewAQLLexer(antlr.NewInputStream(query))
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewAQLParser(tokens)

	listener := &errorListener{DefaultErrorListener: antlr.NewDefaultErrorListener(), tokens: tokens}
	lexer.RemoveErrorListeners()
	p.RemoveErrorListeners()
	p.AddErrorListener(listener)

	tree := p.Query()
	if listener.err != nil {
		return nil, listener.err
	}
	return tree, nil
}

type errorListener struct {
	*antlr.DefaultErrorListener
	tokens *antlr.CommonTokenStream
	err    *Error
}

func (l *errorListener) SyntaxError(r antlr.Recognizer, offending any, _, _ int, msg string, e antlr.RecognitionException) {
	if l.err != nil {
		return
	}
	tok := offending.(antlr.Token)
	l.err = &Error{
		Code:  "syntax",
		Err:   msg,
		Ru:    l.message(r.(antlr.Parser), tok, e),
		Start: tok.GetStart(),
		End:   tok.GetStop() + 1,
	}
}

func (l *errorListener) message(p antlr.Parser, tok antlr.Token, e antlr.RecognitionException) string {
	switch tok.GetTokenType() {
	case parser.AQLParserUNTERMINATED_STRING:
		return "Не закрыта кавычка"
	case parser.AQLParserERROR_CHAR:
		return fmt.Sprintf("Недопустимый символ «%s»", tok.GetText())
	}

	var expected []string
	for _, interval := range p.GetExpectedTokens().GetIntervals() {
		for t := interval.Start; t < interval.Stop; t++ {
			expected = append(expected, tokenNames[t])
		}
	}
	// Так ANTLR ошибается только в условии после поля, но ожидаемыми отдаёт токены начала условия.
	if _, ok := e.(*antlr.NoViableAltException); ok {
		expected = []string{"оператор"}
	}

	where := "В начале запроса"
	if i := tok.GetTokenIndex(); i > 0 {
		where = fmt.Sprintf("После «%s»", l.tokens.Get(i-1).GetText())
	}
	found := fmt.Sprintf("«%s»", tok.GetText())
	if tok.GetTokenType() == antlr.TokenEOF {
		found = "конец запроса"
	}
	return fmt.Sprintf("%s ожидается %s, а не %s", where, strings.Join(slices.Compact(expected), ", "), found)
}
