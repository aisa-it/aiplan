// Пакет jql — текстовый язык запросов задач, аналог JQL (прагматичное подмножество).
//
// Синтаксис:
//   project = "PRJ" AND status IN ("Новая", "В работе") AND assignee = currentUser()
//   AND updated > -7d AND text ~ "ошибка" ORDER BY priority DESC
//
// Поля: project, status, priority, assignee, author, watcher, label, sprint,
//       created, updated, start, target, text
// Операторы: = != ~ > < >= <= IN NOT IN IS EMPTY IS NOT EMPTY
// Связки: AND OR NOT, скобки (любая вложенность)
// Значения: "строка", слово, 2026-01-01, -7d (-2w, -1m), currentUser()
// ORDER BY: field ASC|DESC
package jql

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// AST
// ---------------------------------------------------------------------------

type Op int

const (
	OpEq Op = iota
	OpNeq
	OpContains
	OpGt
	OpLt
	OpGte
	OpLte
	OpIn
	OpNotIn
	OpIsEmpty
	OpIsNotEmpty
)

func (op Op) String() string {
	switch op {
	case OpEq:
		return "="
	case OpNeq:
		return "!="
	case OpContains:
		return "~"
	case OpGt:
		return ">"
	case OpLt:
		return "<"
	case OpGte:
		return ">="
	case OpLte:
		return "<="
	case OpIn:
		return "IN"
	case OpNotIn:
		return "NOT IN"
	case OpIsEmpty:
		return "IS EMPTY"
	case OpIsNotEmpty:
		return "IS NOT EMPTY"
	}
	return "?"
}

// Value — значение условия.
type Value struct {
	Str     string    // обычное строковое значение (или имя функции)
	IsFunc  bool      // currentUser()
	List    []string  // для IN
	Date    time.Time // абсолютная дата (YYYY-MM-DD)
	HasDate bool
	RelDays int  // относительная дата в днях (отрицательное = в прошлом), 0 = не задана
	HasRel  bool
}

type Clause struct {
	Field   string // каноническое имя поля
	Op      Op
	Negated bool // условие с NOT (обрабатывается пост-фильтрацией)
	Value   Value
}

// Group — конъюнкция условий (AND).
type Group struct {
	Clauses []Clause
}

// Query — дизъюнкция групп (OR).
type Query struct {
	Groups  []Group
	OrderBy string // каноническое поле сортировки, "" = по умолчанию
	Desc    bool
}

// ---------------------------------------------------------------------------
// Валидные поля и алиасы
// ---------------------------------------------------------------------------

var fieldAliases = map[string]string{
	"project":  "project",
	"projects": "project",
	"status":   "state",
	"state":    "state",
	"priority": "priority",
	"assignee": "assignee",
	"author":   "author",
	"reporter": "author",
	"watcher":  "watcher",
	"label":    "label",
	"labels":   "label",
	"sprint":   "sprint",
	"created":  "created_at",
	"updated":  "updated_at",
	"start":    "start_date",
	"target":   "target_date",
	"due":      "target_date",
	"text":     "text",
	"summary":  "text",
}

var dateFields = map[string]bool{
	"created_at":  true,
	"updated_at":  true,
	"start_date":  true,
	"target_date": true,
}

// orderAliases — поля для ORDER BY (с алиасами).
var orderAliases = map[string]string{
	"sequence_id": "sequence_id",
	"sequence":    "sequence_id",
	"sequenceid":  "sequence_id",
	"key":         "sequence_id",
	"created":     "created_at",
	"created_at":  "created_at",
	"updated":     "updated_at",
	"updated_at":  "updated_at",
	"start":       "start_date",
	"start_date":  "start_date",
	"target":      "target_date",
	"target_date": "target_date",
	"due":         "target_date",
	"name":        "name",
	"priority":    "priority",
	"status":      "state",
	"state":       "state",
}

func orderField(name string) string {
	if f, ok := orderAliases[strings.ToLower(name)]; ok {
		return f
	}
	return ""
}

// ---------------------------------------------------------------------------
// Лексер
// ---------------------------------------------------------------------------

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokString
	tokNumber
	tokRelative
	tokLParen
	tokRParen
	tokComma
	tokEq
	tokNeq
	tokTilde
	tokGt
	tokLt
	tokGte
	tokLte
	tokAnd
	tokOr
	tokNot
	tokIn
	tokIs
	tokEmpty
	tokOrder
	tokBy
	tokAsc
	tokDesc
)

type token struct {
	kind tokKind
	text string
}

type lexer struct {
	input string
	pos   int
}

func isDigits(input string, from, n int) bool {
	if from+n > len(input) {
		return false
	}
	for i := 0; i < n; i++ {
		c := input[from+i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isIdentChar(r byte) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r >= 0x80: // кириллица и прочий юникод
		return true
	case r == '_' || r == '-':
		return true
	}
	return false
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			l.pos++
			continue
		}
		break
	}
	if l.pos >= len(l.input) {
		return token{kind: tokEOF}, nil
	}

	c := l.input[l.pos]
	switch c {
	case '(':
		l.pos++
		return token{kind: tokLParen, text: "("}, nil
	case ')':
		l.pos++
		return token{kind: tokRParen, text: ")"}, nil
	case ',':
		l.pos++
		return token{kind: tokComma, text: ","}, nil
	case '~':
		l.pos++
		return token{kind: tokTilde, text: "~"}, nil
	case '=':
		l.pos++
		return token{kind: tokEq, text: "="}, nil
	case '>':
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokGte, text: ">="}, nil
		}
		return token{kind: tokGt, text: ">"}, nil
	case '<':
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokLte, text: "<="}, nil
		}
		return token{kind: tokLt, text: "<"}, nil
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{kind: tokNeq, text: "!="}, nil
		}
		return token{}, fmt.Errorf("неожиданный символ '!' на позиции %d", l.pos)
	case '\'', '"':
		quote := c
		l.pos++
		start := l.pos
		var sb strings.Builder
		for l.pos < len(l.input) && l.input[l.pos] != quote {
			sb.WriteByte(l.input[l.pos])
			l.pos++
		}
		if l.pos >= len(l.input) {
			return token{}, fmt.Errorf("незакрытая строка на позиции %d", start)
		}
		l.pos++ // закрывающая кавычка
		return token{kind: tokString, text: sb.String()}, nil
	case '-':
		// относительная дата -7d / -2w / -1m
		if l.pos+1 < len(l.input) && l.input[l.pos+1] >= '0' && l.input[l.pos+1] <= '9' {
			l.pos++
			start := l.pos
			for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
				l.pos++
			}
			num := l.input[start:l.pos]
			unit := byte(0)
			if l.pos < len(l.input) && (l.input[l.pos] == 'd' || l.input[l.pos] == 'w' || l.input[l.pos] == 'm') {
				unit = l.input[l.pos]
				l.pos++
			}
			if unit != 0 {
				return token{kind: tokRelative, text: "-" + num + string(unit)}, nil
			}
			return token{kind: tokNumber, text: "-" + num}, nil
		}
		return token{}, fmt.Errorf("неожиданный символ '-' на позиции %d", l.pos)
	default:
		if c >= '0' && c <= '9' {
			start := l.pos
			for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
				l.pos++
			}
			// дата YYYY-MM-DD: читаем целиком как один токен-идентификатор
			if l.pos-start == 4 && l.pos+5 < len(l.input)+1 &&
				l.input[l.pos] == '-' &&
				isDigits(l.input, l.pos+1, 2) &&
				l.input[l.pos+3] == '-' &&
				isDigits(l.input, l.pos+4, 2) {
				l.pos += 6
				return token{kind: tokIdent, text: l.input[start:l.pos]}, nil
			}
			return token{kind: tokNumber, text: l.input[start:l.pos]}, nil
		}
		if isIdentChar(c) {
			start := l.pos
			for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
				l.pos++
			}
			word := l.input[start:l.pos]
			switch strings.ToUpper(word) {
			case "AND":
				return token{kind: tokAnd, text: word}, nil
			case "OR":
				return token{kind: tokOr, text: word}, nil
			case "NOT":
				return token{kind: tokNot, text: word}, nil
			case "IN":
				return token{kind: tokIn, text: word}, nil
			case "IS":
				return token{kind: tokIs, text: word}, nil
			case "EMPTY":
				return token{kind: tokEmpty, text: word}, nil
			case "ORDER":
				return token{kind: tokOrder, text: word}, nil
			case "BY":
				return token{kind: tokBy, text: word}, nil
			case "ASC":
				return token{kind: tokAsc, text: word}, nil
			case "DESC":
				return token{kind: tokDesc, text: word}, nil
			}
			return token{kind: tokIdent, text: word}, nil
		}
	}
	return token{}, fmt.Errorf("неожиданный символ %q на позиции %d", string(c), l.pos)
}

// ---------------------------------------------------------------------------
// Парсер (recursive descent)
// ---------------------------------------------------------------------------

type parser struct {
	lex   *lexer
	tok   token
	peek  token
}

func newParser(input string) (*parser, error) {
	l := &lexer{input: input}
	p := &parser{lex: l}
	var err error
	if p.tok, err = l.next(); err != nil {
		return nil, err
	}
	if p.peek, err = l.next(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *parser) advance() error {
	p.tok = p.peek
	var err error
	if p.peek, err = p.lex.next(); err != nil {
		return err
	}
	return nil
}

func (p *parser) match(kind tokKind) (bool, error) {
	if p.tok.kind == kind {
		return true, p.advance()
	}
	return false, nil
}

// Parse разбирает JQL-запрос.
func Parse(input string) (*Query, error) {
	p, err := newParser(input)
	if err != nil {
		return nil, err
	}
	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if p.tok.kind != tokEOF {
		return nil, fmt.Errorf("неожиданный токен %q", p.tok.text)
	}
	return q, nil
}

// jqlMaxParseGroups — защита от экспоненциального взрыва ДНФ.
const jqlMaxParseGroups = 32

func (p *parser) parseQuery() (*Query, error) {
	q := &Query{}
	// OR-группы на верхнем уровне: group1 OR group2 OR ...
	for {
		groups, err := p.parseAndDNF()
		if err != nil {
			return nil, err
		}
		q.Groups = append(q.Groups, groups...)
		if len(q.Groups) > jqlMaxParseGroups {
			return nil, fmt.Errorf("запрос слишком сложный (больше %d OR-групп)", jqlMaxParseGroups)
		}
		if p.tok.kind == tokOr {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}

	// ORDER BY
	if p.tok.kind == tokOrder {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind != tokBy {
			return nil, fmt.Errorf("ожидался BY после ORDER")
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind != tokIdent {
			return nil, fmt.Errorf("ожидалось поле после ORDER BY")
		}
		field := orderField(p.tok.text)
		if field == "" {
			return nil, fmt.Errorf("неизвестное поле сортировки %q", p.tok.text)
		}
		q.OrderBy = field
		if err := p.advance(); err != nil {
			return nil, err
		}
		switch p.tok.kind {
		case tokAsc:
			q.Desc = false
			if err := p.advance(); err != nil {
				return nil, err
			}
		case tokDesc:
			q.Desc = true
			if err := p.advance(); err != nil {
				return nil, err
			}
		default:
			// по умолчанию — как в поиске: sequence_id по возрастанию
		}
	}
	return q, nil
}

// parseAndDNF разбирает конъюнкцию, возвращая список OR-групп в ДНФ
// (OR внутри скобок раскрывается декартовым произведением).
func (p *parser) parseAndDNF() ([]Group, error) {
	groups := []Group{{}}
	for {
		orGroups, err := p.parseUnaryDNF()
		if err != nil {
			return nil, err
		}
		var next []Group
		for _, g := range groups {
			for _, ug := range orGroups {
				merged := Group{Clauses: make([]Clause, 0, len(g.Clauses)+len(ug.Clauses))}
				merged.Clauses = append(merged.Clauses, g.Clauses...)
				merged.Clauses = append(merged.Clauses, ug.Clauses...)
				next = append(next, merged)
			}
		}
		groups = next
		if len(groups) > jqlMaxParseGroups {
			return nil, fmt.Errorf("запрос слишком сложный (больше %d OR-групп)", jqlMaxParseGroups)
		}
		if p.tok.kind == tokAnd {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	return groups, nil
}

// parseUnaryDNF разбирает один унарный элемент и возвращает его ДНФ-разложение.
func (p *parser) parseUnaryDNF() ([]Group, error) {
	if p.tok.kind == tokNot {
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseUnaryDNF()
		if err != nil {
			return nil, err
		}
		// De Morgan: NOT(OR групп) = AND(NOT каждой группы),
		// NOT(AND условий) = OR(NOT каждого условия).
		var negated []Group
		for _, gi := range inner {
			orParts := make([]Group, 0, len(gi.Clauses))
			for _, c := range gi.Clauses {
				nc := c
				nc.Negated = !nc.Negated
				orParts = append(orParts, Group{Clauses: []Clause{nc}})
			}
			if len(orParts) == 0 {
				continue
			}
			if len(negated) == 0 {
				negated = orParts
			} else {
				var next []Group
				for _, a := range negated {
					for _, b := range orParts {
						merged := Group{Clauses: append(append([]Clause{}, a.Clauses...), b.Clauses...)}
						next = append(next, merged)
					}
				}
				negated = next
			}
		}
		return negated, nil
	}
	if p.tok.kind == tokLParen {
		if err := p.advance(); err != nil {
			return nil, err
		}
		groups, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.tok.kind != tokRParen {
			return nil, fmt.Errorf("ожидалась ')'")
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return groups, nil
	}
	clause, err := p.parseClause()
	if err != nil {
		return nil, err
	}
	if err := ValidateFieldOp(clause); err != nil {
		return nil, err
	}
	return []Group{{Clauses: []Clause{clause}}}, nil
}

// parseOr разбирает последовательность OR-групп (используется в скобках).
func (p *parser) parseOr() ([]Group, error) {
	var groups []Group
	for {
		gs, err := p.parseAndDNF()
		if err != nil {
			return nil, err
		}
		groups = append(groups, gs...)
		if len(groups) > jqlMaxParseGroups {
			return nil, fmt.Errorf("запрос слишком сложный (больше %d OR-групп)", jqlMaxParseGroups)
		}
		if p.tok.kind == tokOr {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	return groups, nil
}

func (p *parser) parseClause() (Clause, error) {
	if p.tok.kind != tokIdent {
		return Clause{}, fmt.Errorf("ожидалось поле, получено %q", p.tok.text)
	}
	field := canonicalField(p.tok.text)
	if field == "" {
		return Clause{}, fmt.Errorf("неизвестное поле %q", p.tok.text)
	}
	if err := p.advance(); err != nil {
		return Clause{}, err
	}

	clause := Clause{Field: field}

	switch p.tok.kind {
	case tokEq, tokNeq, tokTilde, tokGt, tokLt, tokGte, tokLte:
		switch p.tok.kind {
		case tokEq:
			clause.Op = OpEq
		case tokNeq:
			clause.Op = OpNeq
		case tokTilde:
			clause.Op = OpContains
		case tokGt:
			clause.Op = OpGt
		case tokLt:
			clause.Op = OpLt
		case tokGte:
			clause.Op = OpGte
		case tokLte:
			clause.Op = OpLte
		}
		if err := p.advance(); err != nil {
			return Clause{}, err
		}
		val, err := p.parseValue()
		if err != nil {
			return Clause{}, err
		}
		clause.Value = val
		return clause, nil

	case tokIn:
		if err := p.advance(); err != nil {
			return Clause{}, err
		}
		if p.tok.kind != tokLParen {
			return Clause{}, fmt.Errorf("ожидалась '(' после IN")
		}
		list, err := p.parseInList()
		if err != nil {
			return Clause{}, err
		}
		clause.Op = OpIn
		clause.Value = Value{List: list}
		return clause, nil

	case tokNot:
		// NOT IN ( ... )
		if p.peek.kind != tokIn {
			return Clause{}, fmt.Errorf("NOT поддерживается только в виде NOT IN (...) после поля")
		}
		if err := p.advance(); err != nil { // NOT
			return Clause{}, err
		}
		if err := p.advance(); err != nil { // IN
			return Clause{}, err
		}
		if p.tok.kind != tokLParen {
			return Clause{}, fmt.Errorf("ожидалась '(' после NOT IN")
		}
		list, err := p.parseInList()
		if err != nil {
			return Clause{}, err
		}
		clause.Op = OpNotIn
		clause.Value = Value{List: list}
		return clause, nil

	case tokIs:
		if err := p.advance(); err != nil {
			return Clause{}, err
		}
		negated := false
		if p.tok.kind == tokNot {
			negated = true
			if err := p.advance(); err != nil {
				return Clause{}, err
			}
		}
		if p.tok.kind != tokEmpty {
			return Clause{}, fmt.Errorf("ожидался EMPTY после IS")
		}
		if err := p.advance(); err != nil {
			return Clause{}, err
		}
		clause.Op = OpIsEmpty
		if negated {
			clause.Op = OpIsNotEmpty
		}
		return clause, nil
	}

	return Clause{}, fmt.Errorf("ожидался оператор после поля %q", p.tok.text)
}

// parseInList разбирает список значений в скобках после IN / NOT IN.
// Вызывается, когда текущий токен — '('.
func (p *parser) parseInList() ([]string, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	var list []string
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		list = append(list, val.Str)
		if p.tok.kind == tokComma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		break
	}
	if p.tok.kind != tokRParen {
		return nil, fmt.Errorf("ожидалась ')' после списка")
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return list, nil
}

func (p *parser) parseValue() (Value, error) {
	switch p.tok.kind {
	case tokString:
		v := Value{Str: p.tok.text}
		if err := p.advance(); err != nil {
			return Value{}, err
		}
		return v, nil
	case tokRelative:
		text := p.tok.text
		if err := p.advance(); err != nil {
			return Value{}, err
		}
		days, err := parseRelative(text)
		if err != nil {
			return Value{}, err
		}
		return Value{RelDays: days, HasRel: true}, nil
	case tokNumber:
		text := p.tok.text
		if err := p.advance(); err != nil {
			return Value{}, err
		}
		return Value{Str: text}, nil
	case tokIdent:
		text := p.tok.text
		if err := p.advance(); err != nil {
			return Value{}, err
		}
		// функция вида name()
		if p.tok.kind == tokLParen {
			if err := p.advance(); err != nil {
				return Value{}, err
			}
			if p.tok.kind != tokRParen {
				return Value{}, fmt.Errorf("ожидалась ')' в вызове функции %s", text)
			}
			if err := p.advance(); err != nil {
				return Value{}, err
			}
			if strings.ToLower(text) != "currentuser" {
				return Value{}, fmt.Errorf("неизвестная функция %q (поддерживается только currentUser())", text)
			}
			return Value{Str: "currentUser()", IsFunc: true}, nil
		}
		// дата YYYY-MM-DD?
		if t, ok := parseDate(text); ok {
			return Value{Date: t, HasDate: true}, nil
		}
		return Value{Str: text}, nil
	}
	return Value{}, fmt.Errorf("ожидалось значение, получено %q", p.tok.text)
}

func canonicalField(name string) string {
	if f, ok := fieldAliases[strings.ToLower(name)]; ok {
		return f
	}
	return ""
}

func parseRelative(text string) (int, error) {
	unit := text[len(text)-1]
	num := text[:len(text)-1]
	n := 0
	if _, err := fmt.Sscanf(num, "%d", &n); err != nil {
		return 0, fmt.Errorf("некорректная относительная дата %q", text)
	}
	switch unit {
	case 'd':
		return n, nil
	case 'w':
		return n * 7, nil
	case 'm':
		return n * 30, nil
	}
	return 0, fmt.Errorf("некорректная относительная дата %q", text)
}

func parseDate(text string) (time.Time, bool) {
	if len(text) != 10 || text[4] != '-' || text[7] != '-' {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", text)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ValidateFieldOp проверяет допустимость оператора для поля.
func ValidateFieldOp(clause Clause) error {
	// IS EMPTY / IS NOT EMPTY: списки, приоритет и поля дат
	if clause.Op == OpIsEmpty || clause.Op == OpIsNotEmpty {
		switch clause.Field {
		case "assignee", "watcher", "label", "sprint", "priority",
			"created_at", "updated_at", "start_date", "target_date":
			return nil
		}
		return fmt.Errorf("IS EMPTY поддерживается только для assignee/watcher/label/sprint/priority и полей дат")
	}
	if dateFields[clause.Field] {
		switch clause.Op {
		case OpGt, OpLt, OpGte, OpLte, OpEq, OpNeq:
			if !clause.Value.HasDate && !clause.Value.HasRel {
				return fmt.Errorf("поле %q с оператором %s требует дату (YYYY-MM-DD или -7d)", clause.Field, clause.Op)
			}
		default:
			return fmt.Errorf("оператор %s не поддерживается для поля даты %q", clause.Op, clause.Field)
		}
	}
	if clause.Field == "text" {
		switch clause.Op {
		case OpEq, OpNeq, OpContains:
		default:
			return fmt.Errorf("оператор %s не поддерживается для поля text", clause.Op)
		}
	}
	return nil
}
