package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var defHeaderRE = regexp.MustCompile(`^def ([A-Za-z_][A-Za-z0-9_]*)\((.*)\):$`)
var nativeDefHeaderRE = regexp.MustCompile(`^native def ([A-Za-z_][A-Za-z0-9_]*)\((.*)\) -> ([A-Za-z_][A-Za-z0-9_\[\]:]*) = ([A-Za-z_][A-Za-z0-9_]*)$`)
var classHeaderRE = regexp.MustCompile(`^class ([A-Za-z_][A-Za-z0-9_]*):$`)
var enumHeaderRE = regexp.MustCompile(`^enum ([A-Za-z_][A-Za-z0-9_]*):$`)

func (c *Compiler) translate(source string) error {
	if err := c.collectSource(source, ""); err != nil {
		return err
	}
	if err := c.inferClassFields(); err != nil {
		return err
	}
	c.globalTypes = copyStringMap(c.types)

	mainDef := c.functions["main"]
	if mainDef == nil {
		return fmt.Errorf("missing def main()")
	}
	if len(mainDef.params) != 0 {
		return fmt.Errorf("def main() cannot take parameters yet")
	}
	if err := c.compileMain(mainDef); err != nil {
		return err
	}
	for _, name := range c.functionOrder {
		if name == "main" {
			continue
		}
		if c.functions[name].nativeSymbol != "" {
			continue
		}
		if err := c.inferFunctionReturn(c.functions[name]); err != nil {
			return err
		}
	}
	c.emitFunctionPrototypes()
	c.emitClassConstructors()
	for _, name := range c.functionOrder {
		if name == "main" {
			continue
		}
		if c.functions[name].nativeSymbol != "" {
			continue
		}
		if err := c.compileFunction(c.functions[name]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) collectSource(source string, moduleName string) error {
	program, err := parsePyriteProgram(source)
	if err != nil {
		return err
	}
	for _, item := range program.Items {
		if err := c.collectTopLevelItem(item, moduleName); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) collectTopLevelItem(item pyriteTopLevel, moduleName string) error {
	switch node := item.(type) {
	case *pyriteImportDecl:
		c.imports[node.Name] = true
		if err := c.loadModule(node.Name); err != nil {
			return fmt.Errorf("line %d: %w", node.Line, err)
		}
	case *pyriteBindingDecl:
		if moduleName != "" && !node.Global && !node.Const {
			return c.emitModuleGlobalAST(node.Line, moduleName, node)
		}
		if moduleName != "" {
			return fmt.Errorf("line %d: module globals are not supported yet", node.Line)
		}
		if !node.Global && !node.Const {
			return fmt.Errorf("line %d: statement outside function", node.Line)
		}
		return c.emitGlobalAST(node.Line, node)
	case *pyriteFunctionDecl:
		fn, err := functionFromAST(node, moduleName, "")
		if err != nil {
			return err
		}
		return c.addFunction(fn, node.Line)
	case *pyriteClassDecl:
		if moduleName != "" {
			return fmt.Errorf("line %d: module classes are not supported yet", node.Line)
		}
		cls := &classDef{name: node.Name, indent: node.Indent, fields: map[string]string{}, methods: map[string]*functionDef{}}
		if _, exists := c.classes[cls.name]; exists {
			return fmt.Errorf("line %d: duplicate class %s", node.Line, cls.name)
		}
		c.classes[cls.name] = cls
		for _, method := range node.Methods {
			fn, err := functionFromAST(method, "", cls.name)
			if err != nil {
				return err
			}
			if len(fn.params) == 0 || fn.params[0] != "self" {
				return fmt.Errorf("line %d: class methods need self as first parameter", method.Line)
			}
			fn.paramTypes["self"] = classKind(cls.name)
			if err := c.addFunction(fn, method.Line); err != nil {
				return err
			}
			cls.methods[strings.TrimPrefix(fn.name, cls.name+".")] = fn
		}
	case *pyriteEnumDecl:
		if moduleName != "" {
			return fmt.Errorf("line %d: module enums are not supported yet", node.Line)
		}
		return c.collectEnum(node)
	default:
		return fmt.Errorf("unknown top-level declaration")
	}
	return nil
}

func (c *Compiler) addFunction(fn *functionDef, lineNo int) error {
	if _, exists := c.functions[fn.name]; exists {
		return fmt.Errorf("line %d: duplicate function %s", lineNo, fn.name)
	}
	c.functions[fn.name] = fn
	c.functionOrder = append(c.functionOrder, fn.name)
	return nil
}

func functionFromAST(decl *pyriteFunctionDecl, moduleName, className string) (*functionDef, error) {
	name := decl.Name
	if className != "" {
		name = className + "." + name
	} else if moduleName != "" {
		name = moduleName + "." + name
	}
	fn := &functionDef{
		name:         name,
		paramTypes:   map[string]string{},
		indent:       decl.Indent,
		astBody:      decl.Body,
		nativeSymbol: decl.NativeSymbol,
	}
	if decl.ReturnType != "" {
		returnType, err := normalizeType(decl.ReturnType)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", decl.Line, err)
		}
		fn.returnType = returnType
	}
	for _, param := range decl.Params {
		if param.Name == "" || strings.Contains(param.Name, ".") {
			return nil, fmt.Errorf("line %d: invalid parameter %q", decl.Line, param.Name)
		}
		fn.params = append(fn.params, param.Name)
		if param.Type != "" {
			kind, err := normalizeType(param.Type)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", decl.Line, err)
			}
			fn.paramTypes[param.Name] = kind
		}
		if decl.NativeSymbol != "" && fn.paramTypes[param.Name] == "" {
			return nil, fmt.Errorf("line %d: native parameter %q needs a type", decl.Line, param.Name)
		}
	}
	return fn, nil
}

func (c *Compiler) collectEnum(decl *pyriteEnumDecl) error {
	if _, exists := c.enums[decl.Name]; exists {
		return fmt.Errorf("line %d: duplicate enum %s", decl.Line, decl.Name)
	}
	c.enums[decl.Name] = map[string]int{}
	next := 0
	for _, member := range decl.Members {
		value := next
		if member.HasValue {
			parsed, err := strconv.Atoi(member.Value)
			if err != nil {
				return fmt.Errorf("line %d: enum value must be an integer", member.Line)
			}
			value = parsed
		}
		if !isIdentifier(member.Name) {
			return fmt.Errorf("line %d: invalid enum member %q", member.Line, member.Name)
		}
		c.enums[decl.Name][member.Name] = value
		cName := decl.Name + "." + member.Name
		c.types[cName] = "int"
		c.consts[cName] = true
		c.globals.WriteString(fmt.Sprintf("static const long %s = %d;\n", c.variableCName(cName), value))
		next = value + 1
	}
	return nil
}

func parseClassDef(lineNo int, trimmed string, indent int) (*classDef, error) {
	m := classHeaderRE.FindStringSubmatch(trimmed)
	if m == nil {
		return nil, fmt.Errorf("line %d: invalid class definition", lineNo)
	}
	return &classDef{
		name:    m[1],
		indent:  indent,
		fields:  map[string]string{},
		methods: map[string]*functionDef{},
	}, nil
}

func parseEnumDef(lineNo int, trimmed string) (string, error) {
	m := enumHeaderRE.FindStringSubmatch(trimmed)
	if m == nil {
		return "", fmt.Errorf("line %d: invalid enum definition", lineNo)
	}
	return m[1], nil
}

func (c *Compiler) loadModule(name string) error {
	if c.loadedModules[name] {
		return nil
	}
	if c.target == "freestanding" && freestandingBlockedModule(name) {
		return fmt.Errorf("module %s is hosted-only under --target freestanding", name)
	}
	c.loadedModules[name] = true
	for _, path := range []string{
		filepath.Join("stdlib", name+".pyr"),
		filepath.Join(filepath.Dir(c.srcPath), name+".pyr"),
		filepath.Join(filepath.Dir(c.srcPath), "drivers", name+".pyr"),
		filepath.Join(filepath.Dir(c.srcPath), "shell", name+".pyr"),
		filepath.Join(filepath.Dir(c.srcPath), "tools", name+".pyr"),
	} {
		if samePath(path, c.srcPath) {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			return c.collectSource(string(data), name)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("could not read module %s: %w", name, err)
		}
	}
	return nil
}

func freestandingBlockedModule(name string) bool {
	switch name {
	case "file", "floats", "http", "ints", "json", "net", "random", "regex", "routines", "strings", "time", "xml":
		return true
	default:
		return false
	}
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return leftAbs == rightAbs
}

func parseFunctionDef(lineNo int, trimmed string, indent int) (*functionDef, error) {
	m := defHeaderRE.FindStringSubmatch(trimmed)
	if m == nil {
		return nil, fmt.Errorf("line %d: invalid function definition", lineNo)
	}
	fn := &functionDef{
		name:       m[1],
		paramTypes: map[string]string{},
		indent:     indent,
	}
	params := splitArgs(m[2])
	for _, raw := range params {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		target, err := parseBindingTarget(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if target.name == "" || strings.Contains(target.name, ".") {
			return nil, fmt.Errorf("line %d: invalid parameter %q", lineNo, raw)
		}
		fn.params = append(fn.params, target.name)
		if target.annotated != "" {
			fn.paramTypes[target.name] = target.annotated
		}
	}
	return fn, nil
}

func (c *Compiler) inferClassFields() error {
	oldTypes := c.types
	defer func() { c.types = oldTypes }()
	for _, cls := range c.classes {
		var methods []*functionDef
		if init := cls.methods["__init__"]; init != nil {
			methods = append(methods, init)
		}
		for name, fn := range cls.methods {
			if name != "__init__" {
				methods = append(methods, fn)
			}
		}
		for _, fn := range methods {
			c.types = copyStringMap(c.globalTypes)
			for _, param := range fn.params {
				if fn.paramTypes[param] == "" {
					return fmt.Errorf("function %s parameter %s has unknown type; add an annotation", fn.name, param)
				}
				c.types[param] = fn.paramTypes[param]
			}
			for _, stmt := range fn.astBody {
				if err := c.inferClassFieldsFromStmt(cls, stmt); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Compiler) inferClassFieldsFromStmt(cls *classDef, stmt pyriteStmt) error {
	switch node := stmt.(type) {
	case *pyriteAssignStmt:
		if !strings.HasPrefix(node.Target, "self.") {
			break
		}
		field := strings.TrimSpace(strings.TrimPrefix(node.Target, "self."))
		if field == "" || strings.ContainsAny(field, ". \t") {
			return fmt.Errorf("line %d: invalid class field assignment", node.Line)
		}
		_, kind, err := c.exprAST(node.Value)
		if err != nil {
			return fmt.Errorf("line %d: %w", node.Line, err)
		}
		if existing := cls.fields[field]; existing != "" && !typesCompatible(existing, kind) {
			return fmt.Errorf("line %d: class %s field %s is both %s and %s", node.Line, cls.name, field, existing, kind)
		}
		cls.fields[field] = kind
		c.types["self."+field] = kind
	case *pyriteIfStmt:
		if node.Inline != nil {
			if err := c.inferClassFieldsFromStmt(cls, node.Inline); err != nil {
				return err
			}
		}
	case *pyriteWhileStmt:
		if node.Inline != nil {
			if err := c.inferClassFieldsFromStmt(cls, node.Inline); err != nil {
				return err
			}
		}
	case *pyriteCaseStmt:
		if node.Inline != nil {
			if err := c.inferClassFieldsFromStmt(cls, node.Inline); err != nil {
				return err
			}
		}
	}
	for _, child := range stmt.stmtBase().Children {
		if err := c.inferClassFieldsFromStmt(cls, child); err != nil {
			return err
		}
	}
	return nil
}

func parseNativeFunctionDef(lineNo int, trimmed string, indent int, moduleName string) (*functionDef, error) {
	m := nativeDefHeaderRE.FindStringSubmatch(trimmed)
	if m == nil {
		return nil, fmt.Errorf("line %d: invalid native function definition", lineNo)
	}
	returnType, err := normalizeType(m[3])
	if err != nil {
		return nil, fmt.Errorf("line %d: %w", lineNo, err)
	}
	name := m[1]
	if moduleName != "" {
		name = moduleName + "." + name
	}
	fn := &functionDef{
		name:         name,
		paramTypes:   map[string]string{},
		returnType:   returnType,
		indent:       indent,
		nativeSymbol: m[4],
	}
	params := splitArgs(m[2])
	for _, raw := range params {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		target, err := parseBindingTarget(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if target.name == "" || target.annotated == "" || strings.Contains(target.name, ".") {
			return nil, fmt.Errorf("line %d: native parameter %q needs a name and type", lineNo, raw)
		}
		fn.params = append(fn.params, target.name)
		fn.paramTypes[target.name] = target.annotated
	}
	return fn, nil
}

func (c *Compiler) compileMain(fn *functionDef) error {
	oldLocalDeclared := c.localDeclared
	c.currentFunction = "main"
	c.localDeclared = map[string]bool{}
	defer func() {
		c.currentFunction = ""
		c.localDeclared = oldLocalDeclared
	}()
	if c.target == "freestanding" {
		c.body.WriteString("long kmain(void) {\n")
	} else {
		c.body.WriteString("int main(void) {\n")
	}
	c.body.WriteString("    char *__pyrite_error __attribute__((unused)) = NULL;\n")
	if err := c.predeclareFunctionLocals(fn); err != nil {
		return err
	}
	if err := c.compileFunctionBody(fn); err != nil {
		return err
	}
	c.emitDefers()
	if c.target == "freestanding" {
		c.body.WriteString("    pyrite_kernel_halt();\n")
	} else {
		c.body.WriteString("    return 0;\n")
	}
	c.body.WriteString("}\n")
	return nil
}

func (c *Compiler) compileFunction(fn *functionDef) error {
	for _, param := range fn.params {
		if fn.paramTypes[param] == "" {
			return fmt.Errorf("function %s parameter %s has unknown type; add an annotation or call it with a typed value", fn.name, param)
		}
	}

	oldBody := c.body
	oldTypes := c.types
	oldConsts := c.consts
	oldDefers := c.defers
	oldBlocks := c.blockStack
	oldMainIndent := c.mainIndent
	oldCurrentFunction := c.currentFunction
	oldLocalDeclared := c.localDeclared

	var body bytes.Buffer
	c.body = body
	c.currentFunction = fn.name
	c.types = copyStringMap(c.globalTypes)
	c.consts = map[string]bool{}
	c.defers = nil
	c.blockStack = nil
	c.localDeclared = map[string]bool{}
	for _, param := range fn.params {
		c.types[param] = fn.paramTypes[param]
	}

	c.body.WriteString(fmt.Sprintf("static %s %s(", c.cType(fn.returnType), c.functionCName(fn.name)))
	for i, param := range fn.params {
		if i > 0 {
			c.body.WriteString(", ")
		}
		c.body.WriteString(fmt.Sprintf("%s %s", c.cType(fn.paramTypes[param]), param))
	}
	c.body.WriteString(") {\n")
	c.body.WriteString("    char *__pyrite_error __attribute__((unused)) = NULL;\n")
	if err := c.predeclareFunctionLocals(fn); err != nil {
		c.body = oldBody
		c.types = oldTypes
		c.consts = oldConsts
		c.defers = oldDefers
		c.blockStack = oldBlocks
		c.mainIndent = oldMainIndent
		c.currentFunction = oldCurrentFunction
		c.localDeclared = oldLocalDeclared
		return err
	}
	err := c.compileFunctionBody(fn)
	if err == nil {
		if fn.returnType == "void" {
			c.body.WriteString("    return;\n")
		}
		c.body.WriteString("}\n")
		c.funcs.WriteString(c.body.String())
	}

	c.body = oldBody
	c.types = oldTypes
	c.consts = oldConsts
	c.defers = oldDefers
	c.blockStack = oldBlocks
	c.mainIndent = oldMainIndent
	c.currentFunction = oldCurrentFunction
	c.localDeclared = oldLocalDeclared
	return err
}

func (c *Compiler) compileFunctionBody(fn *functionDef) error {
	return c.compileASTStatements(fn.astBody, fn.indent)
}

func (c *Compiler) compileASTStatements(stmts []pyriteStmt, baseIndent int) error {
	c.mainIndent = baseIndent
	for i := 0; i < len(stmts); i++ {
		stmt := stmts[i]
		if _, ok := stmt.(*pyriteControlStmt); ok && stmt.stmtBase().Text == "else:" {
			return fmt.Errorf("line %d: else without if", stmt.stmtBase().Line)
		}
		if _, ok := stmt.(*pyriteExceptStmt); ok {
			return fmt.Errorf("line %d: except without try", stmt.stmtBase().Line)
		}
		if ifStmt, ok := stmt.(*pyriteIfStmt); ok && len(ifStmt.Children) > 0 {
			next, hasElse := followingElse(stmts, i)
			if err := c.compileIfAST(ifStmt, next); err != nil {
				return err
			}
			if hasElse {
				i++
			}
			continue
		}
		if ctrl, ok := stmt.(*pyriteControlStmt); ok && ctrl.Kind == "try" && len(ctrl.Children) > 0 {
			next, hasExcept := followingExcept(stmts, i)
			if err := c.compileTryAST(ctrl, next); err != nil {
				return err
			}
			if hasExcept {
				i++
			}
			continue
		}
		if err := c.compileASTStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) compileASTStatement(stmt pyriteStmt) error {
	base := stmt.stmtBase()
	switch node := stmt.(type) {
	case *pyriteReturnStmt:
		if node.Value == nil {
			return fmt.Errorf("line %d: return expects a value", base.Line)
		}
		return c.emitReturnExpr(base.Line, node.Value)
	case *pyriteRaiseStmt:
		if node.Value == nil {
			return fmt.Errorf("line %d: raise expects a value", base.Line)
		}
		return c.emitRaiseExpr(base.Line, node.Value)
	case *pyriteIfStmt:
		if node.Inline != nil {
			if err := c.emitIfHeaderAST(base.Line, base.Indent, node.Condition); err != nil {
				return err
			}
			if err := c.compileASTStatement(node.Inline); err != nil {
				return err
			}
			return c.closeBlock(base.Line)
		}
		if err := c.emitIfHeaderAST(base.Line, base.Indent, node.Condition); err != nil {
			return err
		}
	case *pyriteWhileStmt:
		if node.Inline != nil {
			if err := c.emitWhileHeaderAST(base.Line, base.Indent, node.Condition); err != nil {
				return err
			}
			if err := c.compileASTStatement(node.Inline); err != nil {
				return err
			}
			return c.closeBlock(base.Line)
		}
		if err := c.emitWhileHeaderAST(base.Line, base.Indent, node.Condition); err != nil {
			return err
		}
	case *pyriteForStmt:
		if err := c.emitForHeaderAST(base.Line, base.Indent, node.Target, node.Iterable); err != nil {
			return err
		}
	case *pyriteMatchStmt:
		if err := c.emitSwitchAST(base.Line, base.Indent, node.Value); err != nil {
			return err
		}
	case *pyriteCaseStmt:
		if node.Wildcard {
			if err := c.emitDefault(base.Line, base.Indent); err != nil {
				return err
			}
		} else {
			if err := c.emitCaseAST(base.Line, base.Indent, node.Pattern); err != nil {
				return err
			}
		}
		if node.Inline != nil {
			if err := c.compileASTStatement(node.Inline); err != nil {
				return err
			}
			return c.closeBlock(base.Line)
		}
	case *pyriteControlStmt:
		switch node.Kind {
		case "default":
			if err := c.emitDefault(base.Line, base.Indent); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: unsupported control statement %q", base.Line, base.Text)
		}
	case *pyriteVarStmt:
		return c.emitAssignAST(base.Line, node.Name, node.Type, node.Value, false)
	case *pyriteAssignStmt:
		return c.emitAssignAST(base.Line, node.Target, "", node.Value, false)
	case *pyriteExprStmt:
		return c.emitExprStmtAST(base.Line, node.Expr)
	default:
		return fmt.Errorf("line %d: unsupported statement %q", base.Line, base.Text)
	}
	if len(base.Children) == 0 {
		switch stmt.(type) {
		case *pyriteIfStmt, *pyriteWhileStmt, *pyriteForStmt, *pyriteMatchStmt, *pyriteCaseStmt, *pyriteControlStmt, *pyriteExceptStmt:
			return c.closeBlock(base.Line)
		}
		return nil
	}
	if err := c.compileASTStatements(base.Children, c.mainIndent); err != nil {
		return err
	}
	return c.closeBlock(base.Line)
}

func (c *Compiler) emitExprStmtAST(lineNo int, expr pyriteExpr) error {
	call, ok := expr.(*pyriteCallExpr)
	if !ok {
		code, _, err := c.exprAST(expr)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    %s;\n", code))
		return nil
	}
	if callee, ok := call.Callee.(*pyriteNameExpr); ok && callee.Name == "print" {
		if len(call.Args) != 1 {
			return fmt.Errorf("line %d: print expects 1 argument(s)", lineNo)
		}
		return c.emitPrintExpr(lineNo, call.Args[0])
	}
	if callee, ok := call.Callee.(*pyriteNameExpr); ok && (callee.Name == "routine" || callee.Name == "async") {
		return c.emitRoutineAST(lineNo, call.Args)
	}
	code, _, err := c.exprAST(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	c.body.WriteString(fmt.Sprintf("    %s;\n", code))
	return nil
}

func (c *Compiler) compileIfAST(stmt *pyriteIfStmt, elseStmt pyriteStmt) error {
	base := stmt.stmtBase()
	if err := c.emitIfHeaderAST(base.Line, base.Indent, stmt.Condition); err != nil {
		return err
	}
	if err := c.compileASTStatements(base.Children, c.mainIndent); err != nil {
		return err
	}
	if elseStmt != nil {
		elseBase := elseStmt.stmtBase()
		if err := c.emitElseAST(elseBase.Line); err != nil {
			return err
		}
		if err := c.compileASTStatements(elseBase.Children, c.mainIndent); err != nil {
			return err
		}
	}
	return c.closeBlock(base.Line)
}

func (c *Compiler) compileTryAST(stmt *pyriteControlStmt, exceptStmt *pyriteExceptStmt) error {
	base := stmt.stmtBase()
	if err := c.emitTryHeaderAST(base.Indent); err != nil {
		return err
	}
	if err := c.compileASTStatements(base.Children, c.mainIndent); err != nil {
		return err
	}
	if exceptStmt == nil {
		return c.closeBlock(base.Line)
	}
	exceptBase := exceptStmt.stmtBase()
	if err := c.emitExceptHeaderAST(exceptBase.Line, exceptStmt.Name); err != nil {
		return err
	}
	if err := c.compileASTStatements(exceptBase.Children, c.mainIndent); err != nil {
		return err
	}
	return c.closeBlock(exceptBase.Line)
}

func followingElse(stmts []pyriteStmt, index int) (pyriteStmt, bool) {
	if index+1 >= len(stmts) {
		return nil, false
	}
	ctrl, ok := stmts[index+1].(*pyriteControlStmt)
	if ok && ctrl.Kind == "else" {
		return stmts[index+1], true
	}
	return nil, false
}

func followingExcept(stmts []pyriteStmt, index int) (*pyriteExceptStmt, bool) {
	if index+1 >= len(stmts) {
		return nil, false
	}
	stmt, ok := stmts[index+1].(*pyriteExceptStmt)
	if ok {
		return stmt, true
	}
	return nil, false
}

func (c *Compiler) closeBlock(lineNo int) error {
	top := c.blockStack[len(c.blockStack)-1]
	c.blockStack = c.blockStack[:len(c.blockStack)-1]
	switch top.kind {
	case "if", "for", "while", "case":
		c.emitBlockCleanups(top)
		c.body.WriteString("    }\n")
		c.emitPostBlockCleanups(top)
	case "switch":
		c.body.WriteString("    }\n")
	case "try":
		return fmt.Errorf("line %d: try without except", lineNo)
	case "except":
		c.emitBlockCleanups(top)
		c.body.WriteString(fmt.Sprintf("__pyrite_after_try_%d:\n", top.tryID))
		c.emitPostBlockCleanups(top)
	default:
		return fmt.Errorf("line %d: internal compiler error: unknown block %s", lineNo, top.kind)
	}
	return nil
}

func (c *Compiler) emitBlockCleanups(top block) {
	for i := len(top.cleanups) - 1; i >= 0; i-- {
		c.body.WriteString(top.cleanups[i])
	}
}

func (c *Compiler) emitPostBlockCleanups(top block) {
	for i := len(top.postCleanups) - 1; i >= 0; i-- {
		c.body.WriteString(top.postCleanups[i])
	}
}

func (c *Compiler) emitFunctionPrototypes() {
	for _, name := range c.functionOrder {
		if name == "main" {
			continue
		}
		fn := c.functions[name]
		if fn.nativeSymbol != "" {
			continue
		}
		c.prototypes.WriteString(fmt.Sprintf("static %s %s(", c.cType(fn.returnType), c.functionCName(fn.name)))
		for i, param := range fn.params {
			if i > 0 {
				c.prototypes.WriteString(", ")
			}
			c.prototypes.WriteString(fmt.Sprintf("%s %s", c.cType(fn.paramTypes[param]), param))
		}
		c.prototypes.WriteString(");\n")
	}
}

func (c *Compiler) emitClassConstructors() {
	for _, cls := range c.classes {
		init := cls.methods["__init__"]
		c.prototypes.WriteString(fmt.Sprintf("static PyriteClassObject *%s(", c.constructorCName(cls.name)))
		c.writeConstructorParams(&c.prototypes, init)
		c.prototypes.WriteString(");\n")

		c.funcs.WriteString(fmt.Sprintf("static PyriteClassObject *%s(", c.constructorCName(cls.name)))
		c.writeConstructorParams(&c.funcs, init)
		c.funcs.WriteString(") {\n")
		c.funcs.WriteString(fmt.Sprintf("    PyriteClassObject *self = pyrite_class_new(\"%s\");\n", cls.name))
		if init != nil {
			c.funcs.WriteString("    if (self) {\n")
			c.funcs.WriteString(fmt.Sprintf("        %s(self", c.functionCName(init.name)))
			for _, param := range init.params[1:] {
				c.funcs.WriteString(fmt.Sprintf(", %s", param))
			}
			c.funcs.WriteString(");\n")
			c.funcs.WriteString("    }\n")
		}
		c.funcs.WriteString("    return self;\n")
		c.funcs.WriteString("}\n")
	}
}

func (c *Compiler) writeConstructorParams(buf *bytes.Buffer, init *functionDef) {
	if init == nil || len(init.params) <= 1 {
		buf.WriteString("void")
		return
	}
	for i, param := range init.params[1:] {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("%s %s", c.cType(init.paramTypes[param]), param))
	}
}

func (c *Compiler) inferFunctionReturn(fn *functionDef) error {
	oldTypes := c.types
	defer func() {
		c.types = oldTypes
	}()
	c.types = copyStringMap(c.globalTypes)
	for _, param := range fn.params {
		if fn.paramTypes[param] == "" {
			return fmt.Errorf("function %s parameter %s has unknown type; add an annotation or call it with a typed value", fn.name, param)
		}
		c.types[param] = fn.paramTypes[param]
	}
	fn.returnType = "void"
	for _, stmt := range fn.astBody {
		if err := c.inferFunctionStmt(fn, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) inferFunctionStmt(fn *functionDef, stmt pyriteStmt) error {
	switch node := stmt.(type) {
	case *pyriteVarStmt:
		return c.noteFunctionBinding(node.Line, node.Name, node.Type, node.Value)
	case *pyriteAssignStmt:
		return c.noteFunctionBinding(node.Line, node.Target, "", node.Value)
	case *pyriteReturnStmt:
		return c.noteFunctionReturnAST(fn, node.Line, node.Value)
	case *pyriteForStmt:
		if err := c.noteLoopBinding(node.Line, node.Target, node.Iterable); err != nil {
			return err
		}
	case *pyriteIfStmt:
		if node.Inline != nil {
			if err := c.inferFunctionStmt(fn, node.Inline); err != nil {
				return err
			}
		}
	case *pyriteWhileStmt:
		if node.Inline != nil {
			if err := c.inferFunctionStmt(fn, node.Inline); err != nil {
				return err
			}
		}
	case *pyriteCaseStmt:
		if node.Inline != nil {
			if err := c.inferFunctionStmt(fn, node.Inline); err != nil {
				return err
			}
		}
	}
	for _, child := range stmt.stmtBase().Children {
		if err := c.inferFunctionStmt(fn, child); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) noteFunctionBinding(lineNo int, name, annotated string, value pyriteExpr) error {
	if name == "" || strings.Contains(name, ".") {
		return nil
	}
	_, kind, err := c.exprAST(value)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if annotated != "" {
		normalized, err := normalizeType(annotated)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if err := checkType(lineNo, name, normalized, kind); err != nil {
			return err
		}
		kind = normalized
	}
	c.types[name] = kind
	return nil
}

func (c *Compiler) noteLoopBinding(lineNo int, name string, iterable pyriteExpr) error {
	if !isIdentifier(name) {
		return nil
	}
	_, kind, err := c.exprAST(iterable)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if !isListKind(kind) {
		return fmt.Errorf("line %d: foreach expects a list, got %s", lineNo, kind)
	}
	c.types[name] = listElementKind(kind)
	return nil
}

func (c *Compiler) noteFunctionReturnAST(fn *functionDef, lineNo int, expr pyriteExpr) error {
	if expr == nil {
		return nil
	}
	_, kind, err := c.exprAST(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if fn.returnType == "void" {
		fn.returnType = kind
	} else if !typesCompatible(fn.returnType, kind) {
		return fmt.Errorf("line %d: function %s returns both %s and %s", lineNo, fn.name, fn.returnType, kind)
	}
	return nil
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func countIndent(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
			continue
		}
		if r == '\t' {
			n += 4
			continue
		}
		break
	}
	return n
}
