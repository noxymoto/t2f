package lexer

import "fmt"

// TokenType represents the mathematical, logical, and literal types in t2f.
type TokenType string

const (
	// Single-character tokens
	LEFT_PAREN  TokenType = "("
	RIGHT_PAREN TokenType = ")"
	LEFT_BRACE  TokenType = "{"
	RIGHT_BRACE TokenType = "}"
	COMMA       TokenType = ","
	DOT         TokenType = "."
	MINUS       TokenType = "-"
	PLUS        TokenType = "+"
	SEMICOLON   TokenType = ";"
	SLASH       TokenType = "/"
	STAR        TokenType = "*"
	COLON       TokenType = ":"
	LBRACKET    TokenType = "["
	RBRACKET    TokenType = "]"
	AND         TokenType = "&&"
	OR          TokenType = "||"
	// One or two character tokens
	BANG          TokenType = "!"
	BANG_EQUAL    TokenType = "!="
	EQUAL         TokenType = "="
	EQUAL_EQUAL   TokenType = "=="
	GREATER       TokenType = ">"
	GREATER_EQUAL TokenType = ">="
	LESS          TokenType = "<"
	LESS_EQUAL    TokenType = "<="
	ARROW         TokenType = "=>"

	// Literals
	IDENT  TokenType = "IDENT"
	STRING TokenType = "STRING"
	NUMBER TokenType = "NUMBER"

	// Keywords
	ACTOR   TokenType = "ACTOR"
	LET     TokenType = "LET"
	TYPE    TokenType = "TYPE"
	IF      TokenType = "IF"
	ELSE    TokenType = "ELSE"
	TRUE    TokenType = "TRUE"
	FALSE   TokenType = "FALSE"
	PRINT   TokenType = "PRINT"
	WHILE   TokenType = "WHILE"
	FOR     TokenType = "FOR"
	INCLUDE TokenType = "INCLUDE"
	FN      TokenType = "FN"
	RETURN  TokenType = "RETURN"

	EOF     TokenType = "EOF"
	ILLEGAL TokenType = "ILLEGAL"
)

// Token represents a lexical token in the source code
type Token struct {
	Type    TokenType
	Literal string
	Line    int
}

func (t Token) String() string {
	return fmt.Sprintf("{Type: %s, Literal: '%s', Line: %d}", t.Type, t.Literal, t.Line)
}

// LookupIdent checks whether a given identifier is a keyword.
func LookupIdent(ident string) TokenType {
	keywords := map[string]TokenType{
		"actor":   ACTOR,
		"let":     LET,
		"type":    TYPE,
		"if":      IF,
		"else":    ELSE,
		"true":    TRUE,
		"false":   FALSE,
		"print":   PRINT,
		"while":   WHILE,
		"for":     FOR,
		"include": INCLUDE,
		"fn":      FN,
		"return":  RETURN,
	}

	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
