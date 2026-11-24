package aql

import (
	"fmt"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/antlr4-go/antlr/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestParser(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost"), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	lexer := parser.NewAQLLexer(antlr.NewInputStream("priority in test()"))
	stream := antlr.NewCommonTokenStream(lexer, antlr.LexerDefaultTokenChannel)
	p := parser.NewAQLParser(stream)
	tree := p.Query()
	v := NewVisitor(db, nil, nil, nil)

	res := tree.Accept(v)
	if err, ok := res.(error); ok {
		fmt.Println("err", err)
	} else {
		fmt.Println(db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return res.(*gorm.DB).First(&dao.Issue{})
		}),
		)
	}

	fmt.Println(tree.ToStringTree(nil, p))
}
