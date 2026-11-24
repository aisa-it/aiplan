// AQL — язык запросов к задачам, обратно совместимый с JQL.
//
// Генерация (из каталога pkg/aql):
//   antlr4 -Dlanguage=Go -visitor -no-listener -package parser -Xexact-output-dir -o parser AQL.g4
//
// Имена полей и функций не являются токенами: алиасы (reporter → author и т.п.)
// и встроенные/пользовательские поля разрешаются при обходе дерева.
// Ключевые слова зарезервированы, как в JQL: как значение — только в кавычках.
grammar AQL;

options {
    caseInsensitive = true;
}

// ============================== Parser ==============================

query
    : expr? orderBy? EOF
    ;

// Приоритет: NOT > AND > OR.
expr
    : LPAREN expr RPAREN    # parenExpr
    | NOT expr              # notExpr
    | expr AND expr         # andExpr
    | expr OR expr          # orExpr
    | clause                # clauseExpr
    ;

clause
    : field op=(EQ | NEQ | LIKE | NOT_LIKE | LT | LTE | GT | GTE) operand  # compareClause
    | field NOT? IN inOperand                                             # inClause
    | field IS NOT? EMPTY                                                 # isEmptyClause
    | field WAS NOT? (IN inOperand | operand) historyPredicate*            # wasClause
    | field CHANGED historyPredicate*                                     # changedClause
    ;

// Поле по имени (status, "Площадка") или JQL-форма cf[10010].
field
    : value (LBRACKET NUMBER RBRACKET)?
    ;

operand
    : EMPTY
    | function
    | value
    ;

inOperand
    : list
    | function
    ;

list
    : LPAREN operand (COMMA operand)* RPAREN
    ;

function
    : STRING LPAREN (value (COMMA value)*)? RPAREN
    ;

historyPredicate
    : op=(AFTER | BEFORE | ON | DURING | BY | FROM | TO) (operand | list)
    ;

orderBy
    : ORDER BY sortField (COMMA sortField)*
    ;

sortField
    : field (ASC | DESC)?
    ;

value
    : STRING
    | QUOTED_STRING
    | NUMBER
    | DURATION
    ;

// ============================== Lexer ===============================

// Ключевые слова объявлены раньше STRING: при равной длине побеждает первое правило.
AND     : 'AND' | '&&' | '&';
OR      : 'OR' | '||' | '|';
NOT     : 'NOT' | '!';
IN      : 'IN';
IS      : 'IS';
EMPTY   : 'EMPTY' | 'NULL';
ORDER   : 'ORDER';
BY      : 'BY';
ASC     : 'ASC';
DESC    : 'DESC';
WAS     : 'WAS';
CHANGED : 'CHANGED';
AFTER   : 'AFTER';
BEFORE  : 'BEFORE';
ON      : 'ON';
DURING  : 'DURING';
FROM    : 'FROM';
TO      : 'TO';

EQ       : '=';
NEQ      : '!=';
LIKE     : '~';
NOT_LIKE : '!~';
LT       : '<';
LTE      : '<=';
GT       : '>';
GTE      : '>=';
LPAREN   : '(';
RPAREN   : ')';
LBRACKET : '[';
RBRACKET : ']';
COMMA    : ',';

NUMBER : [+-]? DIGIT+;

// Длительность или сдвиг даты: 2h 30m, 2ч30м, -7d, +1w 2d.
// Части можно разделять пробелами — значение остаётся одним токеном.
DURATION : [+-]? DUR_PART ([ \t]* DUR_PART)*;

QUOTED_STRING
    : '"' (ESC | ~["\\\r\n])* '"'
    | '\'' (ESC | ~['\\\r\n])* '\''
    ;

// Незакрытая кавычка — отдельный токен ради понятной ошибки парсера.
UNTERMINATED_STRING
    : '"' (ESC | ~["\\\r\n])*
    | '\'' (ESC | ~['\\\r\n])*
    ;

// Строка без кавычек. Мягче JQL: допускает . @ / + * ? : # — почты, даты, маски.
STRING
    : (ESC | ~[ \t\r\n\f\u00A0"'\\=!<>~(),[\]{}|&])+
    ;

WS : [ \t\r\n\f\u00A0]+ -> skip;

// Любой непредусмотренный символ уходит в парсер и роняет разбор, а не теряется в лексере.
ERROR_CHAR : .;

// Экранирование любого символа; \t \n \r \uXXXX раскрываются при обходе дерева.
fragment ESC      : '\\' ~[\r\n];
fragment DIGIT    : [0-9];
fragment DUR_PART : DIGIT+ ('.' DIGIT+)? DUR_UNIT;
// m — минуты, M — месяцы (как в JQL); регистр различается уже по тексту токена.
fragment DUR_UNIT : [ywdhm] | 'г' | 'мес' | 'нед' | 'н' | 'д' | 'ч' | 'мин' | 'м';
