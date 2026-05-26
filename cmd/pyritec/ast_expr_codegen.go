package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *Compiler) exprAST(expr pyriteExpr) (string, string, error) {
	switch node := expr.(type) {
	case *pyriteLiteralExpr:
		return c.literalExprAST(node)
	case *pyriteNameExpr:
		return c.nameExprAST(node)
	case *pyriteUnaryExpr:
		code, kind, err := c.exprAST(node.Right)
		if err != nil {
			return "", "", err
		}
		if node.Op == "-" && (kind == "int" || kind == "float") {
			return fmt.Sprintf("(-%s)", code), kind, nil
		}
		return "", "", fmt.Errorf("unsupported unary operator %s", node.Op)
	case *pyriteBinaryExpr:
		return c.binaryExprAST(node)
	case *pyriteIndexExpr:
		return c.indexExprAST(node)
	case *pyriteListExpr:
		return c.listLiteralAST(node)
	case *pyriteObjectExpr:
		return c.objectLiteralAST(node)
	case *pyriteCallExpr:
		return c.callExprAST(node)
	case *pyriteMemberExpr:
		return c.memberExprAST(node)
	default:
		return "", "", fmt.Errorf("unsupported expression AST %T", expr)
	}
}

func (c *Compiler) literalExprAST(expr *pyriteLiteralExpr) (string, string, error) {
	switch expr.Kind {
	case "bool":
		if expr.Value == "true" || expr.Value == "True" {
			return "1", "bool", nil
		}
		return "0", "bool", nil
	case "string":
		if strings.HasPrefix(expr.Value, "f\"") {
			return c.fstring(expr.Value)
		}
		if strings.HasPrefix(expr.Value, "b\"") {
			return bytesLiteral(expr.Value)
		}
		return expr.Value, "string", nil
	case "number":
		if n, err := strconv.ParseInt(expr.Value, 10, 64); err == nil {
			return strconv.FormatInt(n, 10), "int", nil
		}
		if n, err := strconv.ParseFloat(expr.Value, 64); err == nil {
			return strconv.FormatFloat(n, 'g', -1, 64), "float", nil
		}
	}
	return "", "", fmt.Errorf("unsupported literal %q", expr.Value)
}

func (c *Compiler) nameExprAST(expr *pyriteNameExpr) (string, string, error) {
	switch expr.Name {
	case "true", "True":
		return "1", "bool", nil
	case "false", "False":
		return "0", "bool", nil
	}
	if kind, ok := c.types[expr.Name]; ok {
		if strings.Contains(expr.Name, ".") {
			return c.variableCName(expr.Name), kind, nil
		}
		return expr.Name, kind, nil
	}
	return "", "", fmt.Errorf("unsupported expression %q", expr.Name)
}

func (c *Compiler) binaryExprAST(expr *pyriteBinaryExpr) (string, string, error) {
	if isComparisonOperator(expr.Op) {
		code, err := c.conditionAST(expr)
		return code, "bool", err
	}
	if expr.Op == "+" {
		leftCode, leftKind, leftErr := c.exprAST(expr.Left)
		rightCode, rightKind, rightErr := c.exprAST(expr.Right)
		if leftErr == nil && rightErr == nil && leftKind == "string" && rightKind == "string" {
			return fmt.Sprintf("pyrite_string_concat(%s, %s)", leftCode, rightCode), "string", nil
		}
		if leftErr == nil && rightErr == nil && leftKind == "bytes" && rightKind == "bytes" {
			return fmt.Sprintf("pyrite_bytes_concat(%s, %s)", leftCode, rightCode), "bytes", nil
		}
	}
	left, leftKind, err := c.numberExprAST(expr.Left)
	if err != nil {
		return "", "", err
	}
	right, rightKind, err := c.numberExprAST(expr.Right)
	if err != nil {
		return "", "", err
	}
	result := numericResult(leftKind, rightKind)
	switch expr.Op {
	case "&":
		if leftKind != "int" || rightKind != "int" {
			return "", "", fmt.Errorf("bitwise & expects int operands")
		}
		return fmt.Sprintf("(%s & %s)", left, right), "int", nil
	case "/":
		return fmt.Sprintf("((double)(%s) / (double)(%s))", left, right), "float", nil
	case "+", "-", "*":
		return fmt.Sprintf("(%s %s %s)", left, expr.Op, right), result, nil
	default:
		return "", "", fmt.Errorf("unsupported binary operator %s", expr.Op)
	}
}

func (c *Compiler) numberExprAST(expr pyriteExpr) (string, string, error) {
	code, kind, err := c.exprAST(expr)
	if err != nil {
		return "", "", err
	}
	if kind != "int" && kind != "float" {
		return "", "", fmt.Errorf("expected number but got %s", kind)
	}
	return code, kind, nil
}

func (c *Compiler) indexExprAST(expr *pyriteIndexExpr) (string, string, error) {
	baseCode, baseKind, err := c.exprAST(expr.Base)
	if err != nil {
		return "", "", err
	}
	idxCode, idxKind, err := c.exprAST(expr.Index)
	if err != nil {
		return "", "", err
	}
	if idxKind != "int" {
		return "", "", fmt.Errorf("index expects int, got %s", idxKind)
	}
	switch baseKind {
	case "list_int":
		return fmt.Sprintf("%s.items[%s]", baseCode, idxCode), "int", nil
	case "bytes":
		return fmt.Sprintf("pyrite_bytes_get(%s, %s)", baseCode, idxCode), "int", nil
	case "string":
		return fmt.Sprintf("pyrite_string_at(%s, %s)", baseCode, idxCode), "string", nil
	}
	if baseKind == "list_any" || strings.HasPrefix(baseKind, "list:") {
		value := fmt.Sprintf("pyrite_list_any_get(%s, %s)", baseCode, idxCode)
		elem := listElementKind(baseKind)
		return anyAccess(value, elem), elem, nil
	}
	return "", "", fmt.Errorf("indexing is not supported for %s", baseKind)
}

func (c *Compiler) callExprAST(expr *pyriteCallExpr) (string, string, error) {
	rendered := renderPyriteExpr(expr)
	if rendered == "mux()" {
		return "pyrite_mux_new()", "mux", nil
	}
	calleeName := renderPyriteExpr(expr.Callee)
	if calleeName != "" {
		if _, exists := c.classes[calleeName]; exists {
			return c.classConstructorCallExprAST(calleeName, expr.Args)
		}
		if c.functions[calleeName] != nil {
			code, kind, err := c.userFunctionCallExprAST(0, calleeName, expr.Args)
			if err != nil {
				return "", "", err
			}
			if kind == "void" {
				return "", "", fmt.Errorf("function %s does not return a value", calleeName)
			}
			return code, kind, nil
		}
	}
	if member, ok := expr.Callee.(*pyriteMemberExpr); ok {
		if code, kind, ok, err := c.methodCallExprAST(member, expr.Args); ok || err != nil {
			return code, kind, err
		}
	}
	if code, kind, ok, err := c.compilerIntrinsicCallExprAST(calleeName, expr.Args); ok || err != nil {
		return code, kind, err
	}
	return "", "", fmt.Errorf("unsupported call %s", rendered)
}

func (c *Compiler) methodCallExprAST(member *pyriteMemberExpr, args []pyriteExpr) (string, string, bool, error) {
	base, baseKind, err := c.exprAST(member.Base)
	if err != nil {
		return "", "", true, err
	}
	method := member.Field
	switch {
	case isClassKind(baseKind):
		return c.classMethodCallExprAST(base, baseKind, method, args)
	case baseKind == "string":
		return c.stringMethodCallExprAST(base, method, args)
	case baseKind == "bytes":
		return c.bytesMethodCallExprAST(base, method, args)
	case isListKind(baseKind):
		return c.listMethodCallExprAST(base, baseKind, method, args)
	case isDictKind(baseKind):
		return c.dictMethodCallExprAST(base, baseKind, method, args)
	case baseKind == "set":
		return c.setMethodCallExprAST(base, method, args)
	case baseKind == "string_builder" || baseKind == "bytes_builder":
		return c.builderMethodCallExprAST(base, baseKind, method, args)
	case baseKind == "int" || baseKind == "float":
		return c.numericMethodCallExprAST(base, baseKind, method, args)
	default:
		return "", "", false, nil
	}
}

func (c *Compiler) stringMethodCallExprAST(base, method string, args []pyriteExpr) (string, string, bool, error) {
	methods := map[string]methodSpec{
		"strip":       {"pyrite_string_strip", 0, "string"},
		"lstrip":      {"pyrite_string_lstrip", 0, "string"},
		"rstrip":      {"pyrite_string_rstrip", 0, "string"},
		"upper":       {"pyrite_string_upper", 0, "string"},
		"lower":       {"pyrite_string_lower", 0, "string"},
		"len":         {"pyrite_string_len", 0, "int"},
		"find":        {"pyrite_string_find", 1, "int"},
		"contains":    {"pyrite_string_contains", 1, "bool"},
		"startswith":  {"pyrite_string_startswith", 1, "bool"},
		"starts_with": {"pyrite_string_startswith", 1, "bool"},
		"endswith":    {"pyrite_string_endswith", 1, "bool"},
		"ends_with":   {"pyrite_string_endswith", 1, "bool"},
		"replace":     {"pyrite_string_replace", 2, "string"},
		"slice":       {"pyrite_string_slice", 2, "string"},
		"get":         {"pyrite_string_at", 1, "string"},
		"at":          {"pyrite_string_at", 1, "string"},
		"byte":        {"pyrite_string_byte_at", 1, "int"},
	}
	spec, exists := methods[method]
	if !exists {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if len(compiled) != spec.args {
		return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
	}
	codes := []string{base}
	for _, arg := range compiled {
		if method == "slice" || method == "get" || method == "at" || method == "byte" {
			if arg.kind != "int" {
				return "", "", true, fmt.Errorf("%s expects int argument, got %s", method, arg.kind)
			}
		} else if arg.kind != "string" {
			return "", "", true, fmt.Errorf("%s expects string argument, got %s", method, arg.kind)
		}
		codes = append(codes, arg.code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
}

func (c *Compiler) bytesMethodCallExprAST(base, method string, args []pyriteExpr) (string, string, bool, error) {
	methods := map[string]methodSpec{
		"len":       {"pyrite_bytes_len", 0, "int"},
		"get":       {"pyrite_bytes_get", 1, "int"},
		"at":        {"pyrite_bytes_get", 1, "int"},
		"slice":     {"pyrite_bytes_slice", 2, "bytes"},
		"push":      {"pyrite_bytes_push", 1, "bytes"},
		"to_string": {"pyrite_bytes_to_string", 0, "string"},
	}
	spec, exists := methods[method]
	if !exists {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if len(compiled) != spec.args {
		return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
	}
	codes := []string{base}
	for _, arg := range compiled {
		if arg.kind != "int" {
			return "", "", true, fmt.Errorf("%s expects int argument, got %s", method, arg.kind)
		}
		codes = append(codes, arg.code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
}

func (c *Compiler) listMethodCallExprAST(base, baseKind, method string, args []pyriteExpr) (string, string, bool, error) {
	if method != "len" && method != "get" && method != "at" && method != "set" && method != "push" && method != "pop" && method != "peek" {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	suffix := "int"
	itemKind := "int"
	if baseKind == "list_any" {
		suffix = "any"
		itemKind = "any"
	} else if strings.HasPrefix(baseKind, "list:") {
		suffix = "any"
		itemKind = listElementKind(baseKind)
	}
	switch method {
	case "len":
		if len(compiled) != 0 {
			return "", "", true, fmt.Errorf("len expects 0 argument(s)")
		}
		return fmt.Sprintf("pyrite_list_%s_len(%s)", suffix, base), "int", true, nil
	case "get", "at":
		if len(compiled) != 1 {
			return "", "", true, fmt.Errorf("%s expects 1 argument(s)", method)
		}
		if compiled[0].kind != "int" {
			return "", "", true, fmt.Errorf("%s expects int index, got %s", method, compiled[0].kind)
		}
		value := fmt.Sprintf("pyrite_list_%s_get(%s, %s)", suffix, base, compiled[0].code)
		if suffix == "any" {
			value = anyAccess(value, itemKind)
		}
		return value, itemKind, true, nil
	case "peek":
		if len(compiled) != 0 {
			return "", "", true, fmt.Errorf("peek expects 0 argument(s)")
		}
		value := fmt.Sprintf("pyrite_list_%s_peek(%s)", suffix, base)
		if suffix == "any" {
			value = anyAccess(value, itemKind)
		}
		return value, itemKind, true, nil
	case "pop":
		if len(compiled) != 0 {
			return "", "", true, fmt.Errorf("pop expects 0 argument(s)")
		}
		return fmt.Sprintf("pyrite_list_%s_pop(%s)", suffix, base), baseKind, true, nil
	case "push":
		if len(compiled) != 1 {
			return "", "", true, fmt.Errorf("push expects 1 argument(s)")
		}
		value := compiled[0]
		if baseKind == "list_int" {
			if value.kind != "int" {
				return "", "", true, fmt.Errorf("push expects int value, got %s", value.kind)
			}
		} else if itemKind != "any" && !typesCompatible(itemKind, value.kind) {
			return "", "", true, fmt.Errorf("push expects %s value, got %s", itemKind, value.kind)
		}
		code := value.code
		if baseKind != "list_int" && value.kind != "any" {
			code, err = anyValue(code, value.kind)
			if err != nil {
				return "", "", true, err
			}
		}
		return fmt.Sprintf("pyrite_list_%s_push(%s, %s)", suffix, base, code), baseKind, true, nil
	case "set":
		if len(compiled) != 2 {
			return "", "", true, fmt.Errorf("set expects 2 argument(s)")
		}
		if compiled[0].kind != "int" {
			return "", "", true, fmt.Errorf("set expects int index, got %s", compiled[0].kind)
		}
		value := compiled[1]
		if baseKind == "list_int" {
			if value.kind != "int" {
				return "", "", true, fmt.Errorf("set expects int value, got %s", value.kind)
			}
		} else if itemKind != "any" && !typesCompatible(itemKind, value.kind) {
			return "", "", true, fmt.Errorf("set expects %s value, got %s", itemKind, value.kind)
		}
		code := value.code
		if baseKind != "list_int" && value.kind != "any" {
			code, err = anyValue(code, value.kind)
			if err != nil {
				return "", "", true, err
			}
		}
		return fmt.Sprintf("pyrite_list_%s_set(%s, %s, %s)", suffix, base, compiled[0].code, code), baseKind, true, nil
	}
	return "", "", false, nil
}

func (c *Compiler) dictMethodCallExprAST(base, baseKind, method string, args []pyriteExpr) (string, string, bool, error) {
	if method != "len" && method != "has" && method != "get" && method != "get_string" && method != "get_int" && method != "set" && method != "remove" {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if method == "len" {
		if len(compiled) != 0 {
			return "", "", true, fmt.Errorf("len expects 0 argument(s)")
		}
		return fmt.Sprintf("pyrite_dict_len(%s)", base), "int", true, nil
	}
	if method == "set" {
		if len(compiled) != 2 {
			return "", "", true, fmt.Errorf("set expects 2 argument(s)")
		}
		key := compiled[0]
		if key.kind != "string" {
			return "", "", true, fmt.Errorf("dict key must be string, got %s", key.kind)
		}
		value := compiled[1]
		want := dictValueKind(baseKind)
		if want != "any" && !typesCompatible(want, value.kind) {
			return "", "", true, fmt.Errorf("dict value must be %s, got %s", want, value.kind)
		}
		code := value.code
		if value.kind != "any" {
			code, err = anyValue(code, value.kind)
			if err != nil {
				return "", "", true, err
			}
		}
		return fmt.Sprintf("pyrite_dict_set(%s, %s, %s)", base, key.code, code), baseKind, true, nil
	}
	if len(compiled) != 1 {
		return "", "", true, fmt.Errorf("%s expects 1 argument(s)", method)
	}
	key := compiled[0]
	if key.kind != "string" {
		return "", "", true, fmt.Errorf("dict key must be string, got %s", key.kind)
	}
	switch method {
	case "has":
		return fmt.Sprintf("pyrite_dict_has(%s, %s)", base, key.code), "bool", true, nil
	case "get":
		kind := dictValueKind(baseKind)
		return anyAccess(fmt.Sprintf("pyrite_dict_get(%s, %s)", base, key.code), kind), kind, true, nil
	case "get_string":
		return fmt.Sprintf("pyrite_dict_get_string(%s, %s)", base, key.code), "string", true, nil
	case "get_int":
		return fmt.Sprintf("pyrite_dict_get_int(%s, %s)", base, key.code), "int", true, nil
	case "remove":
		return fmt.Sprintf("pyrite_dict_remove(%s, %s)", base, key.code), baseKind, true, nil
	}
	return "", "", false, nil
}

func (c *Compiler) setMethodCallExprAST(base, method string, args []pyriteExpr) (string, string, bool, error) {
	if method != "len" && method != "has" && method != "add" && method != "remove" {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if method == "len" {
		if len(compiled) != 0 {
			return "", "", true, fmt.Errorf("len expects 0 argument(s)")
		}
		return fmt.Sprintf("pyrite_set_len(%s)", base), "int", true, nil
	}
	if len(compiled) != 1 {
		return "", "", true, fmt.Errorf("%s expects 1 argument(s)", method)
	}
	value := compiled[0]
	if value.kind != "string" {
		return "", "", true, fmt.Errorf("set value must be string, got %s", value.kind)
	}
	switch method {
	case "has":
		return fmt.Sprintf("pyrite_set_has(%s, %s)", base, value.code), "bool", true, nil
	case "add":
		return fmt.Sprintf("pyrite_set_add(%s, %s)", base, value.code), "set", true, nil
	case "remove":
		return fmt.Sprintf("pyrite_set_remove(%s, %s)", base, value.code), "set", true, nil
	}
	return "", "", false, nil
}

func (c *Compiler) builderMethodCallExprAST(base, baseKind, method string, args []pyriteExpr) (string, string, bool, error) {
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if baseKind == "string_builder" {
		switch method {
		case "write":
			if len(compiled) != 1 {
				return "", "", true, fmt.Errorf("write expects 1 argument(s)")
			}
			if compiled[0].kind != "string" {
				return "", "", true, fmt.Errorf("write expects string, got %s", compiled[0].kind)
			}
			return fmt.Sprintf("pyrite_string_builder_write(%s, %s)", base, compiled[0].code), "string_builder", true, nil
		case "string", "to_string":
			if len(compiled) != 0 {
				return "", "", true, fmt.Errorf("%s expects 0 argument(s)", method)
			}
			return fmt.Sprintf("pyrite_string_builder_string(%s)", base), "string", true, nil
		case "len":
			if len(compiled) != 0 {
				return "", "", true, fmt.Errorf("len expects 0 argument(s)")
			}
			return fmt.Sprintf("pyrite_string_builder_len(%s)", base), "int", true, nil
		}
	}
	if baseKind == "bytes_builder" {
		switch method {
		case "write":
			if len(compiled) != 1 {
				return "", "", true, fmt.Errorf("write expects 1 argument(s)")
			}
			if compiled[0].kind != "bytes" {
				return "", "", true, fmt.Errorf("write expects bytes, got %s", compiled[0].kind)
			}
			return fmt.Sprintf("pyrite_bytes_builder_write(%s, %s)", base, compiled[0].code), "bytes_builder", true, nil
		case "push":
			if len(compiled) != 1 {
				return "", "", true, fmt.Errorf("push expects 1 argument(s)")
			}
			if compiled[0].kind != "int" {
				return "", "", true, fmt.Errorf("push expects int, got %s", compiled[0].kind)
			}
			return fmt.Sprintf("pyrite_bytes_builder_push(%s, %s)", base, compiled[0].code), "bytes_builder", true, nil
		case "bytes", "to_bytes":
			if len(compiled) != 0 {
				return "", "", true, fmt.Errorf("%s expects 0 argument(s)", method)
			}
			return fmt.Sprintf("pyrite_bytes_builder_bytes(%s)", base), "bytes", true, nil
		case "len":
			if len(compiled) != 0 {
				return "", "", true, fmt.Errorf("len expects 0 argument(s)")
			}
			return fmt.Sprintf("pyrite_bytes_builder_len(%s)", base), "int", true, nil
		}
	}
	return "", "", false, nil
}

func (c *Compiler) numericMethodCallExprAST(base, baseKind, method string, args []pyriteExpr) (string, string, bool, error) {
	intMethods := map[string]methodSpec{
		"abs": {"pyrite_int_abs", 0, "int"}, "min": {"pyrite_int_min", 1, "int"}, "max": {"pyrite_int_max", 1, "int"},
		"clamp": {"pyrite_int_clamp", 2, "int"}, "to_float": {"pyrite_int_to_float", 0, "float"},
		"to_string": {"pyrite_int_to_string", 0, "string"}, "is_even": {"pyrite_int_is_even", 0, "bool"}, "is_odd": {"pyrite_int_is_odd", 0, "bool"},
	}
	floatMethods := map[string]methodSpec{
		"abs": {"pyrite_float_abs", 0, "float"}, "min": {"pyrite_float_min", 1, "float"}, "max": {"pyrite_float_max", 1, "float"},
		"clamp": {"pyrite_float_clamp", 2, "float"}, "round": {"pyrite_float_round", 0, "int"}, "floor": {"pyrite_float_floor", 0, "int"},
		"ceil": {"pyrite_float_ceil", 0, "int"}, "trunc": {"pyrite_float_trunc", 0, "int"}, "to_int": {"pyrite_float_to_int", 0, "int"},
		"to_string": {"pyrite_float_to_string", 0, "string"},
	}
	spec, ok := intMethods[method]
	allowIntArgs := false
	if baseKind == "float" {
		spec, ok = floatMethods[method]
		allowIntArgs = true
	}
	if !ok {
		return "", "", false, nil
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	if len(compiled) != spec.args {
		return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
	}
	codes := []string{base}
	for _, arg := range compiled {
		if allowIntArgs {
			if arg.kind != "int" && arg.kind != "float" {
				return "", "", true, fmt.Errorf("%s expects numeric argument, got %s", method, arg.kind)
			}
		} else if arg.kind != baseKind {
			return "", "", true, fmt.Errorf("%s expects %s argument, got %s", method, baseKind, arg.kind)
		}
		codes = append(codes, arg.code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
}

func (c *Compiler) classMethodCallExprAST(base, baseKind, method string, args []pyriteExpr) (string, string, bool, error) {
	cls := c.classes[classNameFromKind(baseKind)]
	if cls == nil {
		return "", "", true, fmt.Errorf("unknown class %s", classNameFromKind(baseKind))
	}
	fn := cls.methods[method]
	if fn == nil {
		return "", "", true, fmt.Errorf("class %s has no method %s", cls.name, method)
	}
	params := fn.params[1:]
	if len(args) != len(params) {
		return "", "", true, fmt.Errorf("%s.%s expects %d argument(s), got %d", cls.name, method, len(params), len(args))
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	codes := []string{base}
	for i, arg := range compiled {
		want := fn.paramTypes[params[i]]
		if !typesCompatible(want, arg.kind) {
			return "", "", true, fmt.Errorf("%s.%s parameter %s is %s but got %s", cls.name, method, params[i], want, arg.kind)
		}
		codes = append(codes, arg.code)
	}
	if fn.returnType == "" {
		if err := c.inferFunctionReturn(fn); err != nil {
			return "", "", true, err
		}
	}
	return fmt.Sprintf("%s(%s)", c.functionCName(fn.name), strings.Join(codes, ", ")), fn.returnType, true, nil
}

func (c *Compiler) compilerIntrinsicCallExprAST(name string, args []pyriteExpr) (string, string, bool, error) {
	if name == "" {
		return "", "", false, nil
	}
	if name == "net.serve_delimited" {
		code, kind, err := c.netServeDelimitedExprAST(args)
		return code, kind, true, err
	}
	spec, exists := compilerIntrinsics()[name]
	if !exists {
		return "", "", false, nil
	}
	if len(args) != len(spec.params) {
		return "", "", true, fmt.Errorf("%s expects %d argument(s), got %d", name, len(spec.params), len(args))
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", true, err
	}
	codes := make([]string, 0, len(compiled))
	for i, arg := range compiled {
		want := spec.params[i]
		code := arg.code
		if want == "any" && arg.kind != "any" {
			code, err = anyValue(code, arg.kind)
			if err != nil {
				return "", "", true, err
			}
		} else if want == "list_any" && arg.kind == "list_int" {
			code = fmt.Sprintf("pyrite_list_int_to_any(%s)", code)
		} else if !typesCompatible(want, arg.kind) {
			return "", "", true, fmt.Errorf("%s parameter %d is %s but got %s", name, i+1, want, arg.kind)
		}
		codes = append(codes, code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.result, true, nil
}

func (c *Compiler) netServeDelimitedExprAST(args []pyriteExpr) (string, string, error) {
	if len(args) != 6 {
		return "", "", fmt.Errorf("net.serve_delimited expects host, port, delimiter, handler, should_close, and message limit")
	}
	host, hostKind, err := c.exprAST(args[0])
	if err != nil {
		return "", "", err
	}
	if hostKind != "string" {
		return "", "", fmt.Errorf("net.serve_delimited host must be string, got %s", hostKind)
	}
	port, portKind, err := c.exprAST(args[1])
	if err != nil {
		return "", "", err
	}
	if portKind != "int" {
		return "", "", fmt.Errorf("net.serve_delimited port must be int, got %s", portKind)
	}
	delimiter, delimiterKind, err := c.exprAST(args[2])
	if err != nil {
		return "", "", err
	}
	if delimiterKind != "string" {
		return "", "", fmt.Errorf("net.serve_delimited delimiter must be string, got %s", delimiterKind)
	}
	handlerName := renderPyriteExpr(args[3])
	handler := c.functions[handlerName]
	if handler == nil {
		return "", "", fmt.Errorf("net.serve_delimited handler must be a function name, got %q", handlerName)
	}
	if len(handler.params) != 1 {
		return "", "", fmt.Errorf("net.serve_delimited handler %s must take one string frame", handlerName)
	}
	handler.paramTypes[handler.params[0]] = "string"
	if handler.returnType == "" {
		if err := c.inferFunctionReturn(handler); err != nil {
			return "", "", err
		}
	}
	if handler.returnType != "string" {
		return "", "", fmt.Errorf("net.serve_delimited handler %s must return string, got %s", handlerName, handler.returnType)
	}
	closeName := renderPyriteExpr(args[4])
	closeFn := c.functions[closeName]
	if closeFn == nil {
		return "", "", fmt.Errorf("net.serve_delimited should_close must be a function name, got %q", closeName)
	}
	if len(closeFn.params) != 1 {
		return "", "", fmt.Errorf("net.serve_delimited should_close %s must take one string frame", closeName)
	}
	closeFn.paramTypes[closeFn.params[0]] = "string"
	if closeFn.returnType == "" {
		if err := c.inferFunctionReturn(closeFn); err != nil {
			return "", "", err
		}
	}
	if closeFn.returnType != "bool" {
		return "", "", fmt.Errorf("net.serve_delimited should_close %s must return bool, got %s", closeName, closeFn.returnType)
	}
	limit, limitKind, err := c.exprAST(args[5])
	if err != nil {
		return "", "", err
	}
	if limitKind != "int" {
		return "", "", fmt.Errorf("net.serve_delimited message limit must be int, got %s", limitKind)
	}
	return fmt.Sprintf("pyrite_net_serve_delimited(%s, %s, %s, %s, %s, %s)", host, port, delimiter, c.functionCName(handlerName), c.functionCName(closeName), limit), "int", nil
}

func (c *Compiler) listLiteralAST(expr *pyriteListExpr) (string, string, error) {
	if len(expr.Items) == 0 {
		return "pyrite_list_any_new((PyriteAny[]){0}, 0)", "list_any", nil
	}
	args, err := c.compileExprArgsAST(expr.Items)
	if err != nil {
		return "", "", err
	}
	allInt := true
	firstKind := ""
	homogeneous := true
	for _, arg := range args {
		if arg.kind != "int" {
			allInt = false
		}
		if firstKind == "" {
			firstKind = arg.kind
		} else if firstKind != arg.kind {
			homogeneous = false
		}
	}
	if allInt {
		return fmt.Sprintf("pyrite_list_int_new((long[]){%s}, %d)", strings.Join(argCodes(args), ", "), len(args)), "list_int", nil
	}
	anyItems := make([]string, 0, len(args))
	for _, arg := range args {
		any, err := anyValue(arg.code, arg.kind)
		if err != nil {
			return "", "", err
		}
		anyItems = append(anyItems, any)
	}
	kind := "list_any"
	if homogeneous && firstKind != "" && firstKind != "any" {
		kind = "list:" + firstKind
	}
	return fmt.Sprintf("pyrite_list_any_new((PyriteAny[]){%s}, %d)", strings.Join(anyItems, ", "), len(anyItems)), kind, nil
}

func (c *Compiler) objectLiteralAST(expr *pyriteObjectExpr) (string, string, error) {
	for _, entry := range expr.Entries {
		key, ok := entry.Key.(*pyriteLiteralExpr)
		if ok && key.Value == "\"kind\"" {
			return "(PyriteObject){.kind=\"student\", .has_kind=1, .active=1, .has_active=1}", "object", nil
		}
	}
	return "(PyriteObject){0}", "object", nil
}

func (c *Compiler) classConstructorCallExprAST(name string, args []pyriteExpr) (string, string, error) {
	cls := c.classes[name]
	init := cls.methods["__init__"]
	params := []string{}
	if init != nil && len(init.params) > 1 {
		params = init.params[1:]
	}
	if len(args) != len(params) {
		return "", "", fmt.Errorf("%s expects %d argument(s), got %d", name, len(params), len(args))
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", err
	}
	codes := make([]string, 0, len(compiled))
	for i, arg := range compiled {
		want := init.paramTypes[params[i]]
		if !typesCompatible(want, arg.kind) {
			return "", "", fmt.Errorf("%s constructor parameter %s is %s but got %s", name, params[i], want, arg.kind)
		}
		codes = append(codes, arg.code)
	}
	return fmt.Sprintf("%s(%s)", c.constructorCName(name), strings.Join(codes, ", ")), classKind(name), nil
}

func (c *Compiler) userFunctionCallExprAST(lineNo int, name string, args []pyriteExpr) (string, string, error) {
	fn := c.functions[name]
	if fn == nil {
		return "", "", fmt.Errorf("line %d: unknown function %s", lineNo, name)
	}
	if len(args) != len(fn.params) {
		return "", "", fmt.Errorf("line %d: %s expects %d argument(s), got %d", lineNo, name, len(fn.params), len(args))
	}
	compiled, err := c.compileExprArgsAST(args)
	if err != nil {
		return "", "", fmt.Errorf("line %d: %w", lineNo, err)
	}
	codes := make([]string, 0, len(compiled))
	for i, arg := range compiled {
		param := fn.params[i]
		want := fn.paramTypes[param]
		if want == "" {
			fn.paramTypes[param] = arg.kind
		} else if !typesCompatible(want, arg.kind) {
			return "", "", fmt.Errorf("line %d: %s parameter %s is %s but got %s", lineNo, name, param, want, arg.kind)
		}
		code := arg.code
		if want == "any" && arg.kind != "any" {
			anyCode, err := anyValue(code, arg.kind)
			if err != nil {
				return "", "", fmt.Errorf("line %d: %w", lineNo, err)
			}
			code = anyCode
		} else if want == "list_any" && arg.kind == "list_int" {
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

type compiledExprArg struct {
	code string
	kind string
}

func (c *Compiler) compileExprArgsAST(args []pyriteExpr) ([]compiledExprArg, error) {
	compiled := make([]compiledExprArg, 0, len(args))
	for _, arg := range args {
		code, kind, err := c.exprAST(arg)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiledExprArg{code: code, kind: kind})
	}
	return compiled, nil
}

func argCodes(args []compiledExprArg) []string {
	codes := make([]string, 0, len(args))
	for _, arg := range args {
		codes = append(codes, arg.code)
	}
	return codes
}

func (c *Compiler) memberExprAST(expr *pyriteMemberExpr) (string, string, error) {
	rendered := renderPyriteExpr(expr)
	if kind, ok := c.types[rendered]; ok {
		return c.variableCName(rendered), kind, nil
	}
	base, baseKind, err := c.exprAST(expr.Base)
	if err != nil {
		return "", "", err
	}
	if isClassKind(baseKind) {
		cls := c.classes[classNameFromKind(baseKind)]
		if cls == nil {
			return "", "", fmt.Errorf("unknown class %s", classNameFromKind(baseKind))
		}
		kind := cls.fields[expr.Field]
		if kind == "" {
			return "", "", fmt.Errorf("class %s has no field %s", cls.name, expr.Field)
		}
		return c.classFieldGetter(base, expr.Field, kind), kind, nil
	}
	if baseKind != "object" {
		return "", "", fmt.Errorf("unsupported member base %s", renderPyriteExpr(expr.Base))
	}
	baseRaw := renderPyriteExpr(expr.Base)
	if !isIdentifier(baseRaw) {
		return "", "", fmt.Errorf("object member base must be a variable")
	}
	switch expr.Field {
	case "name":
		return fmt.Sprintf("obj_name(&%s)", baseRaw), "string", nil
	case "kind":
		return fmt.Sprintf("obj_kind(&%s)", baseRaw), "string", nil
	case "score":
		return fmt.Sprintf("obj_score(&%s)", baseRaw), "int", nil
	default:
		return "", "", fmt.Errorf("unsupported member %s", expr.Field)
	}
}

func (c *Compiler) conditionAST(expr pyriteExpr) (string, error) {
	if binary, ok := expr.(*pyriteBinaryExpr); ok && isComparisonOperator(binary.Op) {
		left, leftKind, err := c.exprAST(binary.Left)
		if err != nil {
			return "", err
		}
		right, rightKind, err := c.exprAST(binary.Right)
		if err != nil {
			return "", err
		}
		if (binary.Op == "==" || binary.Op == "!=") && leftKind == "string" && rightKind == "string" {
			cmp := fmt.Sprintf("strcmp(%s, %s)", left, right)
			if binary.Op == "==" {
				return fmt.Sprintf("(%s == 0)", cmp), nil
			}
			return fmt.Sprintf("(%s != 0)", cmp), nil
		}
		return fmt.Sprintf("%s %s %s", left, binary.Op, right), nil
	}
	code, kind, err := c.exprAST(expr)
	if err != nil {
		return "", err
	}
	switch kind {
	case "bool", "int", "float":
		return code, nil
	case "string":
		return fmt.Sprintf("(%s && %s[0] != '\\0')", code, code), nil
	default:
		return "", fmt.Errorf("unsupported condition type %s", kind)
	}
}

func isComparisonOperator(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func renderPyriteExpr(expr pyriteExpr) string {
	switch node := expr.(type) {
	case *pyriteNameExpr:
		return node.Name
	case *pyriteLiteralExpr:
		return node.Value
	case *pyriteUnaryExpr:
		return node.Op + renderPyriteExpr(node.Right)
	case *pyriteBinaryExpr:
		return renderPyriteExpr(node.Left) + " " + node.Op + " " + renderPyriteExpr(node.Right)
	case *pyriteIndexExpr:
		return renderPyriteExpr(node.Base) + "[" + renderPyriteExpr(node.Index) + "]"
	case *pyriteMemberExpr:
		return renderPyriteExpr(node.Base) + "." + node.Field
	case *pyriteCallExpr:
		args := make([]string, 0, len(node.Args))
		for _, arg := range node.Args {
			args = append(args, renderPyriteExpr(arg))
		}
		return renderPyriteExpr(node.Callee) + "(" + strings.Join(args, ", ") + ")"
	case *pyriteListExpr:
		items := make([]string, 0, len(node.Items))
		for _, item := range node.Items {
			items = append(items, renderPyriteExpr(item))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case *pyriteObjectExpr:
		entries := make([]string, 0, len(node.Entries))
		for _, entry := range node.Entries {
			entries = append(entries, renderPyriteExpr(entry.Key)+": "+renderPyriteExpr(entry.Value))
		}
		return "{" + strings.Join(entries, ", ") + "}"
	default:
		return ""
	}
}
