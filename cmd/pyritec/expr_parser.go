package main

import "fmt"

func parsePyriteExpression(source string) (pyriteExpr, error) {
	tokens, err := lexPyriteLine(source, 1, 1)
	if err != nil {
		return nil, err
	}
	tokens = append(tokens, pyriteToken{Type: tokenEOF, Line: 1, Column: len(source) + 1})
	parser := &pyriteExprParser{tokens: tokens}
	expr, err := parser.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if !parser.at(tokenEOF) {
		return nil, parser.errorAtCurrent("unexpected token after expression")
	}
	return expr, nil
}

func parsePyriteExpressionTokens(tokens []pyriteToken) (pyriteExpr, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty expression")
	}
	exprTokens := append([]pyriteToken{}, tokens...)
	last := exprTokens[len(exprTokens)-1]
	exprTokens = append(exprTokens, pyriteToken{Type: tokenEOF, Line: last.Line, Column: last.Column + len(last.Lexeme)})
	parser := &pyriteExprParser{tokens: exprTokens}
	expr, err := parser.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if !parser.at(tokenEOF) {
		return nil, parser.errorAtCurrent("unexpected token after expression")
	}
	return expr, nil
}

type pyriteExprParser struct {
	tokens []pyriteToken
	pos    int
}

func (p *pyriteExprParser) parseExpression(minPrec int) (pyriteExpr, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}
	for {
		if p.match(tokenLParen) {
			call := &pyriteCallExpr{Callee: left, Line: p.previous().Line}
			if !p.at(tokenRParen) {
				for {
					arg, err := p.parseExpression(0)
					if err != nil {
						return nil, err
					}
					call.Args = append(call.Args, arg)
					if p.match(tokenComma) {
						if p.at(tokenRParen) {
							break
						}
						continue
					}
					if !p.at(tokenRParen) {
						break
					}
					break
				}
			}
			if _, err := p.expect(tokenRParen, "expected ) after call arguments"); err != nil {
				return nil, err
			}
			left = call
			continue
		}
		if p.match(tokenDot) {
			field, err := p.expectIdentifier("expected member name after .")
			if err != nil {
				return nil, err
			}
			left = &pyriteMemberExpr{Base: left, Field: field.Lexeme, Line: field.Line}
			continue
		}
		if p.match(tokenLBracket) {
			index, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokenRBracket, "expected ] after index"); err != nil {
				return nil, err
			}
			left = &pyriteIndexExpr{Base: left, Index: index, Line: p.previous().Line}
			continue
		}
		op := p.peek()
		prec := binaryPrecedence(op)
		if prec < minPrec {
			break
		}
		p.advance()
		right, err := p.parseExpression(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &pyriteBinaryExpr{Left: left, Op: op.Lexeme, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *pyriteExprParser) parsePrefix() (pyriteExpr, error) {
	tok := p.advance()
	switch tok.Type {
	case tokenIdentifier:
		return &pyriteNameExpr{Name: tok.Lexeme, Line: tok.Line}, nil
	case tokenKeyword:
		switch tok.Lexeme {
		case "True", "False", "true", "false":
			return &pyriteLiteralExpr{Value: tok.Lexeme, Kind: "bool", Line: tok.Line}, nil
		default:
			return &pyriteNameExpr{Name: tok.Lexeme, Line: tok.Line}, nil
		}
	case tokenNumber:
		return &pyriteLiteralExpr{Value: tok.Lexeme, Kind: "number", Line: tok.Line}, nil
	case tokenString:
		return &pyriteLiteralExpr{Value: tok.Lexeme, Kind: "string", Line: tok.Line}, nil
	case tokenOperator:
		if tok.Lexeme == "-" {
			right, err := p.parseExpression(5)
			if err != nil {
				return nil, err
			}
			return &pyriteUnaryExpr{Op: tok.Lexeme, Right: right, Line: tok.Line}, nil
		}
	case tokenLParen:
		expr, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRParen, "expected ) after grouped expression"); err != nil {
			return nil, err
		}
		return expr, nil
	case tokenLBracket:
		list := &pyriteListExpr{Line: tok.Line}
		if !p.at(tokenRBracket) {
			for {
				item, err := p.parseExpression(0)
				if err != nil {
					return nil, err
				}
				list.Items = append(list.Items, item)
				if p.match(tokenComma) {
					if p.at(tokenRBracket) {
						break
					}
					continue
				}
				if !p.at(tokenRBracket) {
					break
				}
				break
			}
		}
		if _, err := p.expect(tokenRBracket, "expected ] after list literal"); err != nil {
			return nil, err
		}
		return list, nil
	case tokenLBrace:
		obj := &pyriteObjectExpr{Line: tok.Line}
		if !p.at(tokenRBrace) {
			for {
				key, err := p.parseExpression(0)
				if err != nil {
					return nil, err
				}
				if _, err := p.expect(tokenColon, "expected : in object literal"); err != nil {
					return nil, err
				}
				value, err := p.parseExpression(0)
				if err != nil {
					return nil, err
				}
				obj.Entries = append(obj.Entries, pyriteObjectEntry{Key: key, Value: value})
				if p.match(tokenComma) {
					if p.at(tokenRBrace) {
						break
					}
					continue
				}
				if !p.at(tokenRBrace) {
					break
				}
				break
			}
		}
		if _, err := p.expect(tokenRBrace, "expected } after object literal"); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return nil, p.errorAt(tok, "expected expression")
}

func binaryPrecedence(tok pyriteToken) int {
	switch tok.Type {
	case tokenOperator:
		switch tok.Lexeme {
		case "==", "!=", "<", "<=", ">", ">=":
			return 1
		case "&":
			return 2
		case "+", "-":
			return 3
		case "*", "/":
			return 4
		}
	}
	return -1
}

func (p *pyriteExprParser) expectIdentifier(message string) (pyriteToken, error) {
	if p.at(tokenIdentifier) || p.at(tokenKeyword) {
		return p.advance(), nil
	}
	return pyriteToken{}, p.errorAtCurrent(message)
}

func (p *pyriteExprParser) expect(typ pyriteTokenType, message string) (pyriteToken, error) {
	if p.at(typ) {
		return p.advance(), nil
	}
	return pyriteToken{}, p.errorAtCurrent(message)
}

func (p *pyriteExprParser) match(typ pyriteTokenType) bool {
	if !p.at(typ) {
		return false
	}
	p.advance()
	return true
}

func (p *pyriteExprParser) at(typ pyriteTokenType) bool {
	return p.peek().Type == typ
}

func (p *pyriteExprParser) advance() pyriteToken {
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return p.tokens[p.pos-1]
}

func (p *pyriteExprParser) peek() pyriteToken {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *pyriteExprParser) previous() pyriteToken {
	if p.pos == 0 {
		return p.peek()
	}
	return p.tokens[p.pos-1]
}

func (p *pyriteExprParser) errorAtCurrent(message string) error {
	return p.errorAt(p.peek(), message)
}

func (p *pyriteExprParser) errorAt(tok pyriteToken, message string) error {
	return fmt.Errorf("line %d:%d: %s", tok.Line, tok.Column, message)
}
