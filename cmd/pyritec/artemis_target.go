package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var artemisPrintRE = regexp.MustCompile(`^\s*print\("([^"]*)"\)\s*$`)
var artemisReturnRE = regexp.MustCompile(`^\s*return\s+([0-9]+)\s*$`)

func CompileArtemisProgram(inputPath, outPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	exitCode := 0
	for _, line := range strings.Split(string(data), "\n") {
		if m := artemisPrintRE.FindStringSubmatch(line); m != nil {
			return fmt.Errorf("artemis native target does not support print yet: %q", m[1])
		}
		if m := artemisReturnRE.FindStringSubmatch(line); m != nil {
			value, err := strconv.Atoi(m[1])
			if err != nil {
				return err
			}
			exitCode = value
		}
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
