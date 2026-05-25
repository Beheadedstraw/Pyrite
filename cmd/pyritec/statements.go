package main

import (
	"fmt"
	"regexp"
	"strings"
)

func (c *Compiler) emitStatement(lineNo, indent int, s string) error {
	switch {
	case strings.HasPrefix(s, "const "):
		return c.emitAssign(lineNo, strings.TrimSpace(strings.TrimPrefix(s, "const ")), true)
	case strings.HasPrefix(s, "set "):
		return fmt.Errorf("line %d: use const for immutable bindings, not set", lineNo)
	case strings.HasPrefix(s, "print("):
		return c.emitPrint(lineNo, innerCall(s, "print"))
	case strings.HasPrefix(s, "routine("):
		return c.emitRoutine(lineNo, innerCall(s, "routine"))
	case strings.HasPrefix(s, "async("):
		return c.emitRoutine(lineNo, innerCall(s, "async"))
	case s == "try:":
		tryID := c.nextTryID
		c.nextTryID++
		c.blockStack = append(c.blockStack, block{kind: "try", indent: indent, tryID: tryID})
		return nil
	case isExceptHeader(s):
		if len(c.blockStack) == 0 || c.blockStack[len(c.blockStack)-1].kind != "try" {
			return fmt.Errorf("line %d: except without try", lineNo)
		}
		top := c.blockStack[len(c.blockStack)-1]
		name, err := parseExceptName(s)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    goto __pyrite_after_try_%d;\n", top.tryID))
		c.body.WriteString(fmt.Sprintf("__pyrite_except_%d:\n", top.tryID))
		c.body.WriteString("    ;\n")
		if name != "" {
			c.types[name] = "string"
			c.body.WriteString(fmt.Sprintf("        char *%s = __pyrite_error ? __pyrite_error : \"\";\n", name))
		}
		c.blockStack[len(c.blockStack)-1].kind = "except"
		return nil
	case strings.HasPrefix(s, "for ") && strings.HasSuffix(s, ":"):
		m := regexp.MustCompile(`^for ([A-Za-z_][A-Za-z0-9_]*) in ([A-Za-z_][A-Za-z0-9_]*):$`).FindStringSubmatch(s)
		if m == nil {
			return fmt.Errorf("line %d: unsupported for syntax", lineNo)
		}
		return c.emitListLoop(lineNo, indent, m[2], m[1])
	case strings.HasPrefix(s, "foreach(") && strings.HasSuffix(s, "):"):
		args := splitArgs(innerCall(strings.TrimSuffix(s, ":"), "foreach"))
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("line %d: foreach expects list and optional item name", lineNo)
		}
		itemName := "item"
		if len(args) == 2 {
			itemName = strings.TrimSpace(args[1])
			if !isIdentifier(itemName) {
				return fmt.Errorf("line %d: invalid foreach item name %q", lineNo, itemName)
			}
		}
		return c.emitForeachLoop(lineNo, indent, strings.TrimSpace(args[0]), itemName)
	case strings.HasPrefix(s, "while ") && strings.HasSuffix(s, ":"):
		cond := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "while ")), ":")
		code, err := c.condition(cond)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    while (%s) {\n", code))
		c.blockStack = append(c.blockStack, block{kind: "while", indent: indent})
		return nil
	case isInlineIf(s):
		return c.emitInlineIf(lineNo, s)
	case strings.HasPrefix(s, "switch ") && strings.HasSuffix(s, ":"):
		return c.emitSwitch(lineNo, indent, strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "switch ")), ":"))
	case strings.HasPrefix(s, "case ") && strings.HasSuffix(s, ":"):
		return c.emitCase(lineNo, indent, strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "case ")), ":"))
	case s == "default:":
		return c.emitDefault(lineNo, indent)
	case strings.HasPrefix(s, "if ") && strings.HasSuffix(s, ":"):
		cond := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "if ")), ":")
		code, err := c.condition(cond)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    if (%s) {\n", code))
		c.blockStack = append(c.blockStack, block{kind: "if", indent: indent})
		return nil
	case s == "else:":
		if len(c.blockStack) == 0 || c.blockStack[len(c.blockStack)-1].kind != "if" {
			return fmt.Errorf("line %d: else without if", lineNo)
		}
		c.body.WriteString("    } else {\n")
		return nil
	case strings.HasPrefix(s, "raise "):
		expr := strings.TrimSpace(strings.TrimPrefix(s, "raise "))
		code, kind, err := c.expr(expr)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if kind != "string" {
			return fmt.Errorf("line %d: raise expects a string", lineNo)
		}
		tryID, ok := c.activeTry()
		if !ok {
			return fmt.Errorf("line %d: raise without active try", lineNo)
		}
		c.body.WriteString(fmt.Sprintf("    __pyrite_error = %s;\n", code))
		c.body.WriteString(fmt.Sprintf("    goto __pyrite_except_%d;\n", tryID))
		return nil
	case strings.HasPrefix(s, "return "):
		expr := strings.TrimSpace(strings.TrimPrefix(s, "return "))
		code, kind, err := c.expr(expr)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if c.currentFunction == "main" {
			c.body.WriteString(fmt.Sprintf("    int __pyrite_return = (int)(%s);\n", code))
			c.emitDefers()
			c.body.WriteString("    return __pyrite_return;\n")
			return nil
		}
		fn := c.functions[c.currentFunction]
		if fn == nil {
			return fmt.Errorf("line %d: return outside function", lineNo)
		}
		want := fn.returnType
		if want == "any" && kind != "any" {
			anyCode, err := anyValue(code, kind)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			code = anyCode
		} else if want == "list_any" && kind == "list_int" {
			code = fmt.Sprintf("pyrite_list_int_to_any(%s)", code)
		} else if !typesCompatible(want, kind) {
			return fmt.Errorf("line %d: function %s returns %s but got %s", lineNo, c.currentFunction, want, kind)
		}
		c.body.WriteString(fmt.Sprintf("    return %s;\n", code))
		return nil
	case c.isUserFunctionCall(s):
		return c.emitUserFunctionCall(lineNo, s)
	case c.isClassMethodStatement(s):
		code, _, _, err := c.classMethodCall(s)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    %s;\n", code))
		return nil
	case strings.Contains(s, "="):
		return c.emitAssign(lineNo, s, false)
	default:
		return fmt.Errorf("line %d: unsupported statement %q", lineNo, s)
	}
	return nil
}

func isInlineIf(s string) bool {
	if !strings.HasPrefix(s, "if ") {
		return false
	}
	idx := strings.Index(s, ":")
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(s[idx+1:]) != ""
}

func (c *Compiler) emitInlineIf(lineNo int, s string) error {
	bodyStart := strings.Index(s, ":")
	cond := strings.TrimSpace(strings.TrimPrefix(s[:bodyStart], "if "))
	tail := strings.TrimSpace(s[bodyStart+1:])
	code, err := c.condition(cond)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	c.body.WriteString(fmt.Sprintf("    if (%s) {\n", code))
	if err := c.emitStatement(lineNo, 0, tail); err != nil {
		return err
	}
	c.body.WriteString("    }\n")
	return nil
}

func (c *Compiler) emitAssign(lineNo int, s string, immutable bool) error {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid assignment", lineNo)
	}
	target, err := parseBindingTarget(parts[0])
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	name := target.name
	value := strings.TrimSpace(parts[1])
	if c.consts[name] {
		return fmt.Errorf("line %d: cannot reassign immutable %s", lineNo, name)
	}

	cName := name
	if strings.Contains(name, ".") {
		if _, exists := c.types[name]; !exists {
			return c.emitMemberAssign(lineNo, name, value)
		}
		cName = c.variableCName(name)
	}

	if strings.HasSuffix(value, ").defer()") {
		openCall := strings.TrimSuffix(value, ".defer()")
		code, kind, err := c.expr(openCall)
		if err == nil && isDeferredResource(kind) {
			if err := checkType(lineNo, name, target.annotated, kind); err != nil {
				return err
			}
			if existing := c.types[name]; existing != "" && !typesCompatible(existing, kind) {
				return fmt.Errorf("line %d: cannot assign %s to %s previously inferred as %s", lineNo, kind, name, existing)
			}
			decl := c.decl(name, kind)
			c.types[name] = kind
			c.body.WriteString(fmt.Sprintf("    %s%s = %s;\n", decl, name, code))
			c.emitResourceOpenCheck(name, kind)
			c.defers = append(c.defers, c.resourceDefer(name, kind))
			return nil
		}
	}

	existingBefore := c.types[name]
	preReleased := false
	if existingBefore != "" && c.releasableKind(existingBefore) && existingBefore != "string" && !assignmentValueReferencesName(value, name) {
		c.emitReleaseValue(cName, existingBefore)
		preReleased = true
	}

	checkpoint := c.needsAssignmentCheckpoint(name, value)
	checkpointName := ""
	if checkpoint {
		checkpointName = fmt.Sprintf("__pyrite_checkpoint_%d", c.nextTempID)
		c.nextTempID++
		c.body.WriteString(fmt.Sprintf("    PyriteAllocation *%s = pyrite_checkpoint();\n", checkpointName))
	}
	code, kind, err := c.expr(value)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, name, target.annotated, kind); err != nil {
		return err
	}
	existing := c.types[name]
	if existing != "" && !typesCompatible(existing, kind) {
		return fmt.Errorf("line %d: cannot assign %s to %s previously inferred as %s", lineNo, kind, name, existing)
	}
	storeKind := kind
	if target.annotated != "" {
		storeKind = target.annotated
	} else if existing != "" {
		storeKind = existing
	}
	if storeKind == "any" && kind != "any" {
		code, err = anyValue(code, kind)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
	} else if storeKind == "list_any" && kind == "list_int" {
		code = fmt.Sprintf("pyrite_list_int_to_any(%s)", code)
	}
	decl := c.decl(name, storeKind)
	declared := decl != ""
	if immutable {
		c.consts[name] = true
	}
	if existing != "" && c.releasableKind(storeKind) && storeKind != "string" && !preReleased {
		c.emitReleaseValue(cName, storeKind)
	}
	c.types[name] = storeKind
	if storeKind == "string" {
		if decl != "" {
			c.body.WriteString(fmt.Sprintf("    %s%s = NULL;\n", decl, cName))
		}
		c.body.WriteString(fmt.Sprintf("    pyrite_assign_string(&%s, %s);\n", cName, code))
	} else if storeKind == "bytes" {
		c.body.WriteString(fmt.Sprintf("    %s%s = %s;\n", decl, cName, code))
	} else {
		c.body.WriteString(fmt.Sprintf("    %s%s = %s;\n", decl, cName, code))
	}
	if checkpoint {
		if storeKind == "list_any" {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since_any_list(%s, &%s);\n", checkpointName, cName))
		} else if storeKind == "bytes" {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since(%s, %s.items);\n", checkpointName, cName))
		} else if isClassKind(storeKind) {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since_class_object(%s, %s);\n", checkpointName, cName))
		} else {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since(%s, %s);\n", checkpointName, c.keepPointer(cName, storeKind)))
		}
		c.body.WriteString("    pyrite_temp_reset();\n")
	}
	if declared {
		c.registerBlockCleanup(cName, storeKind)
	}
	if isDeferredResource(storeKind) {
		c.emitResourceOpenCheck(cName, storeKind)
	}
	return nil
}

func assignmentValueReferencesName(value, name string) bool {
	if name == "" {
		return false
	}
	for _, token := range splitAssignmentTokens(value) {
		if token == name {
			return true
		}
	}
	return false
}

func splitAssignmentTokens(value string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (cur.Len() > 0 && r >= '0' && r <= '9') {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func (c *Compiler) needsAssignmentCheckpoint(name, value string) bool {
	if strings.Contains(value, "\"") || strings.Contains(value, ".") || strings.Contains(value, "f\"") || strings.Contains(value, "[") {
		return true
	}
	return c.types[name] == "string" || c.types[name] == "bytes" || c.types[name] == "list_any" || c.types[name] == "any"
}

func (c *Compiler) releasableKind(kind string) bool {
	return kind == "string" || kind == "bytes" || kind == "list_int" || kind == "list_any" || kind == "any"
}

func (c *Compiler) blockScopedReleasableKind(kind string) bool {
	return kind == "string" || kind == "bytes" || kind == "list_int" || kind == "list_any"
}

func (c *Compiler) registerBlockCleanup(name, kind string) {
	if len(c.blockStack) == 0 || !c.blockScopedReleasableKind(kind) {
		return
	}
	cleanup := ""
	switch kind {
	case "string":
		cleanup = fmt.Sprintf("    pyrite_release(%s);\n", name)
	case "bytes":
		cleanup = fmt.Sprintf("    pyrite_release(%s.items);\n", name)
	case "list_int":
		cleanup = fmt.Sprintf("    pyrite_release(%s.items);\n", name)
	case "list_any":
		cleanup = fmt.Sprintf("    pyrite_release_any_list(&%s);\n", name)
	}
	if cleanup == "" {
		return
	}
	top := &c.blockStack[len(c.blockStack)-1]
	top.cleanups = append(top.cleanups, cleanup)
}

func (c *Compiler) emitReleaseValue(name, kind string) {
	switch kind {
	case "string":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s);\n", name))
	case "bytes":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s.items);\n", name))
	case "list_int":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s.items);\n", name))
	case "list_any":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_any_list(&%s);\n", name))
	case "any":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_any(&%s);\n", name))
	}
}

func (c *Compiler) keepPointer(name, kind string) string {
	switch kind {
	case "string":
		return name
	case "bytes":
		return fmt.Sprintf("%s.items", name)
	case "file", "socket", "listener":
		return name
	case "list_int":
		return fmt.Sprintf("%s.items", name)
	case "list_any":
		return fmt.Sprintf("%s.items", name)
	case "any":
		return fmt.Sprintf("(%s.kind == PYRITE_ANY_STRING ? %s.as.s : NULL)", name, name)
	default:
		return "NULL"
	}
}

func isDeferredResource(kind string) bool {
	return kind == "file" || kind == "socket" || kind == "listener"
}

func (c *Compiler) emitListLoop(lineNo, indent int, listName, itemName string) error {
	if !isIdentifier(listName) {
		return fmt.Errorf("line %d: foreach currently expects a list variable", lineNo)
	}
	if c.types[listName] != "list_int" {
		if c.types[listName] != "list_any" {
			return fmt.Errorf("line %d: foreach currently supports lists", lineNo)
		}
		c.types[itemName] = "any"
		c.body.WriteString(fmt.Sprintf("    for (size_t __i_%s = 0; __i_%s < %s.len; __i_%s++) {\n", itemName, itemName, listName, itemName))
		c.body.WriteString(fmt.Sprintf("        PyriteAny %s = %s.items[__i_%s];\n", itemName, listName, itemName))
		c.blockStack = append(c.blockStack, block{kind: "for", indent: indent})
		return nil
	}
	c.types[itemName] = "int"
	c.body.WriteString(fmt.Sprintf("    for (size_t __i_%s = 0; __i_%s < %s.len; __i_%s++) {\n", itemName, itemName, listName, itemName))
	c.body.WriteString(fmt.Sprintf("        long %s = %s.items[__i_%s];\n", itemName, listName, itemName))
	c.blockStack = append(c.blockStack, block{kind: "for", indent: indent})
	return nil
}

func (c *Compiler) emitForeachLoop(lineNo, indent int, listExpr, itemName string) error {
	if isIdentifier(listExpr) {
		return c.emitListLoop(lineNo, indent, listExpr, itemName)
	}
	code, kind, err := c.expr(listExpr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if kind != "list_int" && kind != "list_any" {
		return fmt.Errorf("line %d: foreach expects a list, got %s", lineNo, kind)
	}
	c.nextTempID++
	tempName := fmt.Sprintf("__pyrite_foreach_%d", c.nextTempID)
	c.types[tempName] = kind
	c.body.WriteString(fmt.Sprintf("    %s %s = %s;\n", c.cType(kind), tempName, code))
	if err := c.emitListLoop(lineNo, indent, tempName, itemName); err != nil {
		return err
	}
	if kind == "list_any" && len(c.blockStack) > 0 {
		top := &c.blockStack[len(c.blockStack)-1]
		top.postCleanups = append(top.postCleanups, fmt.Sprintf("    pyrite_release_any_list(&%s);\n", tempName))
	}
	return nil
}

func (c *Compiler) emitSwitch(lineNo, indent int, expr string) error {
	code, kind, err := c.expr(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	switch kind {
	case "string", "int", "bool":
	default:
		return fmt.Errorf("line %d: switch supports string, int, and bool, got %s", lineNo, kind)
	}
	c.nextTempID++
	name := fmt.Sprintf("__pyrite_switch_%d", c.nextTempID)
	c.body.WriteString("    {\n")
	c.body.WriteString(fmt.Sprintf("    %s %s = %s;\n", c.cType(kind), name, code))
	c.blockStack = append(c.blockStack, block{kind: "switch", indent: indent, switchVar: name, switchKind: kind})
	return nil
}

func (c *Compiler) emitCase(lineNo, indent int, expr string) error {
	sw, idx, err := c.activeSwitch()
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if indent != sw.indent+4 {
		return fmt.Errorf("line %d: case must be indented one level under switch", lineNo)
	}
	code, kind, err := c.expr(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if !typesCompatible(sw.switchKind, kind) {
		return fmt.Errorf("line %d: case is %s but switch is %s", lineNo, kind, sw.switchKind)
	}
	prefix := "if"
	if sw.caseCount > 0 {
		prefix = "else if"
	}
	cond := fmt.Sprintf("%s == %s", sw.switchVar, code)
	if sw.switchKind == "string" {
		cond = fmt.Sprintf("strcmp(%s, %s) == 0", sw.switchVar, code)
	}
	c.body.WriteString(fmt.Sprintf("    %s (%s) {\n", prefix, cond))
	c.blockStack[idx].caseCount++
	c.blockStack = append(c.blockStack, block{kind: "case", indent: indent})
	return nil
}

func (c *Compiler) emitDefault(lineNo, indent int) error {
	sw, idx, err := c.activeSwitch()
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if indent != sw.indent+4 {
		return fmt.Errorf("line %d: default must be indented one level under switch", lineNo)
	}
	prefix := "if"
	cond := "1"
	if sw.caseCount > 0 {
		prefix = "else"
		cond = ""
	}
	if cond == "" {
		c.body.WriteString(fmt.Sprintf("    %s {\n", prefix))
	} else {
		c.body.WriteString(fmt.Sprintf("    %s (%s) {\n", prefix, cond))
	}
	c.blockStack[idx].caseCount++
	c.blockStack = append(c.blockStack, block{kind: "case", indent: indent})
	return nil
}

func (c *Compiler) activeSwitch() (block, int, error) {
	for i := len(c.blockStack) - 1; i >= 0; i-- {
		if c.blockStack[i].kind == "switch" {
			return c.blockStack[i], i, nil
		}
	}
	return block{}, -1, fmt.Errorf("case/default without switch")
}

func (c *Compiler) emitResourceOpenCheck(name, kind string) {
	switch kind {
	case "file":
		if tryID, ok := c.activeTry(); ok {
			c.body.WriteString(fmt.Sprintf("    if (!%s) { __pyrite_error = pyrite_last_error_or(\"file.open failed\"); goto __pyrite_except_%d; }\n", name, tryID))
		} else {
			c.body.WriteString(fmt.Sprintf("    if (!%s) { fprintf(stderr, \"%%s\\n\", pyrite_last_error_or(\"file.open failed\")); return 1; }\n", name))
		}
	case "socket":
		if tryID, ok := c.activeTry(); ok {
			c.body.WriteString(fmt.Sprintf("    if (!%s || %s->fd < 0) { __pyrite_error = pyrite_last_error_or(\"net.open failed\"); goto __pyrite_except_%d; }\n", name, name, tryID))
		} else {
			c.body.WriteString(fmt.Sprintf("    if (!%s || %s->fd < 0) { fprintf(stderr, \"%%s\\n\", pyrite_last_error_or(\"net.open failed\")); return 1; }\n", name, name))
		}
	case "listener":
		if tryID, ok := c.activeTry(); ok {
			c.body.WriteString(fmt.Sprintf("    if (!%s || %s->fd < 0) { __pyrite_error = pyrite_last_error_or(\"net.listen failed\"); goto __pyrite_except_%d; }\n", name, name, tryID))
		} else {
			c.body.WriteString(fmt.Sprintf("    if (!%s || %s->fd < 0) { fprintf(stderr, \"%%s\\n\", pyrite_last_error_or(\"net.listen failed\")); return 1; }\n", name, name))
		}
	}
}

func (c *Compiler) resourceDefer(name, kind string) string {
	switch kind {
	case "file":
		return fmt.Sprintf("    pyrite_defer_close_file(\"%s\", %s);\n", name, name)
	case "socket":
		return fmt.Sprintf("    pyrite_socket_close(\"%s\", %s);\n", name, name)
	case "listener":
		return fmt.Sprintf("    pyrite_listener_close(\"%s\", %s);\n", name, name)
	default:
		return ""
	}
}

func (c *Compiler) emitPrint(lineNo int, expr string) error {
	code, kind, err := c.expr(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	switch kind {
	case "any":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_any(%s);\n", code))
	case "int", "bool":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_int(%s);\n", code))
	case "float":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_float(%s);\n", code))
	case "list_int":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_list_int_string(&%s));\n", code))
	case "list_any":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_list_any_string(&%s));\n", code))
	case "bytes":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_bytes_string(&%s));\n", code))
	default:
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(%s);\n", code))
	}
	c.body.WriteString("    pyrite_temp_reset();\n")
	return nil
}

func (c *Compiler) emitRoutine(lineNo int, call string) error {
	args := splitArgs(call)
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("line %d: routine expects a call and optional mux", lineNo)
	}
	call = args[0]
	muxCode := "NULL"
	if len(args) == 2 {
		code, kind, err := c.expr(args[1])
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if kind != "mux" {
			return fmt.Errorf("line %d: routine mux argument must be mux, got %s", lineNo, kind)
		}
		muxCode = code
	}
	if !strings.HasPrefix(call, "print(") {
		if c.isUserFunctionCall(call) {
			return c.emitRoutineFunctionCall(lineNo, call, muxCode)
		}
		return fmt.Errorf("line %d: routine currently supports print(...) and helper function calls", lineNo)
	}
	code, kind, err := c.expr(innerCall(call, "print"))
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	switch kind {
	case "int", "bool":
		c.body.WriteString(fmt.Sprintf("    pyrite_routine_print_int(%s, %s);\n", code, muxCode))
	case "float":
		c.body.WriteString(fmt.Sprintf("    pyrite_routine_print_float(%s, %s);\n", code, muxCode))
	default:
		c.body.WriteString(fmt.Sprintf("    pyrite_routine_print_str(%s, %s);\n", code, muxCode))
	}
	return nil
}

func (c *Compiler) emitRoutineFunctionCall(lineNo int, call, muxCode string) error {
	name, rawArgs, ok := splitCall(call)
	if !ok {
		return fmt.Errorf("line %d: invalid routine function call", lineNo)
	}
	fn := c.functions[name]
	if fn == nil {
		return fmt.Errorf("line %d: unknown function %s", lineNo, name)
	}
	args := splitArgs(rawArgs)
	if len(args) != len(fn.params) {
		return fmt.Errorf("line %d: %s expects %d argument(s), got %d", lineNo, name, len(fn.params), len(args))
	}

	type argInfo struct {
		code string
		kind string
	}
	var infos []argInfo
	for i, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		param := fn.params[i]
		want := fn.paramTypes[param]
		if want == "" {
			fn.paramTypes[param] = kind
		} else if !typesCompatible(want, kind) {
			return fmt.Errorf("line %d: %s parameter %s is %s but got %s", lineNo, name, param, want, kind)
		}
		infos = append(infos, argInfo{code: code, kind: fn.paramTypes[param]})
	}
	if fn.returnType == "" {
		if err := c.inferFunctionReturn(fn); err != nil {
			return err
		}
	}

	id := c.nextRoutineID
	c.nextRoutineID++
	structName := fmt.Sprintf("PyriteRoutineCall_%d", id)
	runName := fmt.Sprintf("pyrite_routine_call_%d", id)

	c.funcs.WriteString(fmt.Sprintf("typedef struct %s {\n", structName))
	c.funcs.WriteString("    PyriteMux *mux;\n")
	for i, info := range infos {
		c.funcs.WriteString(fmt.Sprintf("    %s arg%d;\n", c.cType(info.kind), i))
	}
	c.funcs.WriteString(fmt.Sprintf("} %s;\n", structName))
	c.funcs.WriteString(fmt.Sprintf("static void *%s(void *arg) {\n", runName))
	c.funcs.WriteString(fmt.Sprintf("    %s *task = arg;\n", structName))
	c.funcs.WriteString("    pyrite_mux_lock(task->mux);\n")
	c.funcs.WriteString(fmt.Sprintf("    %s(", c.functionCName(name)))
	for i := range infos {
		if i > 0 {
			c.funcs.WriteString(", ")
		}
		c.funcs.WriteString(fmt.Sprintf("task->arg%d", i))
	}
	c.funcs.WriteString(");\n")
	c.funcs.WriteString("    pyrite_mux_unlock(task->mux);\n")
	c.funcs.WriteString("    pyrite_defer_memory_cleanup();\n")
	c.funcs.WriteString("    return NULL;\n")
	c.funcs.WriteString("}\n")

	c.body.WriteString(fmt.Sprintf("    %s *__routine_task_%d = pyrite_malloc(sizeof(%s));\n", structName, id, structName))
	c.body.WriteString(fmt.Sprintf("    if (__routine_task_%d) {\n", id))
	c.body.WriteString(fmt.Sprintf("        *__routine_task_%d = (%s){.mux = %s", id, structName, muxCode))
	for i, info := range infos {
		c.body.WriteString(fmt.Sprintf(", .arg%d = %s", i, info.code))
	}
	c.body.WriteString("};\n")
	c.body.WriteString(fmt.Sprintf("        pyrite_start_task(%s, __routine_task_%d);\n", runName, id))
	c.body.WriteString("    }\n")
	return nil
}

func (c *Compiler) isUserFunctionCall(s string) bool {
	name, _, ok := splitCall(s)
	if !ok || name == "main" {
		return false
	}
	_, exists := c.functions[name]
	return exists
}

func (c *Compiler) emitUserFunctionCall(lineNo int, s string) error {
	code, _, err := c.userFunctionCallExpr(lineNo, s)
	if err != nil {
		return err
	}
	c.body.WriteString(fmt.Sprintf("    %s;\n", code))
	return nil
}

func (c *Compiler) userFunctionCallExpr(lineNo int, s string) (string, string, error) {
	name, rawArgs, ok := splitCall(s)
	if !ok {
		return "", "", fmt.Errorf("line %d: invalid function call", lineNo)
	}
	fn := c.functions[name]
	if fn == nil {
		return "", "", fmt.Errorf("line %d: unknown function %s", lineNo, name)
	}
	args := splitArgs(rawArgs)
	if len(args) != len(fn.params) {
		return "", "", fmt.Errorf("line %d: %s expects %d argument(s), got %d", lineNo, name, len(fn.params), len(args))
	}
	var codes []string
	for i, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", fmt.Errorf("line %d: %w", lineNo, err)
		}
		param := fn.params[i]
		want := fn.paramTypes[param]
		if want == "" {
			fn.paramTypes[param] = kind
		} else if !typesCompatible(want, kind) {
			return "", "", fmt.Errorf("line %d: %s parameter %s is %s but got %s", lineNo, name, param, want, kind)
		}
		if want == "any" && kind != "any" {
			code, err = anyValue(code, kind)
			if err != nil {
				return "", "", fmt.Errorf("line %d: %w", lineNo, err)
			}
		} else if want == "list_any" && kind == "list_int" {
			code = fmt.Sprintf("pyrite_list_int_to_any(%s)", code)
		}
		codes = append(codes, code)
	}
	if fn.returnType == "" {
		if err := c.inferFunctionReturn(fn); err != nil {
			return "", "", err
		}
	}
	if fn.nativeSymbol != "" {
		return fmt.Sprintf("%s(%s)", fn.nativeSymbol, strings.Join(codes, ", ")), fn.returnType, nil
	}
	return fmt.Sprintf("%s(%s)", c.functionCName(name), strings.Join(codes, ", ")), fn.returnType, nil
}

func splitCall(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	open := strings.IndexByte(s, '(')
	if open <= 0 {
		return "", "", false
	}
	name := strings.TrimSpace(s[:open])
	if !isCallName(name) {
		return "", "", false
	}
	return name, strings.TrimSuffix(s[open+1:], ")"), true
}

func (c *Compiler) functionCName(name string) string {
	return "pyrite_fn_" + sanitizeCName(name)
}

func (c *Compiler) variableCName(name string) string {
	return "pyrite_var_" + sanitizeCName(name)
}

func sanitizeCName(name string) string {
	return strings.NewReplacer(".", "_").Replace(name)
}

func parseExceptName(s string) (string, error) {
	if s == "except:" {
		return "", nil
	}
	name := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "except ")), ":")
	if name == "" {
		return "", fmt.Errorf("except name cannot be empty")
	}
	if strings.ContainsAny(name, " \t") {
		return "", fmt.Errorf("unsupported except syntax")
	}
	return name, nil
}

func (c *Compiler) activeTry() (int, bool) {
	for i := len(c.blockStack) - 1; i >= 0; i-- {
		if c.blockStack[i].kind == "try" {
			return c.blockStack[i].tryID, true
		}
	}
	return 0, false
}

func (c *Compiler) emitGlobal(lineNo int, s string, immutable bool) error {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid global declaration", lineNo)
	}
	target, err := parseBindingTarget(parts[0])
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	name := target.name
	value := strings.TrimSpace(parts[1])
	if strings.HasPrefix(name, "const ") {
		immutable = true
		name = strings.TrimSpace(strings.TrimPrefix(name, "const "))
	}
	if c.consts[name] {
		return fmt.Errorf("line %d: cannot reassign immutable %s", lineNo, name)
	}
	code, kind, err := c.expr(value)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, name, target.annotated, kind); err != nil {
		return err
	}
	storeKind := kind
	if target.annotated != "" {
		storeKind = target.annotated
	}
	c.types[name] = storeKind
	if immutable {
		c.consts[name] = true
	}
	qualifier := ""
	if immutable {
		qualifier = "const "
	}
	switch storeKind {
	case "int":
		c.globals.WriteString(fmt.Sprintf("static %slong %s = %s;\n", qualifier, name, code))
	case "float":
		c.globals.WriteString(fmt.Sprintf("static %sdouble %s = %s;\n", qualifier, name, code))
	case "bool":
		c.globals.WriteString(fmt.Sprintf("static %sint %s = %s;\n", qualifier, name, code))
	case "string":
		c.globals.WriteString(fmt.Sprintf("static %schar *%s = %s;\n", qualifier, name, code))
	default:
		return fmt.Errorf("line %d: global %s cannot use %s yet", lineNo, name, storeKind)
	}
	return nil
}

func (c *Compiler) emitModuleGlobal(lineNo int, moduleName, s string) error {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: invalid module global declaration", lineNo)
	}
	target, err := parseBindingTarget(parts[0])
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if target.name == "" || strings.Contains(target.name, ".") {
		return fmt.Errorf("line %d: invalid module global name", lineNo)
	}
	sourceName := moduleName + "." + target.name
	value := strings.TrimSpace(parts[1])
	code, kind, err := c.expr(value)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, sourceName, target.annotated, kind); err != nil {
		return err
	}
	storeKind := kind
	if target.annotated != "" {
		storeKind = target.annotated
	}
	c.types[sourceName] = storeKind
	cName := c.variableCName(sourceName)
	switch storeKind {
	case "int":
		c.globals.WriteString(fmt.Sprintf("static long %s = %s;\n", cName, code))
	case "float":
		c.globals.WriteString(fmt.Sprintf("static double %s = %s;\n", cName, code))
	case "bool":
		c.globals.WriteString(fmt.Sprintf("static int %s = %s;\n", cName, code))
	case "string":
		c.globals.WriteString(fmt.Sprintf("static char *%s = %s;\n", cName, code))
	case "object":
		c.globals.WriteString(fmt.Sprintf("static PyriteObject %s = %s;\n", cName, code))
	default:
		return fmt.Errorf("line %d: module global %s cannot use %s yet", lineNo, sourceName, storeKind)
	}
	return nil
}

func (c *Compiler) emitMemberAssign(lineNo int, name, value string) error {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("line %d: unsupported member assignment", lineNo)
	}
	if isClassKind(c.types[parts[0]]) {
		cls := c.classes[classNameFromKind(c.types[parts[0]])]
		if cls == nil {
			return fmt.Errorf("line %d: unknown class %s", lineNo, classNameFromKind(c.types[parts[0]]))
		}
		fieldKind := cls.fields[parts[1]]
		code, kind, err := c.expr(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if fieldKind == "" {
			fieldKind = kind
			cls.fields[parts[1]] = kind
		}
		if !typesCompatible(fieldKind, kind) {
			return fmt.Errorf("line %d: class %s field %s is %s but got %s", lineNo, cls.name, parts[1], fieldKind, kind)
		}
		anyCode, err := anyValue(code, kind)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		c.body.WriteString(fmt.Sprintf("    pyrite_class_set(%s, \"%s\", %s);\n", parts[0], parts[1], anyCode))
		return nil
	}
	if c.types[parts[0]] != "object" {
		return fmt.Errorf("line %d: unsupported member assignment", lineNo)
	}
	code, kind, err := c.expr(value)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	switch parts[1] {
	case "name":
		if kind != "string" {
			return fmt.Errorf("line %d: name expects string", lineNo)
		}
		c.body.WriteString(fmt.Sprintf("    %s.name = %s; %s.has_name = 1;\n", parts[0], code, parts[0]))
	case "score":
		if kind != "int" {
			return fmt.Errorf("line %d: score expects int", lineNo)
		}
		c.body.WriteString(fmt.Sprintf("    %s.score = %s; %s.has_score = 1;\n", parts[0], code, parts[0]))
	default:
		return fmt.Errorf("line %d: unsupported member %s", lineNo, parts[1])
	}
	return nil
}

func (c *Compiler) decl(name, kind string) string {
	if _, exists := c.types[name]; exists {
		return ""
	}
	switch kind {
	case "int":
		return "long "
	case "float":
		return "double "
	case "bool":
		return "int "
	case "any":
		return "PyriteAny "
	case "string":
		return "char *"
	case "bytes":
		return "PyriteBytes "
	case "file":
		return "FILE *"
	case "mux":
		return "PyriteMux *"
	case "socket":
		return "PyriteSocket *"
	case "listener":
		return "PyriteListener *"
	case "list_int":
		return "PyriteList "
	case "list_any":
		return "PyriteAnyList "
	case "object":
		return "PyriteObject "
	default:
		if isClassKind(kind) {
			return "PyriteClassObject *"
		}
		return "long "
	}
}

func (c *Compiler) cType(kind string) string {
	switch kind {
	case "void":
		return "void"
	case "int":
		return "long"
	case "float":
		return "double"
	case "bool":
		return "int"
	case "any":
		return "PyriteAny"
	case "string":
		return "char *"
	case "bytes":
		return "PyriteBytes"
	case "mux":
		return "PyriteMux *"
	case "socket":
		return "PyriteSocket *"
	case "listener":
		return "PyriteListener *"
	case "file":
		return "FILE *"
	case "list_int":
		return "PyriteList"
	case "list_any":
		return "PyriteAnyList"
	case "object":
		return "PyriteObject"
	default:
		if isClassKind(kind) {
			return "PyriteClassObject *"
		}
		return "long"
	}
}

func (c *Compiler) isClassMethodStatement(s string) bool {
	_, _, _, ok := splitMethodCall(s)
	if !ok {
		return false
	}
	_, _, ok, err := c.classMethodCall(s)
	return ok && err == nil
}

func (c *Compiler) constructorCName(name string) string {
	return "pyrite_ctor_" + sanitizeCName(name)
}

func (c *Compiler) emitDefers() {
	for i := len(c.defers) - 1; i >= 0; i-- {
		c.body.WriteString(c.defers[i])
	}
	c.body.WriteString("    pyrite_join_routines();\n")
	c.body.WriteString("    pyrite_defer_memory_cleanup();\n")
	c.defers = nil
}
