// Code generated from AQL.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // AQL
import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by AQLParser.
type AQLVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by AQLParser#query.
	VisitQuery(ctx *QueryContext) interface{}

	// Visit a parse tree produced by AQLParser#notExpr.
	VisitNotExpr(ctx *NotExprContext) interface{}

	// Visit a parse tree produced by AQLParser#orExpr.
	VisitOrExpr(ctx *OrExprContext) interface{}

	// Visit a parse tree produced by AQLParser#clauseExpr.
	VisitClauseExpr(ctx *ClauseExprContext) interface{}

	// Visit a parse tree produced by AQLParser#parenExpr.
	VisitParenExpr(ctx *ParenExprContext) interface{}

	// Visit a parse tree produced by AQLParser#andExpr.
	VisitAndExpr(ctx *AndExprContext) interface{}

	// Visit a parse tree produced by AQLParser#compareClause.
	VisitCompareClause(ctx *CompareClauseContext) interface{}

	// Visit a parse tree produced by AQLParser#inClause.
	VisitInClause(ctx *InClauseContext) interface{}

	// Visit a parse tree produced by AQLParser#isEmptyClause.
	VisitIsEmptyClause(ctx *IsEmptyClauseContext) interface{}

	// Visit a parse tree produced by AQLParser#wasClause.
	VisitWasClause(ctx *WasClauseContext) interface{}

	// Visit a parse tree produced by AQLParser#changedClause.
	VisitChangedClause(ctx *ChangedClauseContext) interface{}

	// Visit a parse tree produced by AQLParser#field.
	VisitField(ctx *FieldContext) interface{}

	// Visit a parse tree produced by AQLParser#operand.
	VisitOperand(ctx *OperandContext) interface{}

	// Visit a parse tree produced by AQLParser#inOperand.
	VisitInOperand(ctx *InOperandContext) interface{}

	// Visit a parse tree produced by AQLParser#list.
	VisitList(ctx *ListContext) interface{}

	// Visit a parse tree produced by AQLParser#function.
	VisitFunction(ctx *FunctionContext) interface{}

	// Visit a parse tree produced by AQLParser#historyPredicate.
	VisitHistoryPredicate(ctx *HistoryPredicateContext) interface{}

	// Visit a parse tree produced by AQLParser#orderBy.
	VisitOrderBy(ctx *OrderByContext) interface{}

	// Visit a parse tree produced by AQLParser#sortField.
	VisitSortField(ctx *SortFieldContext) interface{}

	// Visit a parse tree produced by AQLParser#value.
	VisitValue(ctx *ValueContext) interface{}
}
