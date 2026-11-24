// Code generated from AQL.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // AQL
import "github.com/antlr4-go/antlr/v4"

type BaseAQLVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseAQLVisitor) VisitQuery(ctx *QueryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitNotExpr(ctx *NotExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitOrExpr(ctx *OrExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitClauseExpr(ctx *ClauseExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitParenExpr(ctx *ParenExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitAndExpr(ctx *AndExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitCompareClause(ctx *CompareClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitInClause(ctx *InClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitIsEmptyClause(ctx *IsEmptyClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitWasClause(ctx *WasClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitChangedClause(ctx *ChangedClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitField(ctx *FieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitOperand(ctx *OperandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitInOperand(ctx *InOperandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitList(ctx *ListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitFunction(ctx *FunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitHistoryPredicate(ctx *HistoryPredicateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitOrderBy(ctx *OrderByContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitSortField(ctx *SortFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseAQLVisitor) VisitValue(ctx *ValueContext) interface{} {
	return v.VisitChildren(ctx)
}
