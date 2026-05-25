package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var defHeaderRE = regexp.MustCompile(`^def ([A-Za-z_][A-Za-z0-9_]*)\((.*)\):$`)
var nativeDefHeaderRE = regexp.MustCompile(`^native def ([A-Za-z_][A-Za-z0-9_]*)\((.*)\) -> ([A-Za-z_\[\]]+) = ([A-Za-z_][A-Za-z0-9_]*)$`)
var classHeaderRE = regexp.MustCompile(`^class ([A-Za-z_][A-Za-z0-9_]*):$`)

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
	scanner := bufio.NewScanner(strings.NewReader(source))
	var current *functionDef
	var currentClass *classDef
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := countIndent(raw)

		if current != nil && indent > current.indent {
			current.body = append(current.body, sourceLine{lineNo: lineNo, raw: raw, trimmed: trimmed, indent: indent})
			continue
		}
		current = nil
		if currentClass != nil && indent <= currentClass.indent {
			currentClass = nil
		}

		switch {
		case currentClass != nil && strings.HasPrefix(trimmed, "def "):
			fn, err := parseFunctionDef(lineNo, trimmed, indent)
			if err != nil {
				return err
			}
			fn.name = currentClass.name + "." + fn.name
			if len(fn.params) == 0 || fn.params[0] != "self" {
				return fmt.Errorf("line %d: class methods need self as first parameter", lineNo)
			}
			fn.paramTypes["self"] = classKind(currentClass.name)
			if _, exists := c.functions[fn.name]; exists {
				return fmt.Errorf("line %d: duplicate function %s", lineNo, fn.name)
			}
			c.functions[fn.name] = fn
			c.functionOrder = append(c.functionOrder, fn.name)
			currentClass.methods[strings.TrimPrefix(fn.name, currentClass.name+".")] = fn
			current = fn
		case strings.HasPrefix(trimmed, "import "):
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "import "))
			c.imports[name] = true
			if err := c.loadModule(name); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
		case strings.HasPrefix(trimmed, "native def "):
			fn, err := parseNativeFunctionDef(lineNo, trimmed, indent, moduleName)
			if err != nil {
				return err
			}
			if _, exists := c.functions[fn.name]; exists {
				return fmt.Errorf("line %d: duplicate function %s", lineNo, fn.name)
			}
			c.functions[fn.name] = fn
			c.functionOrder = append(c.functionOrder, fn.name)
		case strings.HasPrefix(trimmed, "global "):
			if moduleName != "" {
				return fmt.Errorf("line %d: module globals are not supported yet", lineNo)
			}
			if err := c.emitGlobal(lineNo, strings.TrimSpace(strings.TrimPrefix(trimmed, "global ")), false); err != nil {
				return err
			}
		case strings.HasPrefix(trimmed, "const "):
			if moduleName != "" {
				return fmt.Errorf("line %d: module globals are not supported yet", lineNo)
			}
			if err := c.emitGlobal(lineNo, strings.TrimSpace(strings.TrimPrefix(trimmed, "const ")), true); err != nil {
				return err
			}
		case strings.HasPrefix(trimmed, "class "):
			if moduleName != "" {
				return fmt.Errorf("line %d: module classes are not supported yet", lineNo)
			}
			cls, err := parseClassDef(lineNo, trimmed, indent)
			if err != nil {
				return err
			}
			if _, exists := c.classes[cls.name]; exists {
				return fmt.Errorf("line %d: duplicate class %s", lineNo, cls.name)
			}
			c.classes[cls.name] = cls
			currentClass = cls
		case strings.HasPrefix(trimmed, "def "):
			fn, err := parseFunctionDef(lineNo, trimmed, indent)
			if err != nil {
				return err
			}
			if moduleName != "" {
				fn.name = moduleName + "." + fn.name
			}
			if _, exists := c.functions[fn.name]; exists {
				return fmt.Errorf("line %d: duplicate function %s", lineNo, fn.name)
			}
			c.functions[fn.name] = fn
			c.functionOrder = append(c.functionOrder, fn.name)
			current = fn
		case moduleName != "" && strings.Contains(trimmed, "="):
			if err := c.emitModuleGlobal(lineNo, moduleName, trimmed); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: statement outside function", lineNo)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
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
			for _, line := range fn.body {
				if !strings.HasPrefix(line.trimmed, "self.") || !strings.Contains(line.trimmed, "=") {
					continue
				}
				parts := strings.SplitN(line.trimmed, "=", 2)
				field := strings.TrimSpace(strings.TrimPrefix(parts[0], "self."))
				if field == "" || strings.ContainsAny(field, ". \t") {
					return fmt.Errorf("line %d: invalid class field assignment", line.lineNo)
				}
				_, kind, err := c.expr(parts[1])
				if err != nil {
					return fmt.Errorf("line %d: %w", line.lineNo, err)
				}
				if existing := cls.fields[field]; existing != "" && !typesCompatible(existing, kind) {
					return fmt.Errorf("line %d: class %s field %s is both %s and %s", line.lineNo, cls.name, field, existing, kind)
				}
				cls.fields[field] = kind
				c.types["self."+field] = kind
			}
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
	c.currentFunction = "main"
	defer func() { c.currentFunction = "" }()
	if c.target == "freestanding" {
		c.body.WriteString("long kmain(void) {\n")
	} else {
		c.body.WriteString("int main(void) {\n")
	}
	c.body.WriteString("    char *__pyrite_error __attribute__((unused)) = NULL;\n")
	if err := c.compileLines(fn.body, fn.indent); err != nil {
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

	var body bytes.Buffer
	c.body = body
	c.currentFunction = fn.name
	c.types = copyStringMap(c.globalTypes)
	c.consts = map[string]bool{}
	c.defers = nil
	c.blockStack = nil
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
	err := c.compileLines(fn.body, fn.indent)
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
	return err
}

func (c *Compiler) compileLines(lines []sourceLine, baseIndent int) error {
	c.mainIndent = baseIndent
	for _, line := range lines {
		if err := c.closeBlocksForLine(line.lineNo, line.indent, line.trimmed); err != nil {
			return err
		}
		if err := c.emitStatement(line.lineNo, line.indent, line.trimmed); err != nil {
			return err
		}
	}
	for len(c.blockStack) > 0 {
		if err := c.closeBlock(linesLastLine(lines)); err != nil {
			return err
		}
	}
	return nil
}

func linesLastLine(lines []sourceLine) int {
	if len(lines) == 0 {
		return 0
	}
	return lines[len(lines)-1].lineNo
}

func (c *Compiler) closeBlocksForLine(lineNo, indent int, trimmed string) error {
	for len(c.blockStack) > 0 {
		top := c.blockStack[len(c.blockStack)-1]
		if indent > top.indent {
			return nil
		}
		if indent == top.indent && top.kind == "case" && isCaseHeader(trimmed) {
			if err := c.closeBlock(lineNo); err != nil {
				return err
			}
			continue
		}
		if indent == top.indent && continuesBlock(trimmed, top.kind) {
			return nil
		}
		if err := c.closeBlock(lineNo); err != nil {
			return err
		}
	}
	return nil
}

func continuesBlock(trimmed, kind string) bool {
	return (kind == "if" && trimmed == "else:") || (kind == "try" && isExceptHeader(trimmed))
}

func isExceptHeader(s string) bool {
	return s == "except:" || (strings.HasPrefix(s, "except ") && strings.HasSuffix(s, ":"))
}

func isCaseHeader(s string) bool {
	return (strings.HasPrefix(s, "case ") && strings.HasSuffix(s, ":")) || s == "default:"
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
	c.types = copyStringMap(c.globalTypes)
	for _, param := range fn.params {
		if fn.paramTypes[param] == "" {
			return fmt.Errorf("function %s parameter %s has unknown type; add an annotation or call it with a typed value", fn.name, param)
		}
		c.types[param] = fn.paramTypes[param]
	}
	fn.returnType = "void"
	for _, line := range fn.body {
		if strings.HasPrefix(line.trimmed, "for ") && strings.HasSuffix(line.trimmed, ":") {
			m := regexp.MustCompile(`^for ([A-Za-z_][A-Za-z0-9_]*) in ([A-Za-z_][A-Za-z0-9_]*):$`).FindStringSubmatch(line.trimmed)
			if m != nil {
				switch c.types[m[2]] {
				case "list_int":
					c.types[m[1]] = "int"
				case "list_any":
					c.types[m[1]] = "any"
				}
			}
		}
		if strings.HasPrefix(line.trimmed, "foreach(") && strings.HasSuffix(line.trimmed, "):") {
			args := splitArgs(innerCall(strings.TrimSuffix(line.trimmed, ":"), "foreach"))
			if len(args) >= 1 && len(args) <= 2 {
				itemName := "item"
				if len(args) == 2 {
					itemName = strings.TrimSpace(args[1])
				}
				listName := strings.TrimSpace(args[0])
				switch c.types[listName] {
				case "list_int":
					c.types[itemName] = "int"
				case "list_any":
					c.types[itemName] = "any"
				}
			}
		}
		if strings.Contains(line.trimmed, "=") && !strings.HasPrefix(line.trimmed, "if ") && !strings.HasPrefix(line.trimmed, "while ") {
			parts := strings.SplitN(line.trimmed, "=", 2)
			target, err := parseBindingTarget(parts[0])
			if err != nil {
				c.types = oldTypes
				return fmt.Errorf("line %d: %w", line.lineNo, err)
			}
			if !strings.Contains(target.name, ".") {
				value := strings.TrimSpace(parts[1])
				if strings.HasSuffix(value, ").defer()") {
					value = strings.TrimSuffix(value, ".defer()")
				}
				_, kind, err := c.expr(value)
				if err != nil {
					c.types = oldTypes
					return fmt.Errorf("line %d: %w", line.lineNo, err)
				}
				if target.annotated != "" {
					if err := checkType(line.lineNo, target.name, target.annotated, kind); err != nil {
						c.types = oldTypes
						return err
					}
					kind = target.annotated
				}
				c.types[target.name] = kind
			}
		}
		if !strings.HasPrefix(line.trimmed, "return ") {
			continue
		}
		expr := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "return "))
		_, kind, err := c.expr(expr)
		if err != nil {
			c.types = oldTypes
			return fmt.Errorf("line %d: %w", line.lineNo, err)
		}
		if fn.returnType == "void" {
			fn.returnType = kind
		} else if !typesCompatible(fn.returnType, kind) {
			c.types = oldTypes
			return fmt.Errorf("line %d: function %s returns both %s and %s", line.lineNo, fn.name, fn.returnType, kind)
		}
	}
	c.types = oldTypes
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
