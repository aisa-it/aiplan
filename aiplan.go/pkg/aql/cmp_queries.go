package aql

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/antlr4-go/antlr/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cmpFunc func(v *Visitor, op int, value any) (*gorm.DB, error)

// cmpFieldsFunc — поля, которые сравниваются через =, !=, <, <=, >, >=.
var cmpFieldsFunc = map[string]cmpFunc{
	"created":   dateCmp("issues.created_at"),
	"updated":   dateCmp("issues.updated_at"),
	"completed": dateCmp("issues.completed_at"),
	"startdate": dateCmp("issues.start_date"),
	"duedate":   dateCmp("issues.target_date"),
}

// cmpQuery — сравнение поля с одним значением; = EMPTY и != EMPTY проверяют поле на пустоту.
func (v *Visitor) cmpQuery(f fieldFuncs, op int, value any, valueCtx antlr.ParserRuleContext) *gorm.DB {
	if value == nil {
		if f.empty == nil || (op != parser.AQLParserEQ && op != parser.AQLParserNEQ) {
			panic(at(ErrEmptyUnsupported, valueCtx))
		}
		return f.empty(v, op == parser.AQLParserNEQ)
	}
	cond, err := f.cmp(v, op, value)
	if err != nil {
		panic(at(err, valueCtx))
	}
	return cond
}

// dateCmp — column может быть именем колонки или clause.Expr с выражением.
// cmpIn — IN для полей со сравнением: значение равно одному из списка.
func cmpIn(cmp cmpFunc) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		cond := v.newQuery()
		for _, value := range values {
			eq, err := cmp(v, parser.AQLParserEQ, value)
			if err != nil {
				return nil, err
			}
			cond = cond.Or(eq)
		}
		return cond, nil
	}
}

func dateCmp(column any) cmpFunc {
	return func(v *Visitor, op int, value any) (*gorm.DB, error) {
		value, err := v.dateValue(value)
		if err != nil {
			return nil, err
		}
		if d, ok := value.(Date); ok {
			return dayCmp(v, op, column, d)
		}

		expr := mapCmpExpr(op, column, value)
		if expr == nil {
			return nil, ErrDateOperator
		}
		return v.newQuery().Where(expr), nil
	}
}

// dayCmp сравнивает с целыми сутками: = попадает в них, <= их включает, > начинается со следующих.
func dayCmp(v *Visitor, op int, column any, d Date) (*gorm.DB, error) {
	start := time.Time(d)
	next := start.AddDate(0, 0, 1)

	var expr clause.Expression
	switch op {
	case parser.AQLParserEQ:
		expr = clause.And(clause.Gte{Column: column, Value: start}, clause.Lt{Column: column, Value: next})
	case parser.AQLParserNEQ:
		expr = clause.Or(clause.Lt{Column: column, Value: start}, clause.Gte{Column: column, Value: next})
	case parser.AQLParserLTE:
		expr = clause.Lt{Column: column, Value: next}
	case parser.AQLParserGT:
		expr = clause.Gte{Column: column, Value: next}
	default:
		expr = mapCmpExpr(op, column, start)
	}
	if expr == nil {
		return nil, ErrDateOperator
	}
	return v.newQuery().Where(expr), nil
}

func mapCmpExpr(op int, column, value any) clause.Expression {
	switch op {
	case parser.AQLParserEQ:
		return clause.Eq{Column: column, Value: value}
	case parser.AQLParserNEQ:
		return clause.Neq{Column: column, Value: value}
	case parser.AQLParserLT:
		return clause.Lt{Column: column, Value: value}
	case parser.AQLParserLTE:
		return clause.Lte{Column: column, Value: value}
	case parser.AQLParserGT:
		return clause.Gt{Column: column, Value: value}
	case parser.AQLParserGTE:
		return clause.Gte{Column: column, Value: value}
	}
	return nil
}
