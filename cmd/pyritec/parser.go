package main

import (
	"fmt"
	"strings"
)

type pyriteParser struct {
	tokens []pyriteToken
	pos    int
}

func parsePyriteProgram(source string) (*pyriteProgram, error) {
	tokens, err := lexPyrite(source)
	if err != nil {
		return nil, err
	}
	parser := &pyriteParser{tokens: tokens}
	return parser.parseProgram()
}

func (p *pyriteParser) parseProgram() (*pyriteProgram, error) {
	program := &pyriteProgram{}
	for !p.at(tokenEOF) {
		p.skipNewlines()
		if p.at(tokenEOF) {
			break
		}
		item, err := p.parseTopLevel()
		if err != nil {
			return nil, err
		}
		program.Items = append(program.Items, item)
	}
	return program, nil
}

func (p *pyriteParser) parseTopLevel() (pyriteTopLevel, error) {
	switch {
	case p.matchKeyword("import"):
		return p.parseImport()
	case p.matchKeyword("global"):
		return p.parseBinding(true, false)
	case p.matchKeyword("const"):
		return p.parseBinding(false, true)
	case p.matchKeyword("native"):
		return p.parseNativeFunction()
	case p.matchKeyword("def"):
		return p.parseFunction(false, 0)
	case p.matchKeyword("class"):
		return p.parseClass()
	case p.matchKeyword("enum"):
		return p.parseEnum()
	case p.startsTopLevelBinding():
		return p.parseBinding(false, false)
	default:
		tok := p.peek()
		return nil, fmt.Errorf("line %d: expected top-level declaration, got %q", tok.Line, tok.Lexeme)
	}
}

func (p *pyriteParser) parseImport() (*pyriteImportDecl, error) {
	line := p.previous().Line
	name, err := p.expectIdentifier("expected module name after import")
	if err != nil {
		return nil, err
	}
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	return &pyriteImportDecl{Name: name.Lexeme, Line: line}, nil
}

func (p *pyriteParser) parseBinding(global, immutable bool) (*pyriteBindingDecl, error) {
	line := p.peek().Line
	name, err := p.expectIdentifier("expected binding name")
	if err != nil {
		return nil, err
	}
	var typ string
	if p.match(tokenColon) {
		typeTokens, err := p.collectUntil(tokenEqual)
		if err != nil {
			return nil, err
		}
		typ = tokensText(typeTokens)
	}
	if _, err := p.expect(tokenEqual, "expected = in binding"); err != nil {
		return nil, err
	}
	value, err := p.collectLineTokens()
	if err != nil {
		return nil, err
	}
	valueExpr, err := parsePyriteExpressionTokens(value)
	if err != nil {
		return nil, err
	}
	return &pyriteBindingDecl{
		Name: name.Lexeme, Type: typ, Value: tokensText(value), ValueExpr: valueExpr, Const: immutable,
		Global: global, Line: line,
	}, nil
}

func (p *pyriteParser) startsTopLevelBinding() bool {
	if !p.at(tokenIdentifier) {
		return false
	}
	next := p.lookahead(1)
	return next.Type == tokenColon || next.Type == tokenEqual
}

func (p *pyriteParser) parseNativeFunction() (*pyriteFunctionDecl, error) {
	line := p.previous().Line
	if !p.matchKeyword("def") {
		return nil, p.errorAtCurrent("expected def after native")
	}
	fn, err := p.parseFunctionHeader(line, false)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenArrow, "expected -> return type in native def"); err != nil {
		return nil, err
	}
	retTokens, err := p.collectUntil(tokenEqual)
	if err != nil {
		return nil, err
	}
	fn.ReturnType = tokensText(retTokens)
	if _, err := p.expect(tokenEqual, "expected = native symbol"); err != nil {
		return nil, err
	}
	symbol, err := p.expectIdentifier("expected native symbol")
	if err != nil {
		return nil, err
	}
	fn.NativeSymbol = symbol.Lexeme
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	return fn, nil
}

func (p *pyriteParser) parseFunction(topLevel bool, classIndent int) (*pyriteFunctionDecl, error) {
	line := p.previous().Line
	fn, err := p.parseFunctionHeader(line, true)
	if err != nil {
		return nil, err
	}
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	body, err := p.parseIndentedBlock(fn.Indent)
	if err != nil {
		return nil, err
	}
	fn.Body = body
	_ = topLevel
	_ = classIndent
	return fn, nil
}

func (p *pyriteParser) parseFunctionHeader(line int, requireColon bool) (*pyriteFunctionDecl, error) {
	name, err := p.expectIdentifier("expected function name")
	if err != nil {
		return nil, err
	}
	fn := &pyriteFunctionDecl{Name: name.Lexeme, Line: line, Indent: name.Column - 1}
	if _, err := p.expect(tokenLParen, "expected ( after function name"); err != nil {
		return nil, err
	}
	if !p.at(tokenRParen) {
		for {
			paramName, err := p.expectIdentifier("expected parameter name")
			if err != nil {
				return nil, err
			}
			param := pyriteParam{Name: paramName.Lexeme}
			if p.match(tokenColon) {
				typeTokens, err := p.collectUntilAny(tokenComma, tokenRParen)
				if err != nil {
					return nil, err
				}
				param.Type = tokensText(typeTokens)
			}
			fn.Params = append(fn.Params, param)
			if !p.match(tokenComma) {
				break
			}
		}
	}
	if _, err := p.expect(tokenRParen, "expected ) after parameters"); err != nil {
		return nil, err
	}
	if requireColon {
		if _, err := p.expect(tokenColon, "expected : after function header"); err != nil {
			return nil, err
		}
	}
	return fn, nil
}

func (p *pyriteParser) parseClass() (*pyriteClassDecl, error) {
	line := p.previous().Line
	name, err := p.expectIdentifier("expected class name")
	if err != nil {
		return nil, err
	}
	cls := &pyriteClassDecl{Name: name.Lexeme, Line: line, Indent: name.Column - 1}
	if _, err := p.expect(tokenColon, "expected : after class name"); err != nil {
		return nil, err
	}
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenIndent, "expected indented class body"); err != nil {
		return nil, err
	}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipNewlines()
		if p.at(tokenDedent) {
			break
		}
		if !p.matchKeyword("def") {
			return nil, p.errorAtCurrent("class body currently expects method definitions")
		}
		fn, err := p.parseFunction(false, cls.Indent)
		if err != nil {
			return nil, err
		}
		cls.Methods = append(cls.Methods, fn)
	}
	if _, err := p.expect(tokenDedent, "expected end of class body"); err != nil {
		return nil, err
	}
	return cls, nil
}

func (p *pyriteParser) parseEnum() (*pyriteEnumDecl, error) {
	line := p.previous().Line
	name, err := p.expectIdentifier("expected enum name")
	if err != nil {
		return nil, err
	}
	enum := &pyriteEnumDecl{Name: name.Lexeme, Line: line, Indent: name.Column - 1}
	if _, err := p.expect(tokenColon, "expected : after enum name"); err != nil {
		return nil, err
	}
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenIndent, "expected indented enum body"); err != nil {
		return nil, err
	}
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipNewlines()
		if p.at(tokenDedent) {
			break
		}
		member, err := p.expectIdentifier("expected enum member")
		if err != nil {
			return nil, err
		}
		item := pyriteEnumMember{Name: member.Lexeme, Line: member.Line}
		if p.match(tokenEqual) {
			value, err := p.collectLineTokens()
			if err != nil {
				return nil, err
			}
			valueExpr, err := parsePyriteExpressionTokens(value)
			if err != nil {
				return nil, err
			}
			item.Value = tokensText(value)
			item.ValueExpr = valueExpr
			item.HasValue = true
		} else if err := p.expectLineEnd(); err != nil {
			return nil, err
		}
		enum.Members = append(enum.Members, item)
	}
	if _, err := p.expect(tokenDedent, "expected end of enum body"); err != nil {
		return nil, err
	}
	return enum, nil
}

func (p *pyriteParser) parseIndentedBlock(parentIndent int) ([]pyriteStmt, error) {
	if _, err := p.expect(tokenIndent, "expected indented block"); err != nil {
		return nil, err
	}
	var stmts []pyriteStmt
	for !p.at(tokenDedent) && !p.at(tokenEOF) {
		p.skipNewlines()
		if p.at(tokenDedent) {
			break
		}
		stmt, err := p.parseStatementLine(parentIndent + 4)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}
	if _, err := p.expect(tokenDedent, "expected end of block"); err != nil {
		return nil, err
	}
	return stmts, nil
}

func (p *pyriteParser) parseStatementLine(indent int) (pyriteStmt, error) {
	start := p.peek()
	tokens, err := p.collectLineTokens()
	if err != nil {
		return nil, err
	}
	stmt, err := parsePyriteStatementTokens(tokens, start.Line, indent)
	if err != nil {
		return nil, err
	}
	if lineOpensBlock(tokens) && p.at(tokenIndent) {
		children, err := p.parseIndentedBlock(indent)
		if err != nil {
			return nil, err
		}
		stmt.stmtBase().Children = children
	}
	return stmt, nil
}

func parsePyriteStatementTokens(tokens []pyriteToken, line, indent int) (pyriteStmt, error) {
	if len(tokens) == 0 {
		return &pyriteControlStmt{pyriteStmtBase: pyriteStmtBase{Line: line, Indent: indent, Text: tokensText(tokens)}, Kind: ""}, nil
	}
	base := pyriteStmtBase{Line: line, Indent: indent, Text: tokensText(tokens)}
	first := tokens[0].Lexeme
	switch first {
	case "return":
		if len(tokens) > 1 {
			expr, err := parsePyriteExpressionTokens(tokens[1:])
			if err != nil {
				return nil, err
			}
			return &pyriteReturnStmt{pyriteStmtBase: base, Value: expr}, nil
		}
		return &pyriteReturnStmt{pyriteStmtBase: base}, nil
	case "raise":
		if len(tokens) > 1 {
			expr, err := parsePyriteExpressionTokens(tokens[1:])
			if err != nil {
				return nil, err
			}
			return &pyriteRaiseStmt{pyriteStmtBase: base, Value: expr}, nil
		}
		return &pyriteRaiseStmt{pyriteStmtBase: base}, nil
	case "if":
		condition, inline, hasInline := splitTopLevelColon(tokens[1:])
		var expr pyriteExpr
		if len(condition) > 0 {
			parsed, err := parsePyriteExpressionTokens(condition)
			if err != nil {
				return nil, err
			}
			expr = parsed
		}
		stmt := &pyriteIfStmt{pyriteStmtBase: base, Condition: expr}
		if hasInline && len(inline) > 0 {
			parsed, err := parsePyriteStatementTokens(inline, inline[0].Line, inline[0].Column-1)
			if err != nil {
				return nil, err
			}
			stmt.Inline = parsed
		}
		return stmt, nil
	case "while":
		header, inline, hasInline := splitTopLevelColon(tokens[1:])
		var expr pyriteExpr
		if len(header) > 0 {
			parsed, err := parsePyriteExpressionTokens(header)
			if err != nil {
				return nil, err
			}
			expr = parsed
		}
		stmt := &pyriteWhileStmt{pyriteStmtBase: base, Condition: expr}
		if hasInline && len(inline) > 0 {
			parsed, err := parsePyriteStatementTokens(inline, inline[0].Line, inline[0].Column-1)
			if err != nil {
				return nil, err
			}
			stmt.Inline = parsed
		}
		return stmt, nil
	case "switch", "match":
		header, _, _ := splitTopLevelColon(tokens[1:])
		var expr pyriteExpr
		if len(header) > 0 {
			parsed, err := parsePyriteExpressionTokens(header)
			if err != nil {
				return nil, err
			}
			expr = parsed
		}
		return &pyriteMatchStmt{pyriteStmtBase: base, Value: expr}, nil
	case "case":
		pattern, inline, hasInline := splitTopLevelColon(tokens[1:])
		stmt := &pyriteCaseStmt{pyriteStmtBase: base}
		if len(pattern) == 1 && pattern[0].Lexeme == "_" {
			stmt.Wildcard = true
			if hasInline && len(inline) > 0 {
				parsed, err := parsePyriteStatementTokens(inline, inline[0].Line, inline[0].Column-1)
				if err != nil {
					return nil, err
				}
				stmt.Inline = parsed
			}
			return stmt, nil
		}
		if len(pattern) > 0 {
			expr, err := parsePyriteExpressionTokens(pattern)
			if err != nil {
				return nil, err
			}
			stmt.Pattern = expr
		}
		if hasInline && len(inline) > 0 {
			parsed, err := parsePyriteStatementTokens(inline, inline[0].Line, inline[0].Column-1)
			if err != nil {
				return nil, err
			}
			stmt.Inline = parsed
		}
		return stmt, nil
	case "default", "else", "try":
		return &pyriteControlStmt{pyriteStmtBase: base, Kind: first}, nil
	case "except":
		nameTokens := trimTrailingColon(tokens[1:])
		if len(nameTokens) == 0 {
			return &pyriteExceptStmt{pyriteStmtBase: base}, nil
		}
		if len(nameTokens) == 1 && nameTokens[0].Type == tokenIdentifier {
			return &pyriteExceptStmt{pyriteStmtBase: base, Name: nameTokens[0].Lexeme}, nil
		}
		return nil, fmt.Errorf("line %d:%d: invalid except header", tokens[0].Line, tokens[0].Column)
	case "for":
		return parseForStatement(tokens, base)
	case "foreach":
		expr, err := parsePyriteExpressionTokens(trimTrailingColon(tokens))
		if err != nil {
			return nil, err
		}
		call, ok := expr.(*pyriteCallExpr)
		if !ok {
			return nil, fmt.Errorf("line %d:%d: invalid foreach statement", tokens[0].Line, tokens[0].Column)
		}
		if len(call.Args) < 1 || len(call.Args) > 2 {
			return nil, fmt.Errorf("line %d:%d: foreach expects list and optional item name", tokens[0].Line, tokens[0].Column)
		}
		itemName := "item"
		if len(call.Args) == 2 {
			name, ok := call.Args[1].(*pyriteNameExpr)
			if !ok || !isIdentifier(name.Name) {
				return nil, fmt.Errorf("line %d:%d: invalid foreach item name", tokens[0].Line, tokens[0].Column)
			}
			itemName = name.Name
		}
		return &pyriteForStmt{pyriteStmtBase: base, Target: itemName, Iterable: call.Args[0]}, nil
	}
	if tokens[0].Lexeme == "print" || tokens[0].Lexeme == "routine" || tokens[0].Lexeme == "async" {
		expr, err := parsePyriteExpressionTokens(tokens)
		if err != nil {
			return nil, err
		}
		return &pyriteExprStmt{pyriteStmtBase: base, Expr: expr}, nil
	}
	if idx := topLevelTokenIndex(tokens, tokenEqual); idx >= 0 {
		if idx+1 >= len(tokens) {
			return nil, fmt.Errorf("line %d:%d: expected expression after =", tokens[idx].Line, tokens[idx].Column)
		}
		value, err := parsePyriteExpressionTokens(tokens[idx+1:])
		if err != nil {
			return nil, err
		}
		if colon := topLevelTokenIndex(tokens[:idx], tokenColon); colon >= 0 && len(tokens[:idx]) > 0 {
			return &pyriteVarStmt{
				pyriteStmtBase: base,
				Name:           strings.TrimSpace(tokensText(tokens[:colon])),
				Type:           strings.TrimSpace(tokensText(tokens[colon+1 : idx])),
				Value:          value,
			}, nil
		}
		targetExpr, err := parsePyriteExpressionTokens(tokens[:idx])
		if err != nil {
			return nil, err
		}
		return &pyriteAssignStmt{pyriteStmtBase: base, TargetExpr: targetExpr, Value: value}, nil
	}
	expr, err := parsePyriteExpressionTokens(tokens)
	if err != nil {
		return nil, err
	}
	return &pyriteExprStmt{pyriteStmtBase: base, Expr: expr}, nil
}

func validateStatementExpressions(tokens []pyriteToken) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := parsePyriteStatementTokens(tokens, tokens[0].Line, tokens[0].Column-1)
	return err
}

func validatePyriteExpressionTokens(tokens []pyriteToken) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := parsePyriteExpressionTokens(tokens)
	return err
}

func validateForStatementExpressions(tokens []pyriteToken) error {
	_, err := parseForStatement(tokens, pyriteStmtBase{Line: tokens[0].Line, Indent: tokens[0].Column - 1, Text: tokensText(tokens)})
	return err
}

func parseForStatement(tokens []pyriteToken, base pyriteStmtBase) (pyriteStmt, error) {
	inIndex := -1
	for i, tok := range tokens {
		if tok.Lexeme == "in" {
			inIndex = i
			break
		}
	}
	if inIndex < 0 {
		return &pyriteControlStmt{pyriteStmtBase: base, Kind: "for"}, nil
	}
	iterableTokens, _, _ := splitTopLevelColon(tokens[inIndex+1:])
	var iterable pyriteExpr
	if len(iterableTokens) > 0 {
		parsed, err := parsePyriteExpressionTokens(iterableTokens)
		if err != nil {
			return nil, err
		}
		iterable = parsed
	}
	return &pyriteForStmt{pyriteStmtBase: base, Target: strings.TrimSpace(tokensText(tokens[1:inIndex])), Iterable: iterable}, nil
}

func validateCallArgumentExpressions(tokens []pyriteToken) error {
	if len(tokens) == 0 {
		return nil
	}
	var current []pyriteToken
	depth := 0
	for _, tok := range tokens {
		if tok.Type == tokenComma && depth == 0 {
			if err := validatePyriteExpressionTokens(current); err != nil {
				return err
			}
			current = nil
			continue
		}
		switch tok.Type {
		case tokenLParen, tokenLBracket, tokenLBrace:
			depth++
		case tokenRParen, tokenRBracket, tokenRBrace:
			depth--
		}
		current = append(current, tok)
	}
	return validatePyriteExpressionTokens(current)
}

func trimTrailingColon(tokens []pyriteToken) []pyriteToken {
	if len(tokens) > 0 && tokens[len(tokens)-1].Type == tokenColon {
		return tokens[:len(tokens)-1]
	}
	return tokens
}

func splitTopLevelColon(tokens []pyriteToken) ([]pyriteToken, []pyriteToken, bool) {
	if idx := topLevelTokenIndex(tokens, tokenColon); idx >= 0 {
		return tokens[:idx], tokens[idx+1:], true
	}
	return tokens, nil, false
}

func topLevelTokenIndex(tokens []pyriteToken, typ pyriteTokenType) int {
	depth := 0
	for i, tok := range tokens {
		switch tok.Type {
		case tokenLParen, tokenLBracket, tokenLBrace:
			depth++
		case tokenRParen, tokenRBracket, tokenRBrace:
			depth--
		default:
			if depth == 0 && tok.Type == typ {
				return i
			}
		}
	}
	return -1
}

func lineOpensBlock(tokens []pyriteToken) bool {
	return len(tokens) > 0 && tokens[len(tokens)-1].Type == tokenColon
}

func (p *pyriteParser) collectLineTokens() ([]pyriteToken, error) {
	var out []pyriteToken
	for !p.at(tokenNewline) && !p.at(tokenEOF) {
		if p.at(tokenIndent) || p.at(tokenDedent) {
			return nil, p.errorAtCurrent("unexpected indentation in line")
		}
		out = append(out, p.advance())
	}
	if err := p.expectLineEnd(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *pyriteParser) collectUntil(typ pyriteTokenType) ([]pyriteToken, error) {
	return p.collectUntilAny(typ)
}

func (p *pyriteParser) collectUntilAny(types ...pyriteTokenType) ([]pyriteToken, error) {
	var out []pyriteToken
	depth := 0
	for !p.at(tokenEOF) {
		if depth == 0 {
			for _, typ := range types {
				if p.at(typ) {
					return out, nil
				}
			}
			if p.at(tokenNewline) {
				return nil, p.errorAtCurrent("unexpected end of line")
			}
		}
		tok := p.advance()
		switch tok.Type {
		case tokenLParen, tokenLBracket, tokenLBrace:
			depth++
		case tokenRParen, tokenRBracket, tokenRBrace:
			depth--
		}
		out = append(out, tok)
	}
	return nil, p.errorAtCurrent("unexpected end of file")
}

func (p *pyriteParser) expectLineEnd() error {
	if p.match(tokenNewline) || p.at(tokenEOF) {
		return nil
	}
	return p.errorAtCurrent("expected end of line")
}

func (p *pyriteParser) skipNewlines() {
	for p.match(tokenNewline) {
	}
}

func (p *pyriteParser) expectIdentifier(message string) (pyriteToken, error) {
	if p.at(tokenIdentifier) || p.at(tokenKeyword) {
		return p.advance(), nil
	}
	return pyriteToken{}, p.errorAtCurrent(message)
}

func (p *pyriteParser) expect(typ pyriteTokenType, message string) (pyriteToken, error) {
	if p.at(typ) {
		return p.advance(), nil
	}
	return pyriteToken{}, p.errorAtCurrent(message)
}

func (p *pyriteParser) match(typ pyriteTokenType) bool {
	if !p.at(typ) {
		return false
	}
	p.advance()
	return true
}

func (p *pyriteParser) matchKeyword(word string) bool {
	if p.peek().Type != tokenKeyword || p.peek().Lexeme != word {
		return false
	}
	p.advance()
	return true
}

func (p *pyriteParser) at(typ pyriteTokenType) bool {
	return p.peek().Type == typ
}

func (p *pyriteParser) advance() pyriteToken {
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return p.tokens[p.pos-1]
}

func (p *pyriteParser) peek() pyriteToken {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *pyriteParser) lookahead(offset int) pyriteToken {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[idx]
}

func (p *pyriteParser) previous() pyriteToken {
	if p.pos == 0 {
		return p.peek()
	}
	return p.tokens[p.pos-1]
}

func (p *pyriteParser) errorAtCurrent(message string) error {
	tok := p.peek()
	return fmt.Errorf("line %d:%d: %s", tok.Line, tok.Column, message)
}

func tokensText(tokens []pyriteToken) string {
	var b strings.Builder
	prev := pyriteToken{}
	for i, tok := range tokens {
		if i > 0 && needsTokenSpace(prev, tok) {
			b.WriteByte(' ')
		}
		b.WriteString(tok.Lexeme)
		prev = tok
	}
	return b.String()
}

func needsTokenSpace(left, right pyriteToken) bool {
	if left.Type == tokenKeyword {
		switch left.Lexeme {
		case "return", "raise", "if", "while", "for", "case", "match", "switch":
			return right.Type != tokenColon
		}
	}
	if left.Type == tokenIdentifier || left.Type == tokenKeyword || left.Type == tokenNumber || left.Type == tokenString {
		return right.Type == tokenIdentifier || right.Type == tokenKeyword || right.Type == tokenNumber || right.Type == tokenString
	}
	return false
}
