package main

import (
	"fmt"
	"strings"
)

type bindingTarget struct {
	name      string
	annotated string
}

func parseBindingTarget(raw string) (bindingTarget, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, ".") {
		return bindingTarget{name: raw}, nil
	}

	parts := strings.SplitN(raw, ":", 2)
	if len(parts) == 1 {
		return bindingTarget{name: raw}, nil
	}

	name := strings.TrimSpace(parts[0])
	kind, err := normalizeType(parts[1])
	if err != nil {
		return bindingTarget{}, err
	}
	return bindingTarget{name: name, annotated: kind}, nil
}

func normalizeType(raw string) (string, error) {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), " ", "")) {
	case "int", "integer":
		return "int", nil
	case "float", "double":
		return "float", nil
	case "str", "string":
		return "string", nil
	case "bytes", "bytearray":
		return "bytes", nil
	case "bool", "boolean":
		return "bool", nil
	case "any":
		return "any", nil
	case "file":
		return "file", nil
	case "socket", "sock":
		return "socket", nil
	case "listener", "server", "serversocket":
		return "listener", nil
	case "mux", "mutex":
		return "mux", nil
	case "list", "list[int]", "list_int":
		return "list_int", nil
	case "list[any]", "list_any":
		return "list_any", nil
	case "dict", "object":
		return "object", nil
	default:
		return "", fmt.Errorf("unsupported type annotation %q", strings.TrimSpace(raw))
	}
}

func checkType(lineNo int, name, want, got string) error {
	if want == "" || typesCompatible(want, got) {
		return nil
	}
	return fmt.Errorf("line %d: %s is declared as %s but got %s", lineNo, name, want, got)
}

func typesCompatible(want, got string) bool {
	if want == got {
		return true
	}
	if want == "any" && (got == "int" || got == "float" || got == "bool" || got == "string" || got == "bytes") {
		return true
	}
	if want == "list_any" && got == "list_int" {
		return true
	}
	if strings.HasPrefix(want, "class:") || strings.HasPrefix(got, "class:") {
		return want == got
	}
	return want == "float" && got == "int"
}

func classKind(name string) string {
	return "class:" + name
}

func isClassKind(kind string) bool {
	return strings.HasPrefix(kind, "class:")
}

func classNameFromKind(kind string) string {
	return strings.TrimPrefix(kind, "class:")
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func isCallName(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	for _, part := range parts {
		if !isIdentifier(part) {
			return false
		}
	}
	return true
}

func innerCall(s, name string) string {
	s = strings.TrimSpace(s)
	prefix := name + "("
	return strings.TrimSuffix(strings.TrimPrefix(s, prefix), ")")
}

func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	depth := 0
	inString := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if !inString {
			switch ch {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case ',':
				if depth == 0 {
					args = append(args, strings.TrimSpace(cur.String()))
					cur.Reset()
					continue
				}
			}
		}
		cur.WriteByte(ch)
	}
	if strings.TrimSpace(cur.String()) != "" {
		args = append(args, strings.TrimSpace(cur.String()))
	}
	return args
}
