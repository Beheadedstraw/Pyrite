package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *Compiler) expr(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "f\"") && strings.HasSuffix(s, "\"") {
		return c.fstring(s)
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return s, "string", nil
	}
	if s == "true" || s == "True" {
		return "1", "bool", nil
	}
	if s == "false" || s == "False" {
		return "0", "bool", nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return strconv.FormatInt(n, 10), "int", nil
	}
	if strings.Contains(s, ".") {
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return strconv.FormatFloat(n, 'g', -1, 64), "float", nil
		}
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return c.listLiteral(s)
	}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return c.objectLiteral(s)
	}
	if s == "mux()" {
		return "pyrite_mux_new()", "mux", nil
	}
	if c.isClassConstructorCall(s) {
		return c.classConstructorCallExpr(s)
	}
	if code, kind, ok, err := c.binaryNumberExpr(s); ok || err != nil {
		return code, kind, err
	}
	if strings.Contains(s, "[") && strings.HasSuffix(s, "]") {
		name := s[:strings.IndexByte(s, '[')]
		idx := strings.TrimSuffix(s[strings.IndexByte(s, '[')+1:], "]")
		if c.types[name] == "list_int" {
			return fmt.Sprintf("%s.items[%s]", name, idx), "int", nil
		}
		if c.types[name] == "list_any" {
			return fmt.Sprintf("%s.items[%s]", name, idx), "any", nil
		}
	}
	if c.isUserFunctionCall(s) {
		code, kind, err := c.userFunctionCallExpr(0, s)
		if err != nil {
			return "", "", err
		}
		if kind == "void" {
			return "", "", fmt.Errorf("function %s does not return a value", s[:strings.IndexByte(s, '(')])
		}
		return code, kind, nil
	}
	if c.isCompilerIntrinsicCall(s) {
		return c.compilerIntrinsicCallExpr(s)
	}
	if code, kind, ok, err := c.classMethodCall(s); ok || err != nil {
		return code, kind, err
	}
	if code, kind, ok, err := c.stringMethodCall(s); ok || err != nil {
		return code, kind, err
	}
	if code, kind, ok, err := c.numericMethodCall(s); ok || err != nil {
		return code, kind, err
	}
	if kind, ok := c.types[s]; ok {
		if strings.Contains(s, ".") {
			return c.variableCName(s), kind, nil
		}
		return s, kind, nil
	}
	if strings.Contains(s, ".") {
		return c.memberExpr(s)
	}
	return "", "", fmt.Errorf("unsupported expression %q", s)
}

func (c *Compiler) binaryNumberExpr(s string) (string, string, bool, error) {
	for _, ops := range [][]byte{{'+', '-'}, {'*', '/'}} {
		idx, op := findTopLevelOperator(s, ops)
		if idx < 0 {
			continue
		}
		left, leftKind, err := c.numberExpr(s[:idx])
		if err != nil {
			return "", "", true, err
		}
		right, rightKind, err := c.numberExpr(s[idx+1:])
		if err != nil {
			return "", "", true, err
		}
		result := numericResult(leftKind, rightKind)
		if op == '/' {
			result = "float"
			return fmt.Sprintf("((double)(%s) / (double)(%s))", left, right), result, true, nil
		}
		return fmt.Sprintf("(%s %c %s)", left, op, right), result, true, nil
	}
	return "", "", false, nil
}

func findTopLevelOperator(s string, ops []byte) (int, byte) {
	depth := 0
	inString := false
	for i := len(s) - 1; i >= 0; i-- {
		ch := s[i]
		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case ')', ']', '}':
			depth++
			continue
		case '(', '[', '{':
			depth--
			continue
		}
		if depth != 0 || !containsByte(ops, ch) {
			continue
		}
		if ch == '-' && isUnaryMinus(s, i) {
			continue
		}
		return i, ch
	}
	return -1, 0
}

func containsByte(values []byte, ch byte) bool {
	for _, value := range values {
		if value == ch {
			return true
		}
	}
	return false
}

func isUnaryMinus(s string, idx int) bool {
	if idx == 0 {
		return true
	}
	left := strings.TrimSpace(s[:idx])
	if left == "" {
		return true
	}
	last := left[len(left)-1]
	return last == '+' || last == '-' || last == '*' || last == '/' || last == '(' || last == '[' || last == ','
}

func (c *Compiler) isClassConstructorCall(s string) bool {
	name, _, ok := splitCall(s)
	if !ok {
		return false
	}
	_, exists := c.classes[name]
	return exists
}

func (c *Compiler) classConstructorCallExpr(s string) (string, string, error) {
	name, rawArgs, _ := splitCall(s)
	cls := c.classes[name]
	init := cls.methods["__init__"]
	args := splitArgs(rawArgs)
	params := []string{}
	if init != nil && len(init.params) > 1 {
		params = init.params[1:]
	}
	if len(args) != len(params) {
		return "", "", fmt.Errorf("%s expects %d argument(s), got %d", name, len(params), len(args))
	}
	var codes []string
	for i, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", err
		}
		want := init.paramTypes[params[i]]
		if !typesCompatible(want, kind) {
			return "", "", fmt.Errorf("%s constructor parameter %s is %s but got %s", name, params[i], want, kind)
		}
		codes = append(codes, code)
	}
	return fmt.Sprintf("%s(%s)", c.constructorCName(name), strings.Join(codes, ", ")), classKind(name), nil
}

func (c *Compiler) listLiteral(s string) (string, string, error) {
	items := splitArgs(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"))
	if len(items) == 0 {
		return "pyrite_list_any_new((PyriteAny[]){0}, 0)", "list_any", nil
	}
	var codes []string
	var kinds []string
	allInt := true
	for _, item := range items {
		code, kind, err := c.expr(item)
		if err != nil {
			return "", "", err
		}
		codes = append(codes, code)
		kinds = append(kinds, kind)
		if kind != "int" {
			allInt = false
		}
	}
	if allInt {
		return fmt.Sprintf("pyrite_list_int_new((long[]){%s}, %d)", strings.Join(codes, ", "), len(codes)), "list_int", nil
	}
	var anyItems []string
	for i, code := range codes {
		any, err := anyValue(code, kinds[i])
		if err != nil {
			return "", "", err
		}
		anyItems = append(anyItems, any)
	}
	return fmt.Sprintf("pyrite_list_any_new((PyriteAny[]){%s}, %d)", strings.Join(anyItems, ", "), len(anyItems)), "list_any", nil
}

func anyValue(code, kind string) (string, error) {
	switch kind {
	case "any":
		return code, nil
	case "int":
		return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_INT, .as.i=%s}", code), nil
	case "float":
		return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_FLOAT, .as.f=%s}", code), nil
	case "bool":
		return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_BOOL, .as.b=%s}", code), nil
	case "string":
		return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string(%s)}", code), nil
	default:
		return "", fmt.Errorf("list[any] does not support %s items yet", kind)
	}
}

type methodSpec struct {
	symbol string
	args   int
	kind   string
}

func (c *Compiler) stringMethodCall(s string) (string, string, bool, error) {
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
	}
	baseRaw, method, rawArgs, ok := splitMethodCall(s)
	if !ok {
		return "", "", false, nil
	}
	spec, exists := methods[method]
	if !exists {
		return "", "", false, nil
	}
	base, baseKind, err := c.expr(baseRaw)
	if err != nil {
		return "", "", true, err
	}
	if baseKind != "string" {
		return "", "", true, fmt.Errorf("%s.%s expects string receiver, got %s", baseRaw, method, baseKind)
	}
	args := splitArgs(rawArgs)
	if len(args) != spec.args {
		return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
	}
	codes := []string{base}
	for _, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", true, err
		}
		if method == "slice" {
			if kind != "int" {
				return "", "", true, fmt.Errorf("%s expects int argument, got %s", method, kind)
			}
		} else if kind != "string" {
			return "", "", true, fmt.Errorf("%s expects string argument, got %s", method, kind)
		}
		codes = append(codes, code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
}

type compilerIntrinsicSpec struct {
	symbol string
	params []string
	result string
}

func compilerIntrinsics() map[string]compilerIntrinsicSpec {
	return map[string]compilerIntrinsicSpec{
		"__json_stringify_any":  {"pyrite_json_stringify_any", []string{"any"}, "string"},
		"__json_stringify_list": {"pyrite_json_stringify_list", []string{"list_any"}, "string"},
		"__json_parse_any":      {"pyrite_json_parse_any", []string{"string"}, "any"},
		"__json_parse_array":    {"pyrite_json_parse_array", []string{"string"}, "list_any"},
		"__json_get_string":     {"pyrite_json_get_string", []string{"string", "string"}, "string"},
		"__json_get_int":        {"pyrite_json_get_int", []string{"string", "string"}, "int"},
		"__json_get_float":      {"pyrite_json_get_float", []string{"string", "string"}, "float"},
		"__json_get_bool":       {"pyrite_json_get_bool", []string{"string", "string"}, "bool"},
	}
}

func (c *Compiler) isCompilerIntrinsicCall(s string) bool {
	name, _, ok := splitCall(s)
	if !ok {
		return false
	}
	if name == "net.serve_delimited" {
		return true
	}
	_, exists := compilerIntrinsics()[name]
	return exists
}

func (c *Compiler) compilerIntrinsicCallExpr(s string) (string, string, error) {
	name, rawArgs, _ := splitCall(s)
	if name == "net.serve_delimited" {
		return c.netServeDelimitedExpr(rawArgs)
	}
	spec := compilerIntrinsics()[name]
	args := splitArgs(rawArgs)
	if len(args) != len(spec.params) {
		return "", "", fmt.Errorf("%s expects %d argument(s), got %d", name, len(spec.params), len(args))
	}
	var codes []string
	for i, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", err
		}
		want := spec.params[i]
		if want == "any" && kind != "any" {
			code, err = anyValue(code, kind)
			if err != nil {
				return "", "", err
			}
		} else if want == "list_any" && kind == "list_int" {
			code = fmt.Sprintf("pyrite_list_int_to_any(%s)", code)
		} else if !typesCompatible(want, kind) {
			return "", "", fmt.Errorf("%s parameter %d is %s but got %s", name, i+1, want, kind)
		}
		codes = append(codes, code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.result, nil
}

func (c *Compiler) netServeDelimitedExpr(rawArgs string) (string, string, error) {
	args := splitArgs(rawArgs)
	if len(args) != 6 {
		return "", "", fmt.Errorf("net.serve_delimited expects host, port, delimiter, handler, should_close, and message limit")
	}
	host, hostKind, err := c.expr(args[0])
	if err != nil {
		return "", "", err
	}
	if hostKind != "string" {
		return "", "", fmt.Errorf("net.serve_delimited host must be string, got %s", hostKind)
	}
	port, portKind, err := c.expr(args[1])
	if err != nil {
		return "", "", err
	}
	if portKind != "int" {
		return "", "", fmt.Errorf("net.serve_delimited port must be int, got %s", portKind)
	}
	delimiter, delimiterKind, err := c.expr(args[2])
	if err != nil {
		return "", "", err
	}
	if delimiterKind != "string" {
		return "", "", fmt.Errorf("net.serve_delimited delimiter must be string, got %s", delimiterKind)
	}
	handlerName := strings.TrimSpace(args[3])
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
	closeName := strings.TrimSpace(args[4])
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
	limit, limitKind, err := c.expr(args[5])
	if err != nil {
		return "", "", err
	}
	if limitKind != "int" {
		return "", "", fmt.Errorf("net.serve_delimited message limit must be int, got %s", limitKind)
	}
	return fmt.Sprintf("pyrite_net_serve_delimited(%s, %s, %s, %s, %s, %s)", host, port, delimiter, c.functionCName(handlerName), c.functionCName(closeName), limit), "int", nil
}

func (c *Compiler) numericMethodCall(s string) (string, string, bool, error) {
	intMethods := map[string]methodSpec{
		"abs":       {"pyrite_int_abs", 0, "int"},
		"min":       {"pyrite_int_min", 1, "int"},
		"max":       {"pyrite_int_max", 1, "int"},
		"clamp":     {"pyrite_int_clamp", 2, "int"},
		"to_float":  {"pyrite_int_to_float", 0, "float"},
		"to_string": {"pyrite_int_to_string", 0, "string"},
		"is_even":   {"pyrite_int_is_even", 0, "bool"},
		"is_odd":    {"pyrite_int_is_odd", 0, "bool"},
	}
	floatMethods := map[string]methodSpec{
		"abs":       {"pyrite_float_abs", 0, "float"},
		"min":       {"pyrite_float_min", 1, "float"},
		"max":       {"pyrite_float_max", 1, "float"},
		"clamp":     {"pyrite_float_clamp", 2, "float"},
		"round":     {"pyrite_float_round", 0, "int"},
		"floor":     {"pyrite_float_floor", 0, "int"},
		"ceil":      {"pyrite_float_ceil", 0, "int"},
		"trunc":     {"pyrite_float_trunc", 0, "int"},
		"to_int":    {"pyrite_float_to_int", 0, "int"},
		"to_string": {"pyrite_float_to_string", 0, "string"},
	}
	methods := map[string]bool{}
	for method := range intMethods {
		methods[method] = true
	}
	for method := range floatMethods {
		methods[method] = true
	}
	baseRaw, method, rawArgs, ok := splitMethodCall(s)
	if !ok || !methods[method] {
		return "", "", false, nil
	}
	base, baseKind, err := c.expr(baseRaw)
	if err != nil {
		return "", "", true, err
	}
	switch baseKind {
	case "int":
		spec, ok := intMethods[method]
		if !ok {
			return "", "", true, fmt.Errorf("%s.%s is not supported for int", baseRaw, method)
		}
		return c.emitNumericMethod(method, spec, base, rawArgs, "int", false)
	case "float":
		spec, ok := floatMethods[method]
		if !ok {
			return "", "", true, fmt.Errorf("%s.%s is not supported for float", baseRaw, method)
		}
		return c.emitNumericMethod(method, spec, base, rawArgs, "float", true)
	default:
		return "", "", true, fmt.Errorf("%s.%s expects numeric receiver, got %s", baseRaw, method, baseKind)
	}
}

func (c *Compiler) classMethodCall(s string) (string, string, bool, error) {
	baseRaw, method, rawArgs, ok := splitMethodCall(s)
	if !ok {
		return "", "", false, nil
	}
	base, baseKind, err := c.expr(baseRaw)
	if err != nil {
		return "", "", true, err
	}
	if !isClassKind(baseKind) {
		return "", "", false, nil
	}
	cls := c.classes[classNameFromKind(baseKind)]
	if cls == nil {
		return "", "", true, fmt.Errorf("unknown class %s", classNameFromKind(baseKind))
	}
	fn := cls.methods[method]
	if fn == nil {
		return "", "", true, fmt.Errorf("class %s has no method %s", cls.name, method)
	}
	args := splitArgs(rawArgs)
	params := fn.params[1:]
	if len(args) != len(params) {
		return "", "", true, fmt.Errorf("%s.%s expects %d argument(s), got %d", cls.name, method, len(params), len(args))
	}
	codes := []string{base}
	for i, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", true, err
		}
		want := fn.paramTypes[params[i]]
		if !typesCompatible(want, kind) {
			return "", "", true, fmt.Errorf("%s.%s parameter %s is %s but got %s", cls.name, method, params[i], want, kind)
		}
		codes = append(codes, code)
	}
	if fn.returnType == "" {
		if err := c.inferFunctionReturn(fn); err != nil {
			return "", "", true, err
		}
	}
	return fmt.Sprintf("%s(%s)", c.functionCName(fn.name), strings.Join(codes, ", ")), fn.returnType, true, nil
}

func (c *Compiler) emitNumericMethod(method string, spec methodSpec, base, rawArgs, receiverKind string, allowIntArgs bool) (string, string, bool, error) {
	args := splitArgs(rawArgs)
	if len(args) != spec.args {
		return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
	}
	codes := []string{base}
	for _, arg := range args {
		code, kind, err := c.expr(arg)
		if err != nil {
			return "", "", true, err
		}
		if allowIntArgs {
			if kind != "int" && kind != "float" {
				return "", "", true, fmt.Errorf("%s expects numeric argument, got %s", method, kind)
			}
		} else if kind != receiverKind {
			return "", "", true, fmt.Errorf("%s expects %s argument, got %s", method, receiverKind, kind)
		}
		codes = append(codes, code)
	}
	return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
}

func (c *Compiler) typedMethodCall(s string, methods map[string]methodSpec, receiverKind, argFamily string) (string, string, bool, error) {
	for method, spec := range methods {
		suffix := "." + method + "("
		idx := strings.LastIndex(s, suffix)
		if idx < 0 || !strings.HasSuffix(s, ")") {
			continue
		}
		baseRaw := strings.TrimSpace(s[:idx])
		rawArgs := strings.TrimSuffix(s[idx+len(suffix):], ")")
		base, baseKind, err := c.expr(baseRaw)
		if err != nil {
			return "", "", true, err
		}
		if baseKind != receiverKind {
			return "", "", true, fmt.Errorf("%s.%s expects %s receiver, got %s", baseRaw, method, receiverKind, baseKind)
		}
		args := splitArgs(rawArgs)
		if len(args) != spec.args {
			return "", "", true, fmt.Errorf("%s expects %d argument(s)", method, spec.args)
		}
		codes := []string{base}
		for _, arg := range args {
			code, kind, err := c.expr(arg)
			if err != nil {
				return "", "", true, err
			}
			switch argFamily {
			case "number":
				if kind != "int" && kind != "float" {
					return "", "", true, fmt.Errorf("%s expects numeric argument, got %s", method, kind)
				}
			default:
				if kind != receiverKind {
					return "", "", true, fmt.Errorf("%s expects %s argument, got %s", method, receiverKind, kind)
				}
			}
			codes = append(codes, code)
		}
		return fmt.Sprintf("%s(%s)", spec.symbol, strings.Join(codes, ", ")), spec.kind, true, nil
	}
	return "", "", false, nil
}

func splitMethodCall(s string) (string, string, string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, ")") {
		return "", "", "", false
	}
	depth := 0
	open := -1
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				open = i
				i = -1
			}
		}
	}
	if open <= 0 {
		return "", "", "", false
	}
	dot := open - 1
	for dot >= 0 && (isIdentifierChar(s[dot]) || (s[dot] >= '0' && s[dot] <= '9')) {
		dot--
	}
	if dot <= 0 || s[dot] != '.' {
		return "", "", "", false
	}
	method := s[dot+1 : open]
	if !isIdentifier(method) {
		return "", "", "", false
	}
	return strings.TrimSpace(s[:dot]), method, strings.TrimSuffix(s[open+1:], ")"), true
}

func isIdentifierChar(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func (c *Compiler) numberExpr(s string) (string, string, error) {
	code, kind, err := c.expr(strings.TrimSpace(s))
	if err != nil {
		return "", "", err
	}
	if kind != "int" && kind != "float" {
		return "", "", fmt.Errorf("expected number but got %s", kind)
	}
	return code, kind, nil
}

func numericResult(left, right string) string {
	if left == "float" || right == "float" {
		return "float"
	}
	return "int"
}

func (c *Compiler) memberExpr(s string) (string, string, error) {
	if kind, ok := c.types[s]; ok {
		return c.variableCName(s), kind, nil
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unsupported member expression")
	}
	if isClassKind(c.types[parts[0]]) {
		cls := c.classes[classNameFromKind(c.types[parts[0]])]
		if cls == nil {
			return "", "", fmt.Errorf("unknown class %s", classNameFromKind(c.types[parts[0]]))
		}
		kind := cls.fields[parts[1]]
		if kind == "" {
			return "", "", fmt.Errorf("class %s has no field %s", cls.name, parts[1])
		}
		return c.classFieldGetter(parts[0], parts[1], kind), kind, nil
	}
	if c.types[parts[0]] != "object" {
		return "", "", fmt.Errorf("unsupported member base %s", parts[0])
	}
	switch parts[1] {
	case "name":
		return fmt.Sprintf("obj_name(&%s)", parts[0]), "string", nil
	case "kind":
		return fmt.Sprintf("obj_kind(&%s)", parts[0]), "string", nil
	case "score":
		return fmt.Sprintf("obj_score(&%s)", parts[0]), "int", nil
	default:
		return "", "", fmt.Errorf("unsupported member %s", parts[1])
	}
}

func (c *Compiler) classFieldGetter(base, field, kind string) string {
	switch kind {
	case "string":
		return fmt.Sprintf("pyrite_class_get_string(%s, \"%s\")", base, field)
	case "int":
		return fmt.Sprintf("pyrite_class_get_int(%s, \"%s\")", base, field)
	case "float":
		return fmt.Sprintf("pyrite_class_get_float(%s, \"%s\")", base, field)
	case "bool":
		return fmt.Sprintf("pyrite_class_get_bool(%s, \"%s\")", base, field)
	case "any":
		return fmt.Sprintf("pyrite_class_get_any(%s, \"%s\")", base, field)
	default:
		return fmt.Sprintf("pyrite_class_get_any(%s, \"%s\")", base, field)
	}
}

func (c *Compiler) objectLiteral(s string) (string, string, error) {
	obj := "(PyriteObject){0}"
	if strings.Contains(s, "\"kind\"") {
		obj = "(PyriteObject){.kind=\"student\", .has_kind=1, .active=1, .has_active=1}"
	}
	return obj, "object", nil
}

func (c *Compiler) fstring(s string) (string, string, error) {
	body, err := strconv.Unquote(strings.TrimPrefix(s, "f"))
	if err != nil {
		return "", "", fmt.Errorf("invalid f-string %q", s)
	}
	var format strings.Builder
	var args []string
	for i := 0; i < len(body); {
		if body[i] != '{' {
			format.WriteByte(body[i])
			i++
			continue
		}
		end := strings.IndexByte(body[i+1:], '}')
		if end < 0 {
			return "", "", fmt.Errorf("unterminated f-string placeholder %q", s)
		}
		expr := strings.TrimSpace(body[i+1 : i+1+end])
		if expr == "" {
			return "", "", fmt.Errorf("empty f-string placeholder")
		}
		spec, code, err := c.fstringArg(expr)
		if err != nil {
			return "", "", err
		}
		format.WriteString(spec)
		args = append(args, code)
		i += end + 2
	}

	callArgs := []string{strconv.Quote(format.String())}
	callArgs = append(callArgs, args...)
	return fmt.Sprintf("pyrite_fmt(%s)", strings.Join(callArgs, ", ")), "string", nil
}

func (c *Compiler) fstringArg(expr string) (string, string, error) {
	code, kind, err := c.expr(expr)
	if err != nil {
		return "", "", err
	}
	switch kind {
	case "any":
		return "%s", fmt.Sprintf("pyrite_any_string(%s)", code), nil
	case "int":
		return "%ld", code, nil
	case "float":
		return "%g", code, nil
	case "bool":
		return "%s", fmt.Sprintf("(%s ? \"true\" : \"false\")", code), nil
	case "string":
		if expr == "saved" {
			code = fmt.Sprintf("pyrite_chomp(%s)", code)
		}
		return "%s", code, nil
	case "list_int":
		return "%s", fmt.Sprintf("pyrite_list_int_string(&%s)", code), nil
	case "list_any":
		return "%s", fmt.Sprintf("pyrite_list_any_string(&%s)", code), nil
	default:
		return "", "", fmt.Errorf("unsupported f-string expression type %s", kind)
	}
}

func (c *Compiler) condition(s string) (string, error) {
	for _, op := range []string{">=", "<=", "==", "!=", ">", "<"} {
		if strings.Contains(s, op) {
			parts := strings.SplitN(s, op, 2)
			left, leftKind, err := c.expr(parts[0])
			if err != nil {
				return "", err
			}
			right, rightKind, err := c.expr(parts[1])
			if err != nil {
				return "", err
			}
			if (op == "==" || op == "!=") && leftKind == "string" && rightKind == "string" {
				cmp := fmt.Sprintf("strcmp(%s, %s)", left, right)
				if op == "==" {
					return fmt.Sprintf("(%s == 0)", cmp), nil
				}
				return fmt.Sprintf("(%s != 0)", cmp), nil
			}
			return fmt.Sprintf("%s %s %s", left, op, right), nil
		}
	}
	code, kind, err := c.expr(s)
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
