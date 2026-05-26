package main

import (
	"fmt"
	"strconv"
	"strings"
)

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
	case "bytes":
		return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_BYTES, .as.bytes=pyrite_bytes_copy(%s)}", code), nil
	default:
		if isClassKind(kind) {
			return fmt.Sprintf("(PyriteAny){.kind=PYRITE_ANY_CLASS, .as.obj=%s}", code), nil
		}
		return "", fmt.Errorf("list[any] does not support %s items yet", kind)
	}
}

func anyAccess(code, kind string) string {
	switch kind {
	case "any":
		return code
	case "int":
		return fmt.Sprintf("pyrite_any_as_int(%s)", code)
	case "float":
		return fmt.Sprintf("pyrite_any_as_float(%s)", code)
	case "bool":
		return fmt.Sprintf("pyrite_any_as_bool(%s)", code)
	case "string":
		return fmt.Sprintf("pyrite_any_as_string(%s)", code)
	case "bytes":
		return fmt.Sprintf("pyrite_any_as_bytes(%s)", code)
	default:
		if isClassKind(kind) {
			return fmt.Sprintf("pyrite_any_as_class(%s, \"%s\")", code, classNameFromKind(kind))
		}
		return code
	}
}

func bytesLiteral(s string) (string, string, error) {
	value, err := strconv.Unquote(strings.TrimPrefix(s, "b"))
	if err != nil {
		return "", "", fmt.Errorf("invalid bytes literal %q", s)
	}
	return fmt.Sprintf("pyrite_bytes_from_data((const unsigned char *)\"%s\", %d)", cBytesLiteral(value), len(value)), "bytes", nil
}

func cBytesLiteral(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch ch {
		case '\\':
			out.WriteString("\\\\")
		case '"':
			out.WriteString("\\\"")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if ch < 32 || ch >= 127 {
				out.WriteString(fmt.Sprintf("\\x%02x", ch))
			} else {
				out.WriteByte(ch)
			}
		}
	}
	return out.String()
}

type methodSpec struct {
	symbol string
	args   int
	kind   string
}

type compilerIntrinsicSpec struct {
	symbol string
	params []string
	result string
}

func compilerIntrinsics() map[string]compilerIntrinsicSpec {
	return map[string]compilerIntrinsicSpec{
		"chr":                   {"pyrite_chr", []string{"int"}, "string"},
		"bytes":                 {"pyrite_bytes_from_list", []string{"list_int"}, "bytes"},
		"dict":                  {"pyrite_dict_new", []string{}, "dict"},
		"set":                   {"pyrite_set_new", []string{}, "set"},
		"string_builder":        {"pyrite_string_builder_new", []string{}, "string_builder"},
		"bytes_builder":         {"pyrite_bytes_builder_new", []string{}, "bytes_builder"},
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

func numericResult(left, right string) string {
	if left == "float" || right == "float" {
		return "float"
	}
	return "int"
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

func (c *Compiler) fstringArg(source string) (string, string, error) {
	expr, err := parsePyriteExpression(source)
	if err != nil {
		return "", "", err
	}
	code, kind, err := c.exprAST(expr)
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
		if source == "saved" {
			code = fmt.Sprintf("pyrite_chomp(%s)", code)
		}
		return "%s", code, nil
	case "bytes":
		return "%s", fmt.Sprintf("pyrite_bytes_string(&%s)", code), nil
	case "list_int":
		return "%s", fmt.Sprintf("pyrite_list_int_string(&%s)", code), nil
	case "list_any":
		return "%s", fmt.Sprintf("pyrite_list_any_string(&%s)", code), nil
	default:
		return "", "", fmt.Errorf("unsupported f-string expression type %s", kind)
	}
}
