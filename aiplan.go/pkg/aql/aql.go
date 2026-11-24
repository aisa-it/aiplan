package aql

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
)

func ParseAQLQuery(db *gorm.DB, scope ScopeFunc, templateScope dao.PropertyTemplateScope, currentUser *dao.User, query string) (q *gorm.DB, err error) {
	if currentUser == nil {
		return nil, ErrNoUser
	}

	if len(query) > maxQuerySize {
		e := ErrQueryOverflow
		return nil, &e
	}

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(Error); ok {
				err = &e
				return
			}
			panic(r)
		}
	}()

	tree, err := parseQuery(query)
	if err != nil {
		return nil, err
	}

	v := NewVisitor(db, currentUser, scope, templateScope)
	q = tree.Accept(v).(*gorm.DB)
	return
}
