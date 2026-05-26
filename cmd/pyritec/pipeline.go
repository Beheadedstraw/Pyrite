package main

import "fmt"

type compilerPipeline struct {
	compiler *Compiler
	source   string
}

func (p *compilerPipeline) run() error {
	if err := p.loadAndBind(); err != nil {
		return err
	}
	if err := p.analyze(); err != nil {
		return err
	}
	return p.emit()
}

func (p *compilerPipeline) loadAndBind() error {
	return p.compiler.collectSourcePath(p.source, "", p.compiler.srcPath)
}

func (p *compilerPipeline) analyze() error {
	c := p.compiler
	if err := c.inferClassFields(); err != nil {
		return err
	}
	c.globalTypes = copyStringMap(c.types)
	if err := c.validateProgramSemantics(); err != nil {
		return err
	}
	return nil
}

func (p *compilerPipeline) emit() error {
	c := p.compiler
	mainDef := c.functions["main"]
	if err := c.compileMain(mainDef); err != nil {
		return err
	}
	for _, name := range c.functionOrder {
		if name == "main" {
			continue
		}
		fn := c.functions[name]
		if fn.nativeSymbol != "" {
			continue
		}
		if err := c.inferFunctionReturn(fn); err != nil {
			return err
		}
	}
	hir, err := c.lowerHIR()
	if err != nil {
		return err
	}
	c.hir = hir
	c.emitFunctionPrototypes()
	c.emitClassConstructors()
	for _, name := range c.functionOrder {
		if name == "main" {
			continue
		}
		fn := c.functions[name]
		if fn.nativeSymbol != "" {
			continue
		}
		if err := c.compileFunction(fn); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) validateProgramSemantics() error {
	mainDef := c.functions["main"]
	if mainDef == nil {
		return fmt.Errorf("missing def main()")
	}
	if len(mainDef.params) != 0 {
		return fmt.Errorf("def main() cannot take parameters yet")
	}
	for _, name := range c.functionOrder {
		if err := c.validateFunctionSemantics(c.functions[name]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) validateFunctionSemantics(fn *functionDef) error {
	for _, stmt := range fn.astBody {
		if err := c.validateStatementSemantics(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) validateStatementSemantics(stmt pyriteStmt) error {
	switch node := stmt.(type) {
	case *pyriteVarStmt:
		if !isIdentifier(node.Name) {
			return fmt.Errorf("line %d: invalid variable name %q", node.Line, node.Name)
		}
		if _, err := normalizeType(node.Type); node.Type != "" && err != nil {
			return fmt.Errorf("line %d: %w", node.Line, err)
		}
	case *pyriteAssignStmt:
		if _, err := mustAssignmentTargetName(node.Line, node.TargetExpr); err != nil {
			return err
		}
	case *pyriteForStmt:
		if !isIdentifier(node.Target) {
			return fmt.Errorf("line %d: invalid loop variable %q", node.Line, node.Target)
		}
	case *pyriteControlStmt:
		switch node.Kind {
		case "", "default", "else", "try":
		default:
			return fmt.Errorf("line %d: unsupported control statement %q", node.Line, node.Kind)
		}
	}
	for _, child := range stmt.stmtBase().Children {
		if err := c.validateStatementSemantics(child); err != nil {
			return err
		}
	}
	return nil
}
