package aql

import (
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
)

// fieldFuncs — условия, которые умеет поле.
type fieldFuncs struct {
	in    inFunc
	cmp   cmpFunc
	empty emptyFunc
}

// field — системное поле, а если такого нет — параметр задачи с этим названием.
func (v *Visitor) field(ctx parser.IFieldContext) fieldFuncs {
	raw, ok := ctx.Accept(v).(string)
	if !ok {
		panic(at(ErrFieldName, ctx))
	}
	name := canonicalField(raw)
	f := fieldFuncs{in: inFieldsFunc[name], cmp: cmpFieldsFunc[name], empty: emptyFieldsFunc[name]}
	if f.in == nil && f.cmp == nil && f.empty == nil {
		return v.propertyField(strings.ToLower(raw), ctx)
	}
	return f
}

// Другие имена полей AQL. Ключи в нижнем регистре: имена полей регистронезависимы, как в JQL.
var fieldAliases = map[string]string{
	// JQL
	"reporter":       "author",
	"creator":        "author",
	"issuetype":      "type",
	"due":            "duedate",
	"resolved":       "completed",
	"resolutiondate": "completed",
	"key":            "issue",
	"issuekey":       "issue",
	"id":             "issue",
	"summary":        "title",
	"createddate":    "created",
	"updateddate":    "updated",
	"statuscategory": "statusgroup",

	// Сортировки API поиска задач (order_by)
	"state":       "status",
	"assignees":   "assignee",
	"watchers":    "watcher",
	"name":        "title",
	"sequence_id": "issue",
	"created_at":  "created",
	"updated_at":  "updated",
	"target_date": "duedate",
}

// canonicalField приводит имя поля к нижнему регистру и заменяет синоним на имя AQL.
func canonicalField(name string) string {
	name = strings.ToLower(name)
	if alias, ok := fieldAliases[name]; ok {
		return alias
	}
	return name
}
