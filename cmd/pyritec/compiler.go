package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Compiler struct {
	srcPath     string
	outPath     string
	tmpC        string
	traceDefers bool
	target      string

	imports       map[string]bool
	loadedModules map[string]bool
	types         map[string]string
	consts        map[string]bool
	globalTypes   map[string]string
	functions     map[string]*functionDef
	functionOrder []string
	classes       map[string]*classDef
	enums         map[string]map[string]int
	defers        []string
	globals       bytes.Buffer
	prototypes    bytes.Buffer
	funcs         bytes.Buffer

	inMain          bool
	mainIndent      int
	nextTryID       int
	nextRoutineID   int
	nextTempID      int
	currentFunction string
	blockStack      []block
	localDeclared   map[string]bool
	body            bytes.Buffer
}

type block struct {
	kind         string
	indent       int
	tryID        int
	switchVar    string
	switchKind   string
	caseCount    int
	cleanups     []string
	postCleanups []string
}

type functionDef struct {
	name         string
	params       []string
	paramTypes   map[string]string
	returnType   string
	indent       int
	astBody      []pyriteStmt
	nativeSymbol string
}

type classDef struct {
	name    string
	indent  int
	fields  map[string]string
	methods map[string]*functionDef
}

func NewCompiler(srcPath, outPath string) *Compiler {
	return &Compiler{
		srcPath:       srcPath,
		outPath:       outPath,
		tmpC:          filepath.Join(os.TempDir(), fmt.Sprintf("pyrite_build_%d.c", os.Getpid())),
		imports:       map[string]bool{},
		loadedModules: map[string]bool{},
		types:         map[string]string{},
		consts:        map[string]bool{},
		globalTypes:   map[string]string{},
		functions:     map[string]*functionDef{},
		classes:       map[string]*classDef{},
		enums:         map[string]map[string]int{},
	}
}

func (c *Compiler) Compile() error {
	source, err := os.ReadFile(c.srcPath)
	if err != nil {
		return err
	}

	if err := c.translate(string(source)); err != nil {
		return err
	}

	var out bytes.Buffer
	if c.traceDefers {
		out.WriteString("#define PYRITE_TRACE_DEFER 1\n")
	} else {
		out.WriteString("#define PYRITE_TRACE_DEFER 0\n")
	}
	out.WriteString(c.runtime())
	out.WriteString(c.globals.String())
	out.WriteString(c.prototypes.String())
	out.WriteString(c.funcs.String())
	out.WriteString(c.body.String())

	if err := os.WriteFile(c.tmpC, out.Bytes(), 0644); err != nil {
		return err
	}
	defer os.Remove(c.tmpC)

	args := c.compilerArgs()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("link step failed: %w", err)
	}
	return nil
}

func (c *Compiler) runtime() string {
	if c.target == "freestanding" {
		return runtimeFreestandingC
	}
	return runtimeC
}

func (c *Compiler) compilerArgs() []string {
	if c.target == "freestanding" {
		return []string{
			"gcc",
			"-std=c11",
			"-Wall",
			"-Wextra",
			"-Wno-unused-function",
			"-O2",
			"-ffreestanding",
			"-fno-stack-protector",
			"-fno-pic",
			"-mno-red-zone",
			"-c",
			"-o",
			c.outPath,
			c.tmpC,
		}
	}
	return []string{"gcc", "-std=c11", "-Wall", "-Wextra", "-Wno-unused-function", "-O2", "-pthread", "-o", c.outPath, c.tmpC, "-lm"}
}
