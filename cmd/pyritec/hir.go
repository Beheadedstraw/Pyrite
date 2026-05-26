package main

type pyriteHIRProgram struct {
	Functions map[string]*pyriteHIRFunction
	Order     []string
}

type pyriteHIRFunction struct {
	Name       string
	Params     []pyriteHIRParam
	ReturnType string
	Native     bool
	Body       []pyriteHIRStmt
}

type pyriteHIRParam struct {
	Name string
	Type string
}

type pyriteHIRStmt struct {
	Kind       string
	Line       int
	Name       string
	Type       string
	Expr       pyriteExpr
	Children   []pyriteHIRStmt
	Inline     []pyriteHIRStmt
	Wildcard   bool
	Target     string
	TargetExpr pyriteExpr
}

func (c *Compiler) lowerHIR() (*pyriteHIRProgram, error) {
	program := &pyriteHIRProgram{Functions: map[string]*pyriteHIRFunction{}}
	for _, name := range c.functionOrder {
		fn := c.functions[name]
		lowered, err := c.lowerFunctionHIR(fn)
		if err != nil {
			return nil, err
		}
		program.Functions[name] = lowered
		program.Order = append(program.Order, name)
	}
	return program, nil
}

func (c *Compiler) lowerFunctionHIR(fn *functionDef) (*pyriteHIRFunction, error) {
	lowered := &pyriteHIRFunction{
		Name:       fn.name,
		ReturnType: fn.returnType,
		Native:     fn.nativeSymbol != "",
	}
	for _, param := range fn.params {
		lowered.Params = append(lowered.Params, pyriteHIRParam{Name: param, Type: fn.paramTypes[param]})
	}
	body, err := c.lowerStatementsHIR(fn.astBody)
	if err != nil {
		return nil, err
	}
	lowered.Body = body
	return lowered, nil
}

func (c *Compiler) lowerStatementsHIR(stmts []pyriteStmt) ([]pyriteHIRStmt, error) {
	lowered := make([]pyriteHIRStmt, 0, len(stmts))
	for _, stmt := range stmts {
		item, err := c.lowerStatementHIR(stmt)
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, item)
	}
	return lowered, nil
}

func (c *Compiler) lowerStatementHIR(stmt pyriteStmt) (pyriteHIRStmt, error) {
	base := stmt.stmtBase()
	item := pyriteHIRStmt{Line: base.Line}
	switch node := stmt.(type) {
	case *pyriteReturnStmt:
		item.Kind = "return"
		item.Expr = node.Value
	case *pyriteRaiseStmt:
		item.Kind = "raise"
		item.Expr = node.Value
	case *pyriteIfStmt:
		item.Kind = "if"
		item.Expr = node.Condition
		if node.Inline != nil {
			inline, err := c.lowerStatementHIR(node.Inline)
			if err != nil {
				return item, err
			}
			item.Inline = []pyriteHIRStmt{inline}
		}
	case *pyriteWhileStmt:
		item.Kind = "while"
		item.Expr = node.Condition
		if node.Inline != nil {
			inline, err := c.lowerStatementHIR(node.Inline)
			if err != nil {
				return item, err
			}
			item.Inline = []pyriteHIRStmt{inline}
		}
	case *pyriteForStmt:
		item.Kind = "for"
		item.Name = node.Target
		item.Expr = node.Iterable
	case *pyriteMatchStmt:
		item.Kind = "match"
		item.Expr = node.Value
	case *pyriteCaseStmt:
		item.Kind = "case"
		item.Expr = node.Pattern
		item.Wildcard = node.Wildcard
		if node.Inline != nil {
			inline, err := c.lowerStatementHIR(node.Inline)
			if err != nil {
				return item, err
			}
			item.Inline = []pyriteHIRStmt{inline}
		}
	case *pyriteControlStmt:
		item.Kind = node.Kind
	case *pyriteExceptStmt:
		item.Kind = "except"
		item.Name = node.Name
	case *pyriteVarStmt:
		item.Kind = "var"
		item.Name = node.Name
		item.Type = node.Type
		item.Expr = node.Value
	case *pyriteAssignStmt:
		item.Kind = "assign"
		item.TargetExpr = node.TargetExpr
		item.Expr = node.Value
		if target, ok := assignmentTargetName(node.TargetExpr); ok {
			item.Target = target
		}
	case *pyriteExprStmt:
		item.Kind = "expr"
		item.Expr = node.Expr
	default:
		item.Kind = "unknown"
	}
	children, err := c.lowerStatementsHIR(base.Children)
	if err != nil {
		return item, err
	}
	item.Children = children
	return item, nil
}
