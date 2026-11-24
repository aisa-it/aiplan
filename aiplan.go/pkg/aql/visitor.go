package aql

import (
	"strconv"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/antlr4-go/antlr/v4"
	"gorm.io/gorm"
)

type Visitor struct {
	*parser.BaseAQLVisitor
	query *gorm.DB

	selectExprs     []string
	selectInterface []any

	currentUser    *dao.User
	scopeIssues    ScopeFunc
	scopeTemplates dao.PropertyTemplateScope

	exprCount int
	funcCount int

	templatesCache map[string][]dao.ProjectPropertyTemplate
}

func (v *Visitor) VisitQuery(ctx *parser.QueryContext) interface{} {
	v.selectExprs = []string{
		"issues.*",
	}
	v.selectInterface = []any{}

	q := v.query.Session(&gorm.Session{})
	if ctx.Expr() != nil {
		q = q.Where(ctx.Expr().Accept(v))
	}
	if ctx.OrderBy() != nil {
		q = q.Order(ctx.OrderBy().Accept(v))
	} else {
		q = q.Order("issues.sequence_id DESC")
	}
	return q.Select(strings.Join(v.selectExprs, ", "), v.selectInterface...)
}

// Expressions

func (v *Visitor) VisitNotExpr(ctx *parser.NotExprContext) interface{} {
	return v.newQuery().Not(ctx.Expr().Accept(v))
}

func (v *Visitor) VisitOrExpr(ctx *parser.OrExprContext) interface{} {
	qq := v.newQuery()
	for _, q := range v.visitChildrenExpr(ctx.AllExpr()) {
		qq = qq.Or(q)
	}
	return qq
}

func (v *Visitor) VisitClauseExpr(ctx *parser.ClauseExprContext) interface{} {
	v.count(&v.exprCount, maxExprCount, ErrExprCountExceed, ctx)
	return ctx.Clause().Accept(v).(*gorm.DB)
}

func (v *Visitor) VisitParenExpr(ctx *parser.ParenExprContext) interface{} {
	return ctx.Expr().Accept(v)
}

func (v *Visitor) VisitAndExpr(ctx *parser.AndExprContext) interface{} {
	qq := v.newQuery()
	for _, q := range v.visitChildrenExpr(ctx.AllExpr()) {
		qq = qq.Where(q)
	}
	return qq
}

// Clauses

func (v *Visitor) VisitCompareClause(ctx *parser.CompareClauseContext) interface{} {
	f := v.field(ctx.Field())
	if f.in == nil && f.cmp == nil {
		panic(at(ErrFieldUnsupported, ctx.Field()))
	}
	op := ctx.GetOp().GetTokenType()
	value := ctx.Operand().Accept(v)
	// = и != у полей с IN идут через IN, даже если поле умеет < и >: так одинаково работают EMPTY и отрицание.
	if f.in != nil && (op == parser.AQLParserEQ || op == parser.AQLParserNEQ) {
		return v.inQuery(f, []any{value}, op == parser.AQLParserNEQ, ctx.Operand())
	}
	if f.cmp == nil {
		panic(at(ErrOperator, ctx))
	}
	return v.cmpQuery(f, op, value, ctx.Operand())
}

func (v *Visitor) VisitInClause(ctx *parser.InClauseContext) interface{} {
	f := v.field(ctx.Field())
	if f.in == nil && f.cmp != nil {
		f.in = cmpIn(f.cmp)
	}
	if f.in == nil {
		panic(at(ErrFieldUnsupported, ctx.Field()))
	}
	operand := ctx.InOperand().Accept(v)
	values, ok := operand.([]any)
	if !ok {
		values = []any{operand}
	}
	return v.inQuery(f, values, ctx.NOT() != nil, ctx.InOperand())
}

func (v *Visitor) VisitIsEmptyClause(ctx *parser.IsEmptyClauseContext) interface{} {
	f := v.field(ctx.Field())
	if f.empty == nil {
		panic(at(ErrEmptyUnsupported, ctx.Field()))
	}
	return f.empty(v, ctx.NOT() != nil)
}

func (v *Visitor) VisitWasClause(ctx *parser.WasClauseContext) interface{} {
	panic(at(ErrHistoryUnsupported, ctx))
}

func (v *Visitor) VisitChangedClause(ctx *parser.ChangedClauseContext) interface{} {
	panic(at(ErrHistoryUnsupported, ctx))
}

// Fields

func (v *Visitor) VisitField(ctx *parser.FieldContext) interface{} {
	if ctx.LBRACKET() != nil {
		panic(at(ErrCustomFieldID, ctx))
	}
	return ctx.Value().Accept(v)
}

func (v *Visitor) VisitOperand(ctx *parser.OperandContext) interface{} {
	switch {
	case ctx.EMPTY() != nil:
		return nil
	case ctx.Function() != nil:
		return ctx.Function().Accept(v)
	case ctx.Value() != nil:
		return ctx.Value().Accept(v)
	}

	panic(at(ErrInternal, ctx))
}

func (v *Visitor) VisitInOperand(ctx *parser.InOperandContext) interface{} {
	switch {
	case ctx.Function() != nil:
		return ctx.Function().Accept(v)
	case ctx.List() != nil:
		return ctx.List().Accept(v)
	}
	panic(at(ErrInternal, ctx))
}

func (v *Visitor) VisitList(ctx *parser.ListContext) interface{} {
	if len(ctx.AllOperand()) > maxListSize {
		panic(at(ErrListSizeExceed, ctx))
	}

	res := make([]any, 0, len(ctx.AllOperand()))
	for _, item := range ctx.AllOperand() {
		res = append(res, item.Accept(v))
	}
	return res
}

func (v *Visitor) VisitFunction(ctx *parser.FunctionContext) interface{} {
	v.count(&v.funcCount, maxFuncCount, ErrFuncCountExceed, ctx)

	name := ctx.STRING().GetText()
	args := make([]any, 0, len(ctx.AllValue()))
	for _, arg := range ctx.AllValue() {
		args = append(args, arg.Accept(v))
	}

	_, afterIn := ctx.GetParent().(*parser.InOperandContext)
	listFunc, isList := listFuncs[name]
	scalarFunc, isScalar := scalarFuncs[name]

	var res any
	var err error
	switch {
	case afterIn && isList:
		res, err = listFunc(v, args...)
	case !afterIn && isScalar:
		res, err = scalarFunc(v, args...)
	case isList:
		err = ErrListFunc
	case isScalar:
		err = ErrScalarFunc
	default:
		err = ErrFuncNotFound
	}
	if err != nil {
		panic(at(err, ctx))
	}
	return res
}

func (v *Visitor) VisitHistoryPredicate(ctx *parser.HistoryPredicateContext) interface{} {
	panic(at(ErrHistoryUnsupported, ctx))
}

func (v *Visitor) VisitValue(ctx *parser.ValueContext) interface{} {
	switch {
	case ctx.DURATION() != nil:
		dur, err := parseDuration(ctx.DURATION().GetText())
		if err != nil {
			panic(at(err, ctx))
		}
		return dur
	case ctx.NUMBER() != nil:
		i, err := strconv.Atoi(ctx.NUMBER().GetText())
		if err != nil {
			panic(at(ErrNumberRange, ctx))
		}
		return i
	case ctx.QUOTED_STRING() != nil:
		return unquote(ctx.QUOTED_STRING().GetText())
	case ctx.STRING() != nil:
		return ctx.STRING().GetText()
	}

	panic(at(ErrInternal, ctx))
}

func (v *Visitor) visitChildrenExpr(exprs []parser.IExprContext) []any {
	res := make([]any, 0, len(exprs))
	for _, expr := range exprs {
		res = append(res, expr.Accept(v))
	}
	return res
}

// count увеличивает счётчик и останавливает разбор, когда он превысил лимит.
func (v *Visitor) count(counter *int, limit int, err Error, ctx antlr.ParserRuleContext) {
	*counter++
	if *counter > limit {
		panic(at(err, ctx))
	}
}
