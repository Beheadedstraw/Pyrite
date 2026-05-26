package main

import (
	"fmt"
	"strings"
)

func normalizeType(raw string) (string, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	compact := strings.ToLower(cleaned)
	if strings.HasPrefix(compact, "list[") && strings.HasSuffix(compact, "]") {
		innerRaw := cleaned[5 : len(cleaned)-1]
		inner, err := normalizeType(innerRaw)
		if err != nil {
			if isQualifiedIdentifier(innerRaw) {
				return "list:" + classKind(innerRaw), nil
			}
			return "", err
		}
		if inner == "int" {
			return "list_int", nil
		}
		if inner == "any" {
			return "list_any", nil
		}
		return "list:" + inner, nil
	}
	if strings.HasPrefix(compact, "dict[") && strings.HasSuffix(compact, "]") {
		innerRaw := cleaned[5 : len(cleaned)-1]
		inner, err := normalizeType(innerRaw)
		if err != nil {
			if isQualifiedIdentifier(innerRaw) {
				return "dict:" + classKind(innerRaw), nil
			}
			return "", err
		}
		if inner == "any" {
			return "dict", nil
		}
		return "dict:" + inner, nil
	}
	switch compact {
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
	case "dict", "map":
		return "dict", nil
	case "set":
		return "set", nil
	case "stringbuilder", "string_builder":
		return "string_builder", nil
	case "bytesbuilder", "bytes_builder":
		return "bytes_builder", nil
	case "object":
		return "object", nil
	default:
		if isQualifiedIdentifier(cleaned) {
			return classKind(cleaned), nil
		}
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
	if got == "none" && (want == "any" || want == "string" || isClassKind(want)) {
		return true
	}
	if want == "any" && (got == "none" || got == "int" || got == "float" || got == "bool" || got == "string" || got == "bytes" || isListKind(got) || isDictKind(got) || isClassKind(got)) {
		return true
	}
	if want == "list_any" && isListKind(got) {
		return true
	}
	if strings.HasPrefix(want, "list:") && got == "list_any" {
		return true
	}
	if strings.HasPrefix(want, "dict:") && got == "dict" {
		return true
	}
	if strings.HasPrefix(want, "class:") || strings.HasPrefix(got, "class:") {
		return want == got
	}
	return want == "float" && got == "int"
}

func isListKind(kind string) bool {
	return kind == "list_int" || kind == "list_any" || strings.HasPrefix(kind, "list:")
}

func listElementKind(kind string) string {
	switch {
	case kind == "list_int":
		return "int"
	case kind == "list_any":
		return "any"
	case strings.HasPrefix(kind, "list:"):
		return strings.TrimPrefix(kind, "list:")
	default:
		return "any"
	}
}

func isDictKind(kind string) bool {
	return kind == "dict" || strings.HasPrefix(kind, "dict:")
}

func dictValueKind(kind string) string {
	if strings.HasPrefix(kind, "dict:") {
		return strings.TrimPrefix(kind, "dict:")
	}
	return "any"
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

func isQualifiedIdentifier(s string) bool {
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
