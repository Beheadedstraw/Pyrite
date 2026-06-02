package main

import (
	"fmt"
	"strings"
	"unicode"
)

type pyriteTokenType int

const (
	tokenEOF pyriteTokenType = iota
	tokenNewline
	tokenIndent
	tokenDedent
	tokenIdentifier
	tokenKeyword
	tokenNumber
	tokenString
	tokenColon
	tokenComma
	tokenDot
	tokenLParen
	tokenRParen
	tokenLBracket
	tokenRBracket
	tokenLBrace
	tokenRBrace
	tokenEqual
	tokenArrow
	tokenOperator
)

type pyriteToken struct {
	Type   pyriteTokenType
	Lexeme string
	Line   int
	Column int
}

func lexPyrite(source string) ([]pyriteToken, error) {
	var tokens []pyriteToken
	indents := []int{0}
	depth := 0
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	for lineIndex, raw := range lines {
		lineNo := lineIndex + 1
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := countIndent(raw)
		if depth == 0 {
			if indent > indents[len(indents)-1] {
				indents = append(indents, indent)
				tokens = append(tokens, pyriteToken{Type: tokenIndent, Lexeme: raw[:indent], Line: lineNo, Column: 1})
			} else {
				for indent < indents[len(indents)-1] {
					indents = indents[:len(indents)-1]
					tokens = append(tokens, pyriteToken{Type: tokenDedent, Line: lineNo, Column: indent + 1})
				}
				if indent != indents[len(indents)-1] {
					return nil, fmt.Errorf("line %d: inconsistent indentation", lineNo)
				}
			}
		}
		lineTokens, err := lexPyriteLine(raw[indent:], lineNo, indent+1)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, lineTokens...)
		depth += pyriteLineDepthDelta(lineTokens)
		if depth < 0 {
			return nil, fmt.Errorf("line %d: unmatched closing delimiter", lineNo)
		}
		if depth == 0 {
			tokens = append(tokens, pyriteToken{Type: tokenNewline, Line: lineNo, Column: len(raw) + 1})
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("line %d: unterminated multiline expression", len(lines))
	}
	for len(indents) > 1 {
		indents = indents[:len(indents)-1]
		tokens = append(tokens, pyriteToken{Type: tokenDedent, Line: len(lines), Column: 1})
	}
	tokens = append(tokens, pyriteToken{Type: tokenEOF, Line: len(lines), Column: 1})
	return tokens, nil
}

func pyriteLineDepthDelta(tokens []pyriteToken) int {
	delta := 0
	for _, tok := range tokens {
		switch tok.Type {
		case tokenLParen, tokenLBracket, tokenLBrace:
			delta++
		case tokenRParen, tokenRBracket, tokenRBrace:
			delta--
		}
	}
	return delta
}

func lexPyriteLine(line string, lineNo, baseColumn int) ([]pyriteToken, error) {
	var tokens []pyriteToken
	for i := 0; i < len(line); {
		ch := rune(line[i])
		column := baseColumn + i
		if ch == ' ' || ch == '\t' {
			i++
			continue
		}
		if ch == '#' {
			break
		}
		if (line[i] == 'b' || line[i] == 'f') && i+1 < len(line) && line[i+1] == '"' {
			tok, next, err := lexPyriteString(line, i, lineNo, column)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
			continue
		}
		if line[i] == '"' || line[i] == '\'' {
			tok, next, err := lexPyriteString(line, i, lineNo, column)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
			continue
		}
		if isPyriteIdentStart(ch) {
			start := i
			for i < len(line) && isPyriteIdentPart(rune(line[i])) {
				i++
			}
			lexeme := line[start:i]
			typ := tokenIdentifier
			if pyriteKeywords[lexeme] {
				typ = tokenKeyword
			}
			tokens = append(tokens, pyriteToken{Type: typ, Lexeme: lexeme, Line: lineNo, Column: column})
			continue
		}
		if unicode.IsDigit(ch) {
			start := i
			for i < len(line) && unicode.IsDigit(rune(line[i])) {
				i++
			}
			if i < len(line) && line[i] == '.' && i+1 < len(line) && unicode.IsDigit(rune(line[i+1])) {
				i++
				for i < len(line) && unicode.IsDigit(rune(line[i])) {
					i++
				}
			}
			tokens = append(tokens, pyriteToken{Type: tokenNumber, Lexeme: line[start:i], Line: lineNo, Column: column})
			continue
		}
		if i+1 < len(line) {
			two := line[i : i+2]
			switch two {
			case "->":
				tokens = append(tokens, pyriteToken{Type: tokenArrow, Lexeme: two, Line: lineNo, Column: column})
				i += 2
				continue
			case "==", "!=", "<=", ">=":
				tokens = append(tokens, pyriteToken{Type: tokenOperator, Lexeme: two, Line: lineNo, Column: column})
				i += 2
				continue
			}
		}
		typ, ok := pyriteSingleCharTokens[line[i]]
		if !ok {
			return nil, fmt.Errorf("line %d:%d: unexpected character %q", lineNo, column, line[i])
		}
		tokens = append(tokens, pyriteToken{Type: typ, Lexeme: string(line[i]), Line: lineNo, Column: column})
		i++
	}
	return tokens, nil
}

func lexPyriteString(line string, start, lineNo, column int) (pyriteToken, int, error) {
	i := start
	if (line[i] == 'b' || line[i] == 'f') && i+1 < len(line) && line[i+1] == '"' {
		i++
	}
	quote := line[i]
	i++
	for i < len(line) {
		if line[i] == '\\' {
			i += 2
			continue
		}
		if line[i] == quote {
			i++
			return pyriteToken{Type: tokenString, Lexeme: line[start:i], Line: lineNo, Column: column}, i, nil
		}
		i++
	}
	return pyriteToken{}, 0, fmt.Errorf("line %d:%d: unterminated string", lineNo, column)
}

func isPyriteIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isPyriteIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

var pyriteKeywords = map[string]bool{
	"import": true, "global": true, "const": true, "class": true, "enum": true,
	"native": true, "def": true, "if": true, "else": true, "while": true,
	"for": true, "in": true, "foreach": true, "switch": true, "match": true,
	"case": true, "default": true, "try": true, "except": true, "raise": true,
	"return": true, "async": true, "true": true, "false": true, "True": true,
	"False": true, "None": true, "not": true,
}

var pyriteSingleCharTokens = map[byte]pyriteTokenType{
	':': tokenColon,
	',': tokenComma,
	'.': tokenDot,
	'(': tokenLParen,
	')': tokenRParen,
	'[': tokenLBracket,
	']': tokenRBracket,
	'{': tokenLBrace,
	'}': tokenRBrace,
	'=': tokenEqual,
	'+': tokenOperator,
	'-': tokenOperator,
	'*': tokenOperator,
	'/': tokenOperator,
	'<': tokenOperator,
	'>': tokenOperator,
	'&': tokenOperator,
}
