package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CompileArtemisProgram(inputPath, outPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	program, err := parsePyriteProgram(string(data))
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	exitCode := 0
	for _, item := range program.Items {
		fn, ok := item.(*pyriteFunctionDecl)
		if !ok || fn.Name != "main" {
			continue
		}
		value, err := artemisExitCodeFromStmts(fn.Body)
		if err != nil {
			return err
		}
		exitCode = value
	}
	image := []byte("ARTNAT1")
	image = appendStringInstruction(image, 4, name)
	if exitCode < 0 {
		exitCode = 0
	}
	if exitCode > 126 {
		exitCode = 126
	}
	image = appendNativeExitProgram(image, byte(exitCode))
	return os.WriteFile(outPath, image, 0o644)
}

func artemisExitCodeFromStmts(stmts []pyriteStmt) (int, error) {
	exitCode := 0
	for _, stmt := range stmts {
		switch node := stmt.(type) {
		case *pyriteExprStmt:
			if call, ok := node.Expr.(*pyriteCallExpr); ok {
				if name, ok := callNameExpr(call.Callee); ok && name == "print" {
					return 0, fmt.Errorf("artemis native target does not support print yet")
				}
			}
		case *pyriteReturnStmt:
			value, err := artemisConstInt(node.Value)
			if err != nil {
				return 0, err
			}
			exitCode = value
		}
		if len(stmt.stmtBase().Children) > 0 {
			value, err := artemisExitCodeFromStmts(stmt.stmtBase().Children)
			if err != nil {
				return 0, err
			}
			exitCode = value
		}
	}
	return exitCode, nil
}

func artemisConstInt(expr pyriteExpr) (int, error) {
	value, err := evalEnumIntExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("artemis native target return must be an integer literal")
	}
	return value, nil
}

func appendStringInstruction(image []byte, opcode byte, value string) []byte {
	if len(value) > 127 {
		value = value[:127]
	}
	return append(append(image, opcode, byte(len(value))), []byte(value)...)
}

func appendNativeExitProgram(image []byte, exitCode byte) []byte {
	image = append(image, 0x6a, exitCode, 0x5f)
	image = append(image, 0x6a, 0x3c, 0x58, 0x0f, 0x05)
	return image
}
