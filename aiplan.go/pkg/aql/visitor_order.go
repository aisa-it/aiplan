package aql

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"gorm.io/gorm/clause"
)

func (v *Visitor) VisitOrderBy(ctx *parser.OrderByContext) interface{} {
	order := clause.OrderBy{}
	for _, o := range ctx.AllSortField() {
		order.Columns = append(order.Columns, o.Accept(v).(clause.OrderByColumn))
	}
	return order
}

func (v *Visitor) VisitSortField(ctx *parser.SortFieldContext) interface{} {
	desc := ctx.DESC() != nil
	orderBy, _ := ctx.Field().Accept(v).(string)

	order, err := v.parseSortOrder(orderBy)
	if err != nil {
		panic(at(err, ctx))
	}
	order.Desc = desc
	return order
}
