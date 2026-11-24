// Code generated from AQL.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // AQL
import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr4-go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type AQLParser struct {
	*antlr.BaseParser
}

var AQLParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func aqlParserInit() {
	staticData := &AQLParserStaticData
	staticData.LiteralNames = []string{
		"", "", "", "", "'IN'", "'IS'", "", "'ORDER'", "'BY'", "'ASC'", "'DESC'",
		"'WAS'", "'CHANGED'", "'AFTER'", "'BEFORE'", "'ON'", "'DURING'", "'FROM'",
		"'TO'", "'='", "'!='", "'~'", "'!~'", "'<'", "'<='", "'>'", "'>='",
		"'('", "')'", "'['", "']'", "','",
	}
	staticData.SymbolicNames = []string{
		"", "AND", "OR", "NOT", "IN", "IS", "EMPTY", "ORDER", "BY", "ASC", "DESC",
		"WAS", "CHANGED", "AFTER", "BEFORE", "ON", "DURING", "FROM", "TO", "EQ",
		"NEQ", "LIKE", "NOT_LIKE", "LT", "LTE", "GT", "GTE", "LPAREN", "RPAREN",
		"LBRACKET", "RBRACKET", "COMMA", "NUMBER", "DURATION", "QUOTED_STRING",
		"UNTERMINATED_STRING", "STRING", "WS", "ERROR_CHAR",
	}
	staticData.RuleNames = []string{
		"query", "expr", "clause", "field", "operand", "inOperand", "list",
		"function", "historyPredicate", "orderBy", "sortField", "value",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 38, 159, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 1, 0, 3, 0, 26, 8, 0, 1, 0, 3, 0, 29, 8, 0, 1, 0, 1,
		0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 1, 41, 8, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 5, 1, 49, 8, 1, 10, 1, 12, 1, 52, 9, 1, 1,
		2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 3, 2, 60, 8, 2, 1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 1, 2, 3, 2, 68, 8, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 3, 2, 75, 8,
		2, 1, 2, 1, 2, 1, 2, 3, 2, 80, 8, 2, 1, 2, 5, 2, 83, 8, 2, 10, 2, 12, 2,
		86, 9, 2, 1, 2, 1, 2, 1, 2, 5, 2, 91, 8, 2, 10, 2, 12, 2, 94, 9, 2, 3,
		2, 96, 8, 2, 1, 3, 1, 3, 1, 3, 1, 3, 3, 3, 102, 8, 3, 1, 4, 1, 4, 1, 4,
		3, 4, 107, 8, 4, 1, 5, 1, 5, 3, 5, 111, 8, 5, 1, 6, 1, 6, 1, 6, 1, 6, 5,
		6, 117, 8, 6, 10, 6, 12, 6, 120, 9, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1,
		7, 1, 7, 5, 7, 129, 8, 7, 10, 7, 12, 7, 132, 9, 7, 3, 7, 134, 8, 7, 1,
		7, 1, 7, 1, 8, 1, 8, 1, 8, 3, 8, 141, 8, 8, 1, 9, 1, 9, 1, 9, 1, 9, 1,
		9, 5, 9, 148, 8, 9, 10, 9, 12, 9, 151, 9, 9, 1, 10, 1, 10, 3, 10, 155,
		8, 10, 1, 11, 1, 11, 1, 11, 0, 1, 2, 12, 0, 2, 4, 6, 8, 10, 12, 14, 16,
		18, 20, 22, 0, 4, 1, 0, 19, 26, 2, 0, 8, 8, 13, 18, 1, 0, 9, 10, 2, 0,
		32, 34, 36, 36, 172, 0, 25, 1, 0, 0, 0, 2, 40, 1, 0, 0, 0, 4, 95, 1, 0,
		0, 0, 6, 97, 1, 0, 0, 0, 8, 106, 1, 0, 0, 0, 10, 110, 1, 0, 0, 0, 12, 112,
		1, 0, 0, 0, 14, 123, 1, 0, 0, 0, 16, 137, 1, 0, 0, 0, 18, 142, 1, 0, 0,
		0, 20, 152, 1, 0, 0, 0, 22, 156, 1, 0, 0, 0, 24, 26, 3, 2, 1, 0, 25, 24,
		1, 0, 0, 0, 25, 26, 1, 0, 0, 0, 26, 28, 1, 0, 0, 0, 27, 29, 3, 18, 9, 0,
		28, 27, 1, 0, 0, 0, 28, 29, 1, 0, 0, 0, 29, 30, 1, 0, 0, 0, 30, 31, 5,
		0, 0, 1, 31, 1, 1, 0, 0, 0, 32, 33, 6, 1, -1, 0, 33, 34, 5, 27, 0, 0, 34,
		35, 3, 2, 1, 0, 35, 36, 5, 28, 0, 0, 36, 41, 1, 0, 0, 0, 37, 38, 5, 3,
		0, 0, 38, 41, 3, 2, 1, 4, 39, 41, 3, 4, 2, 0, 40, 32, 1, 0, 0, 0, 40, 37,
		1, 0, 0, 0, 40, 39, 1, 0, 0, 0, 41, 50, 1, 0, 0, 0, 42, 43, 10, 3, 0, 0,
		43, 44, 5, 1, 0, 0, 44, 49, 3, 2, 1, 4, 45, 46, 10, 2, 0, 0, 46, 47, 5,
		2, 0, 0, 47, 49, 3, 2, 1, 3, 48, 42, 1, 0, 0, 0, 48, 45, 1, 0, 0, 0, 49,
		52, 1, 0, 0, 0, 50, 48, 1, 0, 0, 0, 50, 51, 1, 0, 0, 0, 51, 3, 1, 0, 0,
		0, 52, 50, 1, 0, 0, 0, 53, 54, 3, 6, 3, 0, 54, 55, 7, 0, 0, 0, 55, 56,
		3, 8, 4, 0, 56, 96, 1, 0, 0, 0, 57, 59, 3, 6, 3, 0, 58, 60, 5, 3, 0, 0,
		59, 58, 1, 0, 0, 0, 59, 60, 1, 0, 0, 0, 60, 61, 1, 0, 0, 0, 61, 62, 5,
		4, 0, 0, 62, 63, 3, 10, 5, 0, 63, 96, 1, 0, 0, 0, 64, 65, 3, 6, 3, 0, 65,
		67, 5, 5, 0, 0, 66, 68, 5, 3, 0, 0, 67, 66, 1, 0, 0, 0, 67, 68, 1, 0, 0,
		0, 68, 69, 1, 0, 0, 0, 69, 70, 5, 6, 0, 0, 70, 96, 1, 0, 0, 0, 71, 72,
		3, 6, 3, 0, 72, 74, 5, 11, 0, 0, 73, 75, 5, 3, 0, 0, 74, 73, 1, 0, 0, 0,
		74, 75, 1, 0, 0, 0, 75, 79, 1, 0, 0, 0, 76, 77, 5, 4, 0, 0, 77, 80, 3,
		10, 5, 0, 78, 80, 3, 8, 4, 0, 79, 76, 1, 0, 0, 0, 79, 78, 1, 0, 0, 0, 80,
		84, 1, 0, 0, 0, 81, 83, 3, 16, 8, 0, 82, 81, 1, 0, 0, 0, 83, 86, 1, 0,
		0, 0, 84, 82, 1, 0, 0, 0, 84, 85, 1, 0, 0, 0, 85, 96, 1, 0, 0, 0, 86, 84,
		1, 0, 0, 0, 87, 88, 3, 6, 3, 0, 88, 92, 5, 12, 0, 0, 89, 91, 3, 16, 8,
		0, 90, 89, 1, 0, 0, 0, 91, 94, 1, 0, 0, 0, 92, 90, 1, 0, 0, 0, 92, 93,
		1, 0, 0, 0, 93, 96, 1, 0, 0, 0, 94, 92, 1, 0, 0, 0, 95, 53, 1, 0, 0, 0,
		95, 57, 1, 0, 0, 0, 95, 64, 1, 0, 0, 0, 95, 71, 1, 0, 0, 0, 95, 87, 1,
		0, 0, 0, 96, 5, 1, 0, 0, 0, 97, 101, 3, 22, 11, 0, 98, 99, 5, 29, 0, 0,
		99, 100, 5, 32, 0, 0, 100, 102, 5, 30, 0, 0, 101, 98, 1, 0, 0, 0, 101,
		102, 1, 0, 0, 0, 102, 7, 1, 0, 0, 0, 103, 107, 5, 6, 0, 0, 104, 107, 3,
		14, 7, 0, 105, 107, 3, 22, 11, 0, 106, 103, 1, 0, 0, 0, 106, 104, 1, 0,
		0, 0, 106, 105, 1, 0, 0, 0, 107, 9, 1, 0, 0, 0, 108, 111, 3, 12, 6, 0,
		109, 111, 3, 14, 7, 0, 110, 108, 1, 0, 0, 0, 110, 109, 1, 0, 0, 0, 111,
		11, 1, 0, 0, 0, 112, 113, 5, 27, 0, 0, 113, 118, 3, 8, 4, 0, 114, 115,
		5, 31, 0, 0, 115, 117, 3, 8, 4, 0, 116, 114, 1, 0, 0, 0, 117, 120, 1, 0,
		0, 0, 118, 116, 1, 0, 0, 0, 118, 119, 1, 0, 0, 0, 119, 121, 1, 0, 0, 0,
		120, 118, 1, 0, 0, 0, 121, 122, 5, 28, 0, 0, 122, 13, 1, 0, 0, 0, 123,
		124, 5, 36, 0, 0, 124, 133, 5, 27, 0, 0, 125, 130, 3, 22, 11, 0, 126, 127,
		5, 31, 0, 0, 127, 129, 3, 22, 11, 0, 128, 126, 1, 0, 0, 0, 129, 132, 1,
		0, 0, 0, 130, 128, 1, 0, 0, 0, 130, 131, 1, 0, 0, 0, 131, 134, 1, 0, 0,
		0, 132, 130, 1, 0, 0, 0, 133, 125, 1, 0, 0, 0, 133, 134, 1, 0, 0, 0, 134,
		135, 1, 0, 0, 0, 135, 136, 5, 28, 0, 0, 136, 15, 1, 0, 0, 0, 137, 140,
		7, 1, 0, 0, 138, 141, 3, 8, 4, 0, 139, 141, 3, 12, 6, 0, 140, 138, 1, 0,
		0, 0, 140, 139, 1, 0, 0, 0, 141, 17, 1, 0, 0, 0, 142, 143, 5, 7, 0, 0,
		143, 144, 5, 8, 0, 0, 144, 149, 3, 20, 10, 0, 145, 146, 5, 31, 0, 0, 146,
		148, 3, 20, 10, 0, 147, 145, 1, 0, 0, 0, 148, 151, 1, 0, 0, 0, 149, 147,
		1, 0, 0, 0, 149, 150, 1, 0, 0, 0, 150, 19, 1, 0, 0, 0, 151, 149, 1, 0,
		0, 0, 152, 154, 3, 6, 3, 0, 153, 155, 7, 2, 0, 0, 154, 153, 1, 0, 0, 0,
		154, 155, 1, 0, 0, 0, 155, 21, 1, 0, 0, 0, 156, 157, 7, 3, 0, 0, 157, 23,
		1, 0, 0, 0, 21, 25, 28, 40, 48, 50, 59, 67, 74, 79, 84, 92, 95, 101, 106,
		110, 118, 130, 133, 140, 149, 154,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// AQLParserInit initializes any static state used to implement AQLParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewAQLParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func AQLParserInit() {
	staticData := &AQLParserStaticData
	staticData.once.Do(aqlParserInit)
}

// NewAQLParser produces a new parser instance for the optional input antlr.TokenStream.
func NewAQLParser(input antlr.TokenStream) *AQLParser {
	AQLParserInit()
	this := new(AQLParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &AQLParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "AQL.g4"

	return this
}

// AQLParser tokens.
const (
	AQLParserEOF                 = antlr.TokenEOF
	AQLParserAND                 = 1
	AQLParserOR                  = 2
	AQLParserNOT                 = 3
	AQLParserIN                  = 4
	AQLParserIS                  = 5
	AQLParserEMPTY               = 6
	AQLParserORDER               = 7
	AQLParserBY                  = 8
	AQLParserASC                 = 9
	AQLParserDESC                = 10
	AQLParserWAS                 = 11
	AQLParserCHANGED             = 12
	AQLParserAFTER               = 13
	AQLParserBEFORE              = 14
	AQLParserON                  = 15
	AQLParserDURING              = 16
	AQLParserFROM                = 17
	AQLParserTO                  = 18
	AQLParserEQ                  = 19
	AQLParserNEQ                 = 20
	AQLParserLIKE                = 21
	AQLParserNOT_LIKE            = 22
	AQLParserLT                  = 23
	AQLParserLTE                 = 24
	AQLParserGT                  = 25
	AQLParserGTE                 = 26
	AQLParserLPAREN              = 27
	AQLParserRPAREN              = 28
	AQLParserLBRACKET            = 29
	AQLParserRBRACKET            = 30
	AQLParserCOMMA               = 31
	AQLParserNUMBER              = 32
	AQLParserDURATION            = 33
	AQLParserQUOTED_STRING       = 34
	AQLParserUNTERMINATED_STRING = 35
	AQLParserSTRING              = 36
	AQLParserWS                  = 37
	AQLParserERROR_CHAR          = 38
)

// AQLParser rules.
const (
	AQLParserRULE_query            = 0
	AQLParserRULE_expr             = 1
	AQLParserRULE_clause           = 2
	AQLParserRULE_field            = 3
	AQLParserRULE_operand          = 4
	AQLParserRULE_inOperand        = 5
	AQLParserRULE_list             = 6
	AQLParserRULE_function         = 7
	AQLParserRULE_historyPredicate = 8
	AQLParserRULE_orderBy          = 9
	AQLParserRULE_sortField        = 10
	AQLParserRULE_value            = 11
)

// IQueryContext is an interface to support dynamic dispatch.
type IQueryContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EOF() antlr.TerminalNode
	Expr() IExprContext
	OrderBy() IOrderByContext

	// IsQueryContext differentiates from other interfaces.
	IsQueryContext()
}

type QueryContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyQueryContext() *QueryContext {
	var p = new(QueryContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_query
	return p
}

func InitEmptyQueryContext(p *QueryContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_query
}

func (*QueryContext) IsQueryContext() {}

func NewQueryContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *QueryContext {
	var p = new(QueryContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_query

	return p
}

func (s *QueryContext) GetParser() antlr.Parser { return s.parser }

func (s *QueryContext) EOF() antlr.TerminalNode {
	return s.GetToken(AQLParserEOF, 0)
}

func (s *QueryContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *QueryContext) OrderBy() IOrderByContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOrderByContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOrderByContext)
}

func (s *QueryContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *QueryContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *QueryContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitQuery(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Query() (localctx IQueryContext) {
	localctx = NewQueryContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, AQLParserRULE_query)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(25)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&98918465544) != 0 {
		{
			p.SetState(24)
			p.expr(0)
		}

	}
	p.SetState(28)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == AQLParserORDER {
		{
			p.SetState(27)
			p.OrderBy()
		}

	}
	{
		p.SetState(30)
		p.Match(AQLParserEOF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IExprContext is an interface to support dynamic dispatch.
type IExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsExprContext differentiates from other interfaces.
	IsExprContext()
}

type ExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExprContext() *ExprContext {
	var p = new(ExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_expr
	return p
}

func InitEmptyExprContext(p *ExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_expr
}

func (*ExprContext) IsExprContext() {}

func NewExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExprContext {
	var p = new(ExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_expr

	return p
}

func (s *ExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ExprContext) CopyAll(ctx *ExprContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *ExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type NotExprContext struct {
	ExprContext
}

func NewNotExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *NotExprContext {
	var p = new(NotExprContext)

	InitEmptyExprContext(&p.ExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*ExprContext))

	return p
}

func (s *NotExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NotExprContext) NOT() antlr.TerminalNode {
	return s.GetToken(AQLParserNOT, 0)
}

func (s *NotExprContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *NotExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitNotExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type OrExprContext struct {
	ExprContext
}

func NewOrExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OrExprContext {
	var p = new(OrExprContext)

	InitEmptyExprContext(&p.ExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*ExprContext))

	return p
}

func (s *OrExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OrExprContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *OrExprContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *OrExprContext) OR() antlr.TerminalNode {
	return s.GetToken(AQLParserOR, 0)
}

func (s *OrExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitOrExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ClauseExprContext struct {
	ExprContext
}

func NewClauseExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ClauseExprContext {
	var p = new(ClauseExprContext)

	InitEmptyExprContext(&p.ExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*ExprContext))

	return p
}

func (s *ClauseExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ClauseExprContext) Clause() IClauseContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IClauseContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IClauseContext)
}

func (s *ClauseExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitClauseExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type ParenExprContext struct {
	ExprContext
}

func NewParenExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ParenExprContext {
	var p = new(ParenExprContext)

	InitEmptyExprContext(&p.ExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*ExprContext))

	return p
}

func (s *ParenExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParenExprContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserLPAREN, 0)
}

func (s *ParenExprContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ParenExprContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserRPAREN, 0)
}

func (s *ParenExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitParenExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

type AndExprContext struct {
	ExprContext
}

func NewAndExprContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AndExprContext {
	var p = new(AndExprContext)

	InitEmptyExprContext(&p.ExprContext)
	p.parser = parser
	p.CopyAll(ctx.(*ExprContext))

	return p
}

func (s *AndExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AndExprContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *AndExprContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *AndExprContext) AND() antlr.TerminalNode {
	return s.GetToken(AQLParserAND, 0)
}

func (s *AndExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitAndExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Expr() (localctx IExprContext) {
	return p.expr(0)
}

func (p *AQLParser) expr(_p int) (localctx IExprContext) {
	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()

	_parentState := p.GetState()
	localctx = NewExprContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IExprContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 2
	p.EnterRecursionRule(localctx, 2, AQLParserRULE_expr, _p)
	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(40)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case AQLParserLPAREN:
		localctx = NewParenExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(33)
			p.Match(AQLParserLPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(34)
			p.expr(0)
		}
		{
			p.SetState(35)
			p.Match(AQLParserRPAREN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case AQLParserNOT:
		localctx = NewNotExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(37)
			p.Match(AQLParserNOT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(38)
			p.expr(4)
		}

	case AQLParserNUMBER, AQLParserDURATION, AQLParserQUOTED_STRING, AQLParserSTRING:
		localctx = NewClauseExprContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(39)
			p.Clause()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(50)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 4, p.GetParserRuleContext())
	if p.HasError() {
		goto errorExit
	}
	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(48)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}

			switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 3, p.GetParserRuleContext()) {
			case 1:
				localctx = NewAndExprContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, AQLParserRULE_expr)
				p.SetState(42)

				if !(p.Precpred(p.GetParserRuleContext(), 3)) {
					p.SetError(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 3)", ""))
					goto errorExit
				}
				{
					p.SetState(43)
					p.Match(AQLParserAND)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}
				{
					p.SetState(44)
					p.expr(4)
				}

			case 2:
				localctx = NewOrExprContext(p, NewExprContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, AQLParserRULE_expr)
				p.SetState(45)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					p.SetError(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
					goto errorExit
				}
				{
					p.SetState(46)
					p.Match(AQLParserOR)
					if p.HasError() {
						// Recognition error - abort rule
						goto errorExit
					}
				}
				{
					p.SetState(47)
					p.expr(3)
				}

			case antlr.ATNInvalidAltNumber:
				goto errorExit
			}

		}
		p.SetState(52)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 4, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.UnrollRecursionContexts(_parentctx)
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IClauseContext is an interface to support dynamic dispatch.
type IClauseContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsClauseContext differentiates from other interfaces.
	IsClauseContext()
}

type ClauseContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyClauseContext() *ClauseContext {
	var p = new(ClauseContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_clause
	return p
}

func InitEmptyClauseContext(p *ClauseContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_clause
}

func (*ClauseContext) IsClauseContext() {}

func NewClauseContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ClauseContext {
	var p = new(ClauseContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_clause

	return p
}

func (s *ClauseContext) GetParser() antlr.Parser { return s.parser }

func (s *ClauseContext) CopyAll(ctx *ClauseContext) {
	s.CopyFrom(&ctx.BaseParserRuleContext)
}

func (s *ClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ClauseContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type IsEmptyClauseContext struct {
	ClauseContext
}

func NewIsEmptyClauseContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *IsEmptyClauseContext {
	var p = new(IsEmptyClauseContext)

	InitEmptyClauseContext(&p.ClauseContext)
	p.parser = parser
	p.CopyAll(ctx.(*ClauseContext))

	return p
}

func (s *IsEmptyClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IsEmptyClauseContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *IsEmptyClauseContext) IS() antlr.TerminalNode {
	return s.GetToken(AQLParserIS, 0)
}

func (s *IsEmptyClauseContext) EMPTY() antlr.TerminalNode {
	return s.GetToken(AQLParserEMPTY, 0)
}

func (s *IsEmptyClauseContext) NOT() antlr.TerminalNode {
	return s.GetToken(AQLParserNOT, 0)
}

func (s *IsEmptyClauseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitIsEmptyClause(s)

	default:
		return t.VisitChildren(s)
	}
}

type ChangedClauseContext struct {
	ClauseContext
}

func NewChangedClauseContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *ChangedClauseContext {
	var p = new(ChangedClauseContext)

	InitEmptyClauseContext(&p.ClauseContext)
	p.parser = parser
	p.CopyAll(ctx.(*ClauseContext))

	return p
}

func (s *ChangedClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ChangedClauseContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *ChangedClauseContext) CHANGED() antlr.TerminalNode {
	return s.GetToken(AQLParserCHANGED, 0)
}

func (s *ChangedClauseContext) AllHistoryPredicate() []IHistoryPredicateContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHistoryPredicateContext); ok {
			len++
		}
	}

	tst := make([]IHistoryPredicateContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHistoryPredicateContext); ok {
			tst[i] = t.(IHistoryPredicateContext)
			i++
		}
	}

	return tst
}

func (s *ChangedClauseContext) HistoryPredicate(i int) IHistoryPredicateContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHistoryPredicateContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHistoryPredicateContext)
}

func (s *ChangedClauseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitChangedClause(s)

	default:
		return t.VisitChildren(s)
	}
}

type CompareClauseContext struct {
	ClauseContext
	op antlr.Token
}

func NewCompareClauseContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *CompareClauseContext {
	var p = new(CompareClauseContext)

	InitEmptyClauseContext(&p.ClauseContext)
	p.parser = parser
	p.CopyAll(ctx.(*ClauseContext))

	return p
}

func (s *CompareClauseContext) GetOp() antlr.Token { return s.op }

func (s *CompareClauseContext) SetOp(v antlr.Token) { s.op = v }

func (s *CompareClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CompareClauseContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *CompareClauseContext) Operand() IOperandContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperandContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperandContext)
}

func (s *CompareClauseContext) EQ() antlr.TerminalNode {
	return s.GetToken(AQLParserEQ, 0)
}

func (s *CompareClauseContext) NEQ() antlr.TerminalNode {
	return s.GetToken(AQLParserNEQ, 0)
}

func (s *CompareClauseContext) LIKE() antlr.TerminalNode {
	return s.GetToken(AQLParserLIKE, 0)
}

func (s *CompareClauseContext) NOT_LIKE() antlr.TerminalNode {
	return s.GetToken(AQLParserNOT_LIKE, 0)
}

func (s *CompareClauseContext) LT() antlr.TerminalNode {
	return s.GetToken(AQLParserLT, 0)
}

func (s *CompareClauseContext) LTE() antlr.TerminalNode {
	return s.GetToken(AQLParserLTE, 0)
}

func (s *CompareClauseContext) GT() antlr.TerminalNode {
	return s.GetToken(AQLParserGT, 0)
}

func (s *CompareClauseContext) GTE() antlr.TerminalNode {
	return s.GetToken(AQLParserGTE, 0)
}

func (s *CompareClauseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitCompareClause(s)

	default:
		return t.VisitChildren(s)
	}
}

type InClauseContext struct {
	ClauseContext
}

func NewInClauseContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InClauseContext {
	var p = new(InClauseContext)

	InitEmptyClauseContext(&p.ClauseContext)
	p.parser = parser
	p.CopyAll(ctx.(*ClauseContext))

	return p
}

func (s *InClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InClauseContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *InClauseContext) IN() antlr.TerminalNode {
	return s.GetToken(AQLParserIN, 0)
}

func (s *InClauseContext) InOperand() IInOperandContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInOperandContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInOperandContext)
}

func (s *InClauseContext) NOT() antlr.TerminalNode {
	return s.GetToken(AQLParserNOT, 0)
}

func (s *InClauseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitInClause(s)

	default:
		return t.VisitChildren(s)
	}
}

type WasClauseContext struct {
	ClauseContext
}

func NewWasClauseContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *WasClauseContext {
	var p = new(WasClauseContext)

	InitEmptyClauseContext(&p.ClauseContext)
	p.parser = parser
	p.CopyAll(ctx.(*ClauseContext))

	return p
}

func (s *WasClauseContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *WasClauseContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *WasClauseContext) WAS() antlr.TerminalNode {
	return s.GetToken(AQLParserWAS, 0)
}

func (s *WasClauseContext) IN() antlr.TerminalNode {
	return s.GetToken(AQLParserIN, 0)
}

func (s *WasClauseContext) InOperand() IInOperandContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IInOperandContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IInOperandContext)
}

func (s *WasClauseContext) Operand() IOperandContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperandContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperandContext)
}

func (s *WasClauseContext) NOT() antlr.TerminalNode {
	return s.GetToken(AQLParserNOT, 0)
}

func (s *WasClauseContext) AllHistoryPredicate() []IHistoryPredicateContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHistoryPredicateContext); ok {
			len++
		}
	}

	tst := make([]IHistoryPredicateContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHistoryPredicateContext); ok {
			tst[i] = t.(IHistoryPredicateContext)
			i++
		}
	}

	return tst
}

func (s *WasClauseContext) HistoryPredicate(i int) IHistoryPredicateContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHistoryPredicateContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHistoryPredicateContext)
}

func (s *WasClauseContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitWasClause(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Clause() (localctx IClauseContext) {
	localctx = NewClauseContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, AQLParserRULE_clause)
	var _la int

	var _alt int

	p.SetState(95)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 11, p.GetParserRuleContext()) {
	case 1:
		localctx = NewCompareClauseContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(53)
			p.Field()
		}
		{
			p.SetState(54)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*CompareClauseContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&133693440) != 0) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*CompareClauseContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(55)
			p.Operand()
		}

	case 2:
		localctx = NewInClauseContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(57)
			p.Field()
		}
		p.SetState(59)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		if _la == AQLParserNOT {
			{
				p.SetState(58)
				p.Match(AQLParserNOT)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		{
			p.SetState(61)
			p.Match(AQLParserIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(62)
			p.InOperand()
		}

	case 3:
		localctx = NewIsEmptyClauseContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(64)
			p.Field()
		}
		{
			p.SetState(65)
			p.Match(AQLParserIS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(67)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		if _la == AQLParserNOT {
			{
				p.SetState(66)
				p.Match(AQLParserNOT)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		{
			p.SetState(69)
			p.Match(AQLParserEMPTY)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 4:
		localctx = NewWasClauseContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(71)
			p.Field()
		}
		{
			p.SetState(72)
			p.Match(AQLParserWAS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(74)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		if _la == AQLParserNOT {
			{
				p.SetState(73)
				p.Match(AQLParserNOT)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}

		}
		p.SetState(79)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}

		switch p.GetTokenStream().LA(1) {
		case AQLParserIN:
			{
				p.SetState(76)
				p.Match(AQLParserIN)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			{
				p.SetState(77)
				p.InOperand()
			}

		case AQLParserEMPTY, AQLParserNUMBER, AQLParserDURATION, AQLParserQUOTED_STRING, AQLParserSTRING:
			{
				p.SetState(78)
				p.Operand()
			}

		default:
			p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
			goto errorExit
		}
		p.SetState(84)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 9, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(81)
					p.HistoryPredicate()
				}

			}
			p.SetState(86)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 9, p.GetParserRuleContext())
			if p.HasError() {
				goto errorExit
			}
		}

	case 5:
		localctx = NewChangedClauseContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(87)
			p.Field()
		}
		{
			p.SetState(88)
			p.Match(AQLParserCHANGED)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(92)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 10, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(89)
					p.HistoryPredicate()
				}

			}
			p.SetState(94)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 10, p.GetParserRuleContext())
			if p.HasError() {
				goto errorExit
			}
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IFieldContext is an interface to support dynamic dispatch.
type IFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Value() IValueContext
	LBRACKET() antlr.TerminalNode
	NUMBER() antlr.TerminalNode
	RBRACKET() antlr.TerminalNode

	// IsFieldContext differentiates from other interfaces.
	IsFieldContext()
}

type FieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFieldContext() *FieldContext {
	var p = new(FieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_field
	return p
}

func InitEmptyFieldContext(p *FieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_field
}

func (*FieldContext) IsFieldContext() {}

func NewFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FieldContext {
	var p = new(FieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_field

	return p
}

func (s *FieldContext) GetParser() antlr.Parser { return s.parser }

func (s *FieldContext) Value() IValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueContext)
}

func (s *FieldContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(AQLParserLBRACKET, 0)
}

func (s *FieldContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(AQLParserNUMBER, 0)
}

func (s *FieldContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(AQLParserRBRACKET, 0)
}

func (s *FieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Field() (localctx IFieldContext) {
	localctx = NewFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, AQLParserRULE_field)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(97)
		p.Value()
	}
	p.SetState(101)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == AQLParserLBRACKET {
		{
			p.SetState(98)
			p.Match(AQLParserLBRACKET)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(99)
			p.Match(AQLParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(100)
			p.Match(AQLParserRBRACKET)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IOperandContext is an interface to support dynamic dispatch.
type IOperandContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EMPTY() antlr.TerminalNode
	Function() IFunctionContext
	Value() IValueContext

	// IsOperandContext differentiates from other interfaces.
	IsOperandContext()
}

type OperandContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperandContext() *OperandContext {
	var p = new(OperandContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_operand
	return p
}

func InitEmptyOperandContext(p *OperandContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_operand
}

func (*OperandContext) IsOperandContext() {}

func NewOperandContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperandContext {
	var p = new(OperandContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_operand

	return p
}

func (s *OperandContext) GetParser() antlr.Parser { return s.parser }

func (s *OperandContext) EMPTY() antlr.TerminalNode {
	return s.GetToken(AQLParserEMPTY, 0)
}

func (s *OperandContext) Function() IFunctionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionContext)
}

func (s *OperandContext) Value() IValueContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueContext)
}

func (s *OperandContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperandContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OperandContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitOperand(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Operand() (localctx IOperandContext) {
	localctx = NewOperandContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, AQLParserRULE_operand)
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 13, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(103)
			p.Match(AQLParserEMPTY)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(104)
			p.Function()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(105)
			p.Value()
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IInOperandContext is an interface to support dynamic dispatch.
type IInOperandContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	List() IListContext
	Function() IFunctionContext

	// IsInOperandContext differentiates from other interfaces.
	IsInOperandContext()
}

type InOperandContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyInOperandContext() *InOperandContext {
	var p = new(InOperandContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_inOperand
	return p
}

func InitEmptyInOperandContext(p *InOperandContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_inOperand
}

func (*InOperandContext) IsInOperandContext() {}

func NewInOperandContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *InOperandContext {
	var p = new(InOperandContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_inOperand

	return p
}

func (s *InOperandContext) GetParser() antlr.Parser { return s.parser }

func (s *InOperandContext) List() IListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IListContext)
}

func (s *InOperandContext) Function() IFunctionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFunctionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFunctionContext)
}

func (s *InOperandContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InOperandContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *InOperandContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitInOperand(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) InOperand() (localctx IInOperandContext) {
	localctx = NewInOperandContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, AQLParserRULE_inOperand)
	p.SetState(110)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case AQLParserLPAREN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(108)
			p.List()
		}

	case AQLParserSTRING:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(109)
			p.Function()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IListContext is an interface to support dynamic dispatch.
type IListContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	LPAREN() antlr.TerminalNode
	AllOperand() []IOperandContext
	Operand(i int) IOperandContext
	RPAREN() antlr.TerminalNode
	AllCOMMA() []antlr.TerminalNode
	COMMA(i int) antlr.TerminalNode

	// IsListContext differentiates from other interfaces.
	IsListContext()
}

type ListContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyListContext() *ListContext {
	var p = new(ListContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_list
	return p
}

func InitEmptyListContext(p *ListContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_list
}

func (*ListContext) IsListContext() {}

func NewListContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ListContext {
	var p = new(ListContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_list

	return p
}

func (s *ListContext) GetParser() antlr.Parser { return s.parser }

func (s *ListContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserLPAREN, 0)
}

func (s *ListContext) AllOperand() []IOperandContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IOperandContext); ok {
			len++
		}
	}

	tst := make([]IOperandContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IOperandContext); ok {
			tst[i] = t.(IOperandContext)
			i++
		}
	}

	return tst
}

func (s *ListContext) Operand(i int) IOperandContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperandContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperandContext)
}

func (s *ListContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserRPAREN, 0)
}

func (s *ListContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(AQLParserCOMMA)
}

func (s *ListContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(AQLParserCOMMA, i)
}

func (s *ListContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ListContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ListContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitList(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) List() (localctx IListContext) {
	localctx = NewListContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, AQLParserRULE_list)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(112)
		p.Match(AQLParserLPAREN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(113)
		p.Operand()
	}
	p.SetState(118)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == AQLParserCOMMA {
		{
			p.SetState(114)
			p.Match(AQLParserCOMMA)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(115)
			p.Operand()
		}

		p.SetState(120)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(121)
		p.Match(AQLParserRPAREN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IFunctionContext is an interface to support dynamic dispatch.
type IFunctionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	STRING() antlr.TerminalNode
	LPAREN() antlr.TerminalNode
	RPAREN() antlr.TerminalNode
	AllValue() []IValueContext
	Value(i int) IValueContext
	AllCOMMA() []antlr.TerminalNode
	COMMA(i int) antlr.TerminalNode

	// IsFunctionContext differentiates from other interfaces.
	IsFunctionContext()
}

type FunctionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFunctionContext() *FunctionContext {
	var p = new(FunctionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_function
	return p
}

func InitEmptyFunctionContext(p *FunctionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_function
}

func (*FunctionContext) IsFunctionContext() {}

func NewFunctionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FunctionContext {
	var p = new(FunctionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_function

	return p
}

func (s *FunctionContext) GetParser() antlr.Parser { return s.parser }

func (s *FunctionContext) STRING() antlr.TerminalNode {
	return s.GetToken(AQLParserSTRING, 0)
}

func (s *FunctionContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserLPAREN, 0)
}

func (s *FunctionContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(AQLParserRPAREN, 0)
}

func (s *FunctionContext) AllValue() []IValueContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IValueContext); ok {
			len++
		}
	}

	tst := make([]IValueContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IValueContext); ok {
			tst[i] = t.(IValueContext)
			i++
		}
	}

	return tst
}

func (s *FunctionContext) Value(i int) IValueContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValueContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValueContext)
}

func (s *FunctionContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(AQLParserCOMMA)
}

func (s *FunctionContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(AQLParserCOMMA, i)
}

func (s *FunctionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FunctionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FunctionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitFunction(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Function() (localctx IFunctionContext) {
	localctx = NewFunctionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, AQLParserRULE_function)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(123)
		p.Match(AQLParserSTRING)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(124)
		p.Match(AQLParserLPAREN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(133)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&98784247808) != 0 {
		{
			p.SetState(125)
			p.Value()
		}
		p.SetState(130)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == AQLParserCOMMA {
			{
				p.SetState(126)
				p.Match(AQLParserCOMMA)
				if p.HasError() {
					// Recognition error - abort rule
					goto errorExit
				}
			}
			{
				p.SetState(127)
				p.Value()
			}

			p.SetState(132)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}

	}
	{
		p.SetState(135)
		p.Match(AQLParserRPAREN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHistoryPredicateContext is an interface to support dynamic dispatch.
type IHistoryPredicateContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetOp returns the op token.
	GetOp() antlr.Token

	// SetOp sets the op token.
	SetOp(antlr.Token)

	// Getter signatures
	AFTER() antlr.TerminalNode
	BEFORE() antlr.TerminalNode
	ON() antlr.TerminalNode
	DURING() antlr.TerminalNode
	BY() antlr.TerminalNode
	FROM() antlr.TerminalNode
	TO() antlr.TerminalNode
	Operand() IOperandContext
	List() IListContext

	// IsHistoryPredicateContext differentiates from other interfaces.
	IsHistoryPredicateContext()
}

type HistoryPredicateContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
	op     antlr.Token
}

func NewEmptyHistoryPredicateContext() *HistoryPredicateContext {
	var p = new(HistoryPredicateContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_historyPredicate
	return p
}

func InitEmptyHistoryPredicateContext(p *HistoryPredicateContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_historyPredicate
}

func (*HistoryPredicateContext) IsHistoryPredicateContext() {}

func NewHistoryPredicateContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HistoryPredicateContext {
	var p = new(HistoryPredicateContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_historyPredicate

	return p
}

func (s *HistoryPredicateContext) GetParser() antlr.Parser { return s.parser }

func (s *HistoryPredicateContext) GetOp() antlr.Token { return s.op }

func (s *HistoryPredicateContext) SetOp(v antlr.Token) { s.op = v }

func (s *HistoryPredicateContext) AFTER() antlr.TerminalNode {
	return s.GetToken(AQLParserAFTER, 0)
}

func (s *HistoryPredicateContext) BEFORE() antlr.TerminalNode {
	return s.GetToken(AQLParserBEFORE, 0)
}

func (s *HistoryPredicateContext) ON() antlr.TerminalNode {
	return s.GetToken(AQLParserON, 0)
}

func (s *HistoryPredicateContext) DURING() antlr.TerminalNode {
	return s.GetToken(AQLParserDURING, 0)
}

func (s *HistoryPredicateContext) BY() antlr.TerminalNode {
	return s.GetToken(AQLParserBY, 0)
}

func (s *HistoryPredicateContext) FROM() antlr.TerminalNode {
	return s.GetToken(AQLParserFROM, 0)
}

func (s *HistoryPredicateContext) TO() antlr.TerminalNode {
	return s.GetToken(AQLParserTO, 0)
}

func (s *HistoryPredicateContext) Operand() IOperandContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IOperandContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IOperandContext)
}

func (s *HistoryPredicateContext) List() IListContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IListContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IListContext)
}

func (s *HistoryPredicateContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HistoryPredicateContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HistoryPredicateContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitHistoryPredicate(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) HistoryPredicate() (localctx IHistoryPredicateContext) {
	localctx = NewHistoryPredicateContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, AQLParserRULE_historyPredicate)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(137)

		var _lt = p.GetTokenStream().LT(1)

		localctx.(*HistoryPredicateContext).op = _lt

		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&516352) != 0) {
			var _ri = p.GetErrorHandler().RecoverInline(p)

			localctx.(*HistoryPredicateContext).op = _ri
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	p.SetState(140)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case AQLParserEMPTY, AQLParserNUMBER, AQLParserDURATION, AQLParserQUOTED_STRING, AQLParserSTRING:
		{
			p.SetState(138)
			p.Operand()
		}

	case AQLParserLPAREN:
		{
			p.SetState(139)
			p.List()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IOrderByContext is an interface to support dynamic dispatch.
type IOrderByContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ORDER() antlr.TerminalNode
	BY() antlr.TerminalNode
	AllSortField() []ISortFieldContext
	SortField(i int) ISortFieldContext
	AllCOMMA() []antlr.TerminalNode
	COMMA(i int) antlr.TerminalNode

	// IsOrderByContext differentiates from other interfaces.
	IsOrderByContext()
}

type OrderByContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOrderByContext() *OrderByContext {
	var p = new(OrderByContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_orderBy
	return p
}

func InitEmptyOrderByContext(p *OrderByContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_orderBy
}

func (*OrderByContext) IsOrderByContext() {}

func NewOrderByContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OrderByContext {
	var p = new(OrderByContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_orderBy

	return p
}

func (s *OrderByContext) GetParser() antlr.Parser { return s.parser }

func (s *OrderByContext) ORDER() antlr.TerminalNode {
	return s.GetToken(AQLParserORDER, 0)
}

func (s *OrderByContext) BY() antlr.TerminalNode {
	return s.GetToken(AQLParserBY, 0)
}

func (s *OrderByContext) AllSortField() []ISortFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISortFieldContext); ok {
			len++
		}
	}

	tst := make([]ISortFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISortFieldContext); ok {
			tst[i] = t.(ISortFieldContext)
			i++
		}
	}

	return tst
}

func (s *OrderByContext) SortField(i int) ISortFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISortFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISortFieldContext)
}

func (s *OrderByContext) AllCOMMA() []antlr.TerminalNode {
	return s.GetTokens(AQLParserCOMMA)
}

func (s *OrderByContext) COMMA(i int) antlr.TerminalNode {
	return s.GetToken(AQLParserCOMMA, i)
}

func (s *OrderByContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OrderByContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *OrderByContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitOrderBy(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) OrderBy() (localctx IOrderByContext) {
	localctx = NewOrderByContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, AQLParserRULE_orderBy)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(142)
		p.Match(AQLParserORDER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(143)
		p.Match(AQLParserBY)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(144)
		p.SortField()
	}
	p.SetState(149)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == AQLParserCOMMA {
		{
			p.SetState(145)
			p.Match(AQLParserCOMMA)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(146)
			p.SortField()
		}

		p.SetState(151)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ISortFieldContext is an interface to support dynamic dispatch.
type ISortFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Field() IFieldContext
	ASC() antlr.TerminalNode
	DESC() antlr.TerminalNode

	// IsSortFieldContext differentiates from other interfaces.
	IsSortFieldContext()
}

type SortFieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySortFieldContext() *SortFieldContext {
	var p = new(SortFieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_sortField
	return p
}

func InitEmptySortFieldContext(p *SortFieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_sortField
}

func (*SortFieldContext) IsSortFieldContext() {}

func NewSortFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SortFieldContext {
	var p = new(SortFieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_sortField

	return p
}

func (s *SortFieldContext) GetParser() antlr.Parser { return s.parser }

func (s *SortFieldContext) Field() IFieldContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFieldContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFieldContext)
}

func (s *SortFieldContext) ASC() antlr.TerminalNode {
	return s.GetToken(AQLParserASC, 0)
}

func (s *SortFieldContext) DESC() antlr.TerminalNode {
	return s.GetToken(AQLParserDESC, 0)
}

func (s *SortFieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SortFieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SortFieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitSortField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) SortField() (localctx ISortFieldContext) {
	localctx = NewSortFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, AQLParserRULE_sortField)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(152)
		p.Field()
	}
	p.SetState(154)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == AQLParserASC || _la == AQLParserDESC {
		{
			p.SetState(153)
			_la = p.GetTokenStream().LA(1)

			if !(_la == AQLParserASC || _la == AQLParserDESC) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IValueContext is an interface to support dynamic dispatch.
type IValueContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	STRING() antlr.TerminalNode
	QUOTED_STRING() antlr.TerminalNode
	NUMBER() antlr.TerminalNode
	DURATION() antlr.TerminalNode

	// IsValueContext differentiates from other interfaces.
	IsValueContext()
}

type ValueContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyValueContext() *ValueContext {
	var p = new(ValueContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_value
	return p
}

func InitEmptyValueContext(p *ValueContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = AQLParserRULE_value
}

func (*ValueContext) IsValueContext() {}

func NewValueContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ValueContext {
	var p = new(ValueContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = AQLParserRULE_value

	return p
}

func (s *ValueContext) GetParser() antlr.Parser { return s.parser }

func (s *ValueContext) STRING() antlr.TerminalNode {
	return s.GetToken(AQLParserSTRING, 0)
}

func (s *ValueContext) QUOTED_STRING() antlr.TerminalNode {
	return s.GetToken(AQLParserQUOTED_STRING, 0)
}

func (s *ValueContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(AQLParserNUMBER, 0)
}

func (s *ValueContext) DURATION() antlr.TerminalNode {
	return s.GetToken(AQLParserDURATION, 0)
}

func (s *ValueContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ValueContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ValueContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case AQLVisitor:
		return t.VisitValue(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *AQLParser) Value() (localctx IValueContext) {
	localctx = NewValueContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, AQLParserRULE_value)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(156)
		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&98784247808) != 0) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

func (p *AQLParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 1:
		var t *ExprContext = nil
		if localctx != nil {
			t = localctx.(*ExprContext)
		}
		return p.Expr_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *AQLParser) Expr_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 3)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 2)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
