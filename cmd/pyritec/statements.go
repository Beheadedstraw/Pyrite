package main

import (
	"fmt"
	"strings"
)

func (c *Compiler) emitReturnExpr(lineNo int, expr pyriteExpr) error {
	code, kind, err := c.exprAST(expr)
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
}

func (c *Compiler) emitRaiseExpr(lineNo int, expr pyriteExpr) error {
	code, kind, err := c.exprAST(expr)
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
}

func (c *Compiler) emitIfHeaderAST(lineNo, indent int, condition pyriteExpr) error {
	code, err := c.conditionAST(condition)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	c.body.WriteString(fmt.Sprintf("    if (%s) {\n", code))
	c.blockStack = append(c.blockStack, block{kind: "if", indent: indent})
	return nil
}

func (c *Compiler) emitWhileHeaderAST(lineNo, indent int, condition pyriteExpr) error {
	code, err := c.conditionAST(condition)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	c.body.WriteString(fmt.Sprintf("    while (%s) {\n", code))
	c.blockStack = append(c.blockStack, block{kind: "while", indent: indent})
	return nil
}

func (c *Compiler) emitElseAST(lineNo int) error {
	if len(c.blockStack) == 0 || c.blockStack[len(c.blockStack)-1].kind != "if" {
		return fmt.Errorf("line %d: else without if", lineNo)
	}
	c.body.WriteString("    } else {\n")
	return nil
}

func (c *Compiler) emitTryHeaderAST(indent int) error {
	tryID := c.nextTryID
	c.nextTryID++
	c.blockStack = append(c.blockStack, block{kind: "try", indent: indent, tryID: tryID})
	return nil
}

func (c *Compiler) emitExceptHeaderAST(lineNo int, name string) error {
	if len(c.blockStack) == 0 || c.blockStack[len(c.blockStack)-1].kind != "try" {
		return fmt.Errorf("line %d: except without try", lineNo)
	}
	top := c.blockStack[len(c.blockStack)-1]
	c.body.WriteString(fmt.Sprintf("    goto __pyrite_after_try_%d;\n", top.tryID))
	c.body.WriteString(fmt.Sprintf("__pyrite_except_%d:\n", top.tryID))
	c.body.WriteString("    ;\n")
	if name != "" {
		c.types[name] = "string"
		c.body.WriteString(fmt.Sprintf("        char *%s = __pyrite_error ? __pyrite_error : \"\";\n", name))
	}
	c.blockStack[len(c.blockStack)-1].kind = "except"
	return nil
}

func (c *Compiler) emitAssignAST(lineNo int, name, annotated string, valueExpr pyriteExpr, immutable bool) error {
	if name == "" {
		return fmt.Errorf("line %d: invalid assignment target", lineNo)
	}
	annotation := ""
	if annotated != "" {
		kind, err := normalizeType(annotated)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		annotation = kind
	}
	if c.consts[name] {
		return fmt.Errorf("line %d: cannot reassign immutable %s", lineNo, name)
	}

	cName := name
	if strings.Contains(name, ".") {
		if _, exists := c.types[name]; !exists {
			return c.emitMemberAssignAST(lineNo, name, valueExpr)
		}
		cName = c.variableCName(name)
	}

	if openExpr, ok := deferredResourceExpr(valueExpr); ok {
		code, kind, err := c.exprAST(openExpr)
		if err == nil && isDeferredResource(kind) {
			if err := checkType(lineNo, name, annotation, kind); err != nil {
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
	if existingBefore != "" && c.releasableKind(existingBefore) && existingBefore != "string" && !assignmentExprReferencesName(valueExpr, name) {
		c.emitReleaseValue(cName, existingBefore)
		preReleased = true
	}

	checkpoint := c.needsAssignmentCheckpointAST(name, valueExpr)
	checkpointName := ""
	if checkpoint {
		checkpointName = fmt.Sprintf("__pyrite_checkpoint_%d", c.nextTempID)
		c.nextTempID++
		c.body.WriteString(fmt.Sprintf("    PyriteAllocation *%s = pyrite_checkpoint();\n", checkpointName))
	}
	code, kind, err := c.exprAST(valueExpr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, name, annotation, kind); err != nil {
		return err
	}
	existing := c.types[name]
	if existing != "" && !typesCompatible(existing, kind) {
		return fmt.Errorf("line %d: cannot assign %s to %s previously inferred as %s", lineNo, kind, name, existing)
	}
	storeKind := kind
	if annotation != "" {
		storeKind = annotation
	} else if existing != "" {
		storeKind = existing
	}
	if storeKind == "none" {
		return fmt.Errorf("line %d: None assignments need a type annotation", lineNo)
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
	if existing != "" && c.releasableKind(storeKind) && storeKind != "string" && !preReleased && !assignmentExprReferencesName(valueExpr, name) {
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
		if storeKind != "list_int" && isListKind(storeKind) {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since_any_list(%s, &%s);\n", checkpointName, cName))
		} else if storeKind == "bytes" {
			c.body.WriteString(fmt.Sprintf("    pyrite_release_since(%s, %s.items);\n", checkpointName, cName))
		} else if isDictKind(storeKind) || storeKind == "set" {
			c.body.WriteString(fmt.Sprintf("    (void)%s;\n", checkpointName))
			c.body.WriteString("    /* dict/set values keep nested allocations for now. */\n")
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

func deferredResourceExpr(expr pyriteExpr) (pyriteExpr, bool) {
	call, ok := expr.(*pyriteCallExpr)
	if !ok || len(call.Args) != 0 {
		return nil, false
	}
	member, ok := call.Callee.(*pyriteMemberExpr)
	if !ok || member.Field != "defer" {
		return nil, false
	}
	return member.Base, true
}

func assignmentExprReferencesName(expr pyriteExpr, name string) bool {
	if name == "" || expr == nil {
		return false
	}
	switch node := expr.(type) {
	case *pyriteNameExpr:
		return node.Name == name
	case *pyriteUnaryExpr:
		return assignmentExprReferencesName(node.Right, name)
	case *pyriteBinaryExpr:
		return assignmentExprReferencesName(node.Left, name) || assignmentExprReferencesName(node.Right, name)
	case *pyriteIndexExpr:
		return assignmentExprReferencesName(node.Base, name) || assignmentExprReferencesName(node.Index, name)
	case *pyriteMemberExpr:
		return assignmentExprReferencesName(node.Base, name)
	case *pyriteCallExpr:
		if assignmentExprReferencesName(node.Callee, name) {
			return true
		}
		for _, arg := range node.Args {
			if assignmentExprReferencesName(arg, name) {
				return true
			}
		}
	case *pyriteListExpr:
		for _, item := range node.Items {
			if assignmentExprReferencesName(item, name) {
				return true
			}
		}
	case *pyriteObjectExpr:
		for _, entry := range node.Entries {
			if assignmentExprReferencesName(entry.Key, name) || assignmentExprReferencesName(entry.Value, name) {
				return true
			}
		}
	}
	return false
}

func (c *Compiler) needsAssignmentCheckpointAST(name string, expr pyriteExpr) bool {
	kind := c.types[name]
	if kind == "string" || kind == "bytes" || isListKind(kind) || isDictKind(kind) || kind == "set" || kind == "string_builder" || kind == "bytes_builder" || kind == "any" {
		return true
	}
	return assignmentExprMayAllocate(expr)
}

func assignmentExprMayAllocate(expr pyriteExpr) bool {
	switch node := expr.(type) {
	case *pyriteLiteralExpr:
		return node.Kind == "string"
	case *pyriteListExpr, *pyriteObjectExpr, *pyriteCallExpr, *pyriteMemberExpr, *pyriteIndexExpr:
		return true
	case *pyriteUnaryExpr:
		return assignmentExprMayAllocate(node.Right)
	case *pyriteBinaryExpr:
		return assignmentExprMayAllocate(node.Left) || assignmentExprMayAllocate(node.Right)
	}
	return false
}

func (c *Compiler) releasableKind(kind string) bool {
	return kind == "string" || kind == "bytes" || isListKind(kind) || isDictKind(kind) || kind == "set" || kind == "string_builder" || kind == "bytes_builder" || kind == "any"
}

func (c *Compiler) blockScopedReleasableKind(kind string) bool {
	return kind == "string" || kind == "bytes" || isListKind(kind) || isDictKind(kind) || kind == "set" || kind == "string_builder" || kind == "bytes_builder"
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
	case "dict":
		cleanup = fmt.Sprintf("    pyrite_release_dict(&%s);\n", name)
	case "set":
		cleanup = fmt.Sprintf("    pyrite_release_set(&%s);\n", name)
	case "string_builder", "bytes_builder":
		cleanup = fmt.Sprintf("    pyrite_release(%s.items);\n", name)
	}
	if cleanup == "" && strings.HasPrefix(kind, "list:") {
		cleanup = fmt.Sprintf("    pyrite_release_any_list(&%s);\n", name)
	}
	if cleanup == "" && strings.HasPrefix(kind, "dict:") {
		cleanup = fmt.Sprintf("    pyrite_release_dict(&%s);\n", name)
	}
	if cleanup == "" {
		return
	}
	top := &c.blockStack[len(c.blockStack)-1]
	top.cleanups = append(top.cleanups, cleanup)
}

func (c *Compiler) emitReleaseValue(name, kind string) {
	if strings.HasPrefix(kind, "list:") {
		c.body.WriteString(fmt.Sprintf("    pyrite_release_any_list(&%s);\n", name))
		return
	}
	if strings.HasPrefix(kind, "dict:") {
		c.body.WriteString(fmt.Sprintf("    pyrite_release_dict(&%s);\n", name))
		return
	}
	switch kind {
	case "string":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s);\n", name))
	case "bytes":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s.items);\n", name))
	case "list_int":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s.items);\n", name))
	case "list_any":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_any_list(&%s);\n", name))
	case "dict":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_dict(&%s);\n", name))
	case "set":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_set(&%s);\n", name))
	case "string_builder", "bytes_builder":
		c.body.WriteString(fmt.Sprintf("    pyrite_release(%s.items);\n", name))
	case "any":
		c.body.WriteString(fmt.Sprintf("    pyrite_release_any(&%s);\n", name))
	}
}

func (c *Compiler) keepPointer(name, kind string) string {
	if strings.HasPrefix(kind, "list:") {
		return fmt.Sprintf("%s.items", name)
	}
	if strings.HasPrefix(kind, "dict:") {
		return fmt.Sprintf("%s.entries", name)
	}
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
	case "dict":
		return fmt.Sprintf("%s.entries", name)
	case "set":
		return fmt.Sprintf("%s.items", name)
	case "string_builder", "bytes_builder":
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
	listKind := c.types[listName]
	return c.emitListLoopCode(lineNo, indent, listName, listKind, itemName)
}

func (c *Compiler) emitListLoopCode(lineNo, indent int, listCode, listKind, itemName string) error {
	if !isIdentifier(itemName) {
		return fmt.Errorf("line %d: invalid foreach item name %q", lineNo, itemName)
	}
	if !isListKind(listKind) {
		return fmt.Errorf("line %d: foreach currently supports lists", lineNo)
	}
	itemKind := listElementKind(listKind)
	if listKind != "list_int" {
		c.types[itemName] = itemKind
		c.body.WriteString(fmt.Sprintf("    for (size_t __i_%s = 0; __i_%s < %s.len; __i_%s++) {\n", itemName, itemName, listCode, itemName))
		c.body.WriteString(fmt.Sprintf("        %s %s = %s;\n", c.cType(itemKind), itemName, anyAccess(fmt.Sprintf("%s.items[__i_%s]", listCode, itemName), itemKind)))
		c.blockStack = append(c.blockStack, block{kind: "for", indent: indent})
		return nil
	}
	c.types[itemName] = "int"
	c.body.WriteString(fmt.Sprintf("    for (size_t __i_%s = 0; __i_%s < %s.len; __i_%s++) {\n", itemName, itemName, listCode, itemName))
	c.body.WriteString(fmt.Sprintf("        long %s = %s.items[__i_%s];\n", itemName, listCode, itemName))
	c.blockStack = append(c.blockStack, block{kind: "for", indent: indent})
	return nil
}

func (c *Compiler) emitForHeaderAST(lineNo, indent int, itemName string, iterable pyriteExpr) error {
	if !isIdentifier(itemName) {
		return fmt.Errorf("line %d: invalid foreach item name %q", lineNo, itemName)
	}
	if iterable == nil {
		return fmt.Errorf("line %d: missing for iterable", lineNo)
	}
	if name, ok := iterable.(*pyriteNameExpr); ok {
		return c.emitListLoop(lineNo, indent, name.Name, itemName)
	}
	code, kind, err := c.exprAST(iterable)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if !isListKind(kind) {
		return fmt.Errorf("line %d: foreach expects a list, got %s", lineNo, kind)
	}
	c.nextTempID++
	tempName := fmt.Sprintf("__pyrite_foreach_%d", c.nextTempID)
	c.types[tempName] = kind
	c.body.WriteString(fmt.Sprintf("    %s %s = %s;\n", c.cType(kind), tempName, code))
	if err := c.emitListLoopCode(lineNo, indent, tempName, kind, itemName); err != nil {
		return err
	}
	if kind != "list_int" && len(c.blockStack) > 0 {
		top := &c.blockStack[len(c.blockStack)-1]
		top.postCleanups = append(top.postCleanups, fmt.Sprintf("    pyrite_release_any_list(&%s);\n", tempName))
	}
	return nil
}

func (c *Compiler) emitSwitchAST(lineNo, indent int, expr pyriteExpr) error {
	if expr == nil {
		return fmt.Errorf("line %d: missing switch expression", lineNo)
	}
	code, kind, err := c.exprAST(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	return c.emitSwitchCode(lineNo, indent, code, kind)
}

func (c *Compiler) emitSwitchCode(lineNo, indent int, code, kind string) error {
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

func (c *Compiler) emitCaseAST(lineNo, indent int, expr pyriteExpr) error {
	if expr == nil {
		return fmt.Errorf("line %d: missing case expression", lineNo)
	}
	code, kind, err := c.exprAST(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	return c.emitCaseCode(lineNo, indent, code, kind)
}

func (c *Compiler) emitCaseCode(lineNo, indent int, code, kind string) error {
	sw, idx, err := c.activeSwitch()
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if indent != sw.indent+4 {
		return fmt.Errorf("line %d: case must be indented one level under switch", lineNo)
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

func (c *Compiler) emitPrintExpr(lineNo int, expr pyriteExpr) error {
	code, kind, err := c.exprAST(expr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	c.emitPrintValue(code, kind)
	return nil
}

func (c *Compiler) emitPrintValue(code, kind string) {
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
	case "dict":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_dict_string(&%s));\n", code))
	case "set":
		c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_set_string(&%s));\n", code))
	default:
		if strings.HasPrefix(kind, "list:") {
			c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_list_any_string(&%s));\n", code))
		} else if strings.HasPrefix(kind, "dict:") {
			c.body.WriteString(fmt.Sprintf("    pyrite_print_str(pyrite_dict_string(&%s));\n", code))
		} else if isClassKind(kind) {
			c.body.WriteString(fmt.Sprintf("    pyrite_print_str(%s && %s->class_name ? %s->class_name : \"<object>\");\n", code, code, code))
		} else {
			c.body.WriteString(fmt.Sprintf("    pyrite_print_str(%s);\n", code))
		}
	}
	c.body.WriteString("    pyrite_temp_reset();\n")
}

func (c *Compiler) emitRoutineAST(lineNo int, args []pyriteExpr) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("line %d: routine expects a call and optional mux", lineNo)
	}
	muxCode := "NULL"
	if len(args) == 2 {
		code, kind, err := c.exprAST(args[1])
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if kind != "mux" {
			return fmt.Errorf("line %d: routine mux argument must be mux, got %s", lineNo, kind)
		}
		muxCode = code
	}
	call, ok := args[0].(*pyriteCallExpr)
	if !ok {
		return fmt.Errorf("line %d: routine expects a function call", lineNo)
	}
	if callee, ok := call.Callee.(*pyriteNameExpr); ok && callee.Name == "print" {
		if len(call.Args) != 1 {
			return fmt.Errorf("line %d: print expects 1 argument(s)", lineNo)
		}
		code, kind, err := c.exprAST(call.Args[0])
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
	name, ok := callNameExpr(call.Callee)
	if !ok || c.functions[name] == nil {
		return fmt.Errorf("line %d: routine currently supports print(...) and helper function calls", lineNo)
	}
	return c.emitRoutineFunctionCallAST(lineNo, name, call.Args, muxCode)
}

func (c *Compiler) emitRoutineFunctionCallAST(lineNo int, name string, args []pyriteExpr, muxCode string) error {
	fn := c.functions[name]
	if fn == nil {
		return fmt.Errorf("line %d: unknown function %s", lineNo, name)
	}
	if len(args) != len(fn.params) {
		return fmt.Errorf("line %d: %s expects %d argument(s), got %d", lineNo, name, len(fn.params), len(args))
	}

	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	for i, arg := range compiled {
		param := fn.params[i]
		want := fn.paramTypes[param]
		if want == "" {
			fn.paramTypes[param] = arg.kind
			compiled[i].kind = arg.kind
			continue
		}
		if !typesCompatible(want, arg.kind) {
			return fmt.Errorf("line %d: %s parameter %s is %s but got %s", lineNo, name, param, want, arg.kind)
		}
		if want == "any" && arg.kind != "any" {
			code, err := anyValue(arg.code, arg.kind)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			compiled[i].code = code
		} else if want == "list_any" && arg.kind == "list_int" {
			compiled[i].code = fmt.Sprintf("pyrite_list_int_to_any(%s)", arg.code)
		}
		compiled[i].kind = want
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
	for i, arg := range compiled {
		c.funcs.WriteString(fmt.Sprintf("    %s arg%d;\n", c.cType(arg.kind), i))
	}
	c.funcs.WriteString(fmt.Sprintf("} %s;\n", structName))
	c.funcs.WriteString(fmt.Sprintf("static void *%s(void *arg) {\n", runName))
	c.funcs.WriteString(fmt.Sprintf("    %s *task = arg;\n", structName))
	c.funcs.WriteString("    pyrite_mux_lock(task->mux);\n")
	c.funcs.WriteString(fmt.Sprintf("    %s(", c.functionCName(name)))
	for i := range compiled {
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
	for i, arg := range compiled {
		c.body.WriteString(fmt.Sprintf(", .arg%d = %s", i, arg.code))
	}
	c.body.WriteString("};\n")
	c.body.WriteString(fmt.Sprintf("        pyrite_start_task(%s, __routine_task_%d);\n", runName, id))
	c.body.WriteString("    }\n")
	return nil
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

func (c *Compiler) activeTry() (int, bool) {
	for i := len(c.blockStack) - 1; i >= 0; i-- {
		if c.blockStack[i].kind == "try" {
			return c.blockStack[i].tryID, true
		}
	}
	return 0, false
}

func (c *Compiler) emitGlobalAST(lineNo int, decl *pyriteBindingDecl) error {
	annotated := ""
	if decl.Type != "" {
		kind, err := normalizeType(decl.Type)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		annotated = kind
	}
	code, kind, err := c.exprAST(decl.ValueExpr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, decl.Name, annotated, kind); err != nil {
		return err
	}
	storeKind := kind
	if annotated != "" {
		storeKind = annotated
	}
	if storeKind == "none" {
		return fmt.Errorf("line %d: None globals need a type annotation", lineNo)
	}
	c.types[decl.Name] = storeKind
	if decl.Const {
		c.consts[decl.Name] = true
	}
	qualifier := ""
	if decl.Const {
		qualifier = "const "
	}
	switch storeKind {
	case "int":
		c.globals.WriteString(fmt.Sprintf("static %slong %s = %s;\n", qualifier, decl.Name, code))
	case "float":
		c.globals.WriteString(fmt.Sprintf("static %sdouble %s = %s;\n", qualifier, decl.Name, code))
	case "bool":
		c.globals.WriteString(fmt.Sprintf("static %sint %s = %s;\n", qualifier, decl.Name, code))
	case "string":
		c.globals.WriteString(fmt.Sprintf("static %schar *%s = %s;\n", qualifier, decl.Name, code))
	default:
		return fmt.Errorf("line %d: global %s cannot use %s yet", lineNo, decl.Name, storeKind)
	}
	return nil
}

func (c *Compiler) emitModuleGlobalAST(lineNo int, moduleName string, decl *pyriteBindingDecl) error {
	if decl.Name == "" || strings.Contains(decl.Name, ".") {
		return fmt.Errorf("line %d: invalid module global name", lineNo)
	}
	sourceName := moduleName + "." + decl.Name
	annotated := ""
	if decl.Type != "" {
		kind, err := normalizeType(decl.Type)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		annotated = kind
	}
	code, kind, err := c.exprAST(decl.ValueExpr)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}
	if err := checkType(lineNo, sourceName, annotated, kind); err != nil {
		return err
	}
	storeKind := kind
	if annotated != "" {
		storeKind = annotated
	}
	if storeKind == "none" {
		return fmt.Errorf("line %d: None module globals need a type annotation", lineNo)
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

func (c *Compiler) emitMemberAssignAST(lineNo int, name string, value pyriteExpr) error {
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
		code, kind, err := c.exprAST(value)
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
	code, kind, err := c.exprAST(value)
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
	if c.localDeclared != nil && c.localDeclared[name] {
		return ""
	}
	if _, exists := c.types[name]; exists {
		return ""
	}
	if strings.HasPrefix(kind, "list:") {
		return "PyriteAnyList "
	}
	if strings.HasPrefix(kind, "dict:") {
		return "PyriteDict "
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
	case "dict":
		return "PyriteDict "
	case "set":
		return "PyriteSet "
	case "string_builder":
		return "PyriteStringBuilder "
	case "bytes_builder":
		return "PyriteBytesBuilder "
	case "object":
		return "PyriteObject "
	default:
		if isClassKind(kind) {
			return "PyriteClassObject *"
		}
		return "long "
	}
}

func (c *Compiler) predeclareFunctionLocals(fn *functionDef) error {
	if len(fn.astBody) == 0 {
		return nil
	}
	declared := map[string]string{}
	for _, stmt := range fn.astBody {
		if err := c.predeclareStmtLocals(stmt, declared); err != nil {
			return err
		}
	}
	if len(declared) > 0 {
		c.body.WriteString("\n")
	}
	return nil
}

func (c *Compiler) predeclareStmtLocals(stmt pyriteStmt, declared map[string]string) error {
	switch node := stmt.(type) {
	case *pyriteVarStmt:
		if err := c.predeclareLocal(node.Line, node.Name, node.Type, node.Value, declared); err != nil {
			return err
		}
	case *pyriteAssignStmt:
		target, ok := assignmentTargetName(node.TargetExpr)
		if !ok {
			return nil
		}
		if err := c.predeclareLocal(node.Line, target, "", node.Value, declared); err != nil {
			return err
		}
	case *pyriteIfStmt:
		if node.Inline != nil {
			if err := c.predeclareStmtLocals(node.Inline, declared); err != nil {
				return err
			}
		}
	case *pyriteWhileStmt:
		if node.Inline != nil {
			if err := c.predeclareStmtLocals(node.Inline, declared); err != nil {
				return err
			}
		}
	case *pyriteCaseStmt:
		if node.Inline != nil {
			if err := c.predeclareStmtLocals(node.Inline, declared); err != nil {
				return err
			}
		}
	}
	for _, child := range stmt.stmtBase().Children {
		if err := c.predeclareStmtLocals(child, declared); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) predeclareLocal(lineNo int, name, annotated string, value pyriteExpr, declared map[string]string) error {
	if name == "" || strings.Contains(name, ".") {
		return nil
	}
	if declared[name] != "" {
		return nil
	}
	kind := annotated
	if kind != "" {
		normalized, err := normalizeType(kind)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		kind = normalized
	} else {
		if value == nil {
			return nil
		}
		_, inferred, err := c.exprAST(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		kind = inferred
	}
	decl := c.decl(name, kind)
	if decl == "" {
		return nil
	}
	c.body.WriteString(fmt.Sprintf("    %s%s%s;\n", decl, name, localZeroInitializer(kind)))
	c.localDeclared[name] = true
	c.types[name] = kind
	declared[name] = kind
	return nil
}

func localZeroInitializer(kind string) string {
	if strings.HasPrefix(kind, "list:") || strings.HasPrefix(kind, "dict:") {
		return " = {0}"
	}
	switch kind {
	case "string", "file", "mux", "socket", "listener":
		return " = NULL"
	case "bytes", "list_int", "list_any", "dict", "set", "string_builder", "bytes_builder", "object", "any":
		return " = {0}"
	default:
		if isClassKind(kind) {
			return " = NULL"
		}
		return " = 0"
	}
}

func (c *Compiler) cType(kind string) string {
	if strings.HasPrefix(kind, "list:") {
		return "PyriteAnyList"
	}
	if strings.HasPrefix(kind, "dict:") {
		return "PyriteDict"
	}
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
	case "dict":
		return "PyriteDict"
	case "set":
		return "PyriteSet"
	case "string_builder":
		return "PyriteStringBuilder"
	case "bytes_builder":
		return "PyriteBytesBuilder"
	case "object":
		return "PyriteObject"
	default:
		if isClassKind(kind) {
			return "PyriteClassObject *"
		}
		return "long"
	}
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
