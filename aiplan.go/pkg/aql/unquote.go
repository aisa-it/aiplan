package aql

import (
	"strconv"
	"strings"
)

var simpleEscapes = map[rune]rune{'t': '\t', 'n': '\n', 'r': '\r'}

// unquote снимает кавычки с QUOTED_STRING и раскрывает экранирование.
func unquote(raw string) string {
	return unescape(raw[1 : len(raw)-1])
}

// \t \n \r и \uXXXX раскрываются, любой другой экранированный символ остаётся как есть: \" → ".
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' || i+1 == len(rs) {
			b.WriteRune(rs[i])
			continue
		}
		i++
		if r, ok := simpleEscapes[rs[i]]; ok {
			b.WriteRune(r)
			continue
		}
		if r, ok := unicodeEscape(rs[i:]); ok {
			b.WriteRune(r)
			i += 4
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func unicodeEscape(rs []rune) (rune, bool) {
	if rs[0] != 'u' || len(rs) < 5 {
		return 0, false
	}
	r, _, _, err := strconv.UnquoteChar(`\u`+string(rs[1:5]), 0)
	return r, err == nil
}
