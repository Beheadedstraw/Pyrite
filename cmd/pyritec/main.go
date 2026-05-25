package main

import (
	"fmt"
	"os"
)

func main() {
	inputPath, outPath, traceDefers, target, ok := parseArgs(os.Args[1:])
	if !ok {
		usage()
		os.Exit(2)
	}

	c := NewCompiler(inputPath, outPath)
	c.traceDefers = traceDefers
	c.target = target
	if err := c.Compile(); err != nil {
		fmt.Fprintln(os.Stderr, "pyritec:", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (inputPath, outPath string, traceDefers bool, target string, ok bool) {
	target = "hosted"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--trace-defer":
			traceDefers = true
		case "--freestanding":
			target = "freestanding"
		case "--target":
			if i+1 >= len(args) {
				return "", "", false, "", false
			}
			i++
			target = args[i]
		case "-o", "--output":
			if i+1 >= len(args) {
				return "", "", false, "", false
			}
			i++
			outPath = args[i]
		default:
			if inputPath != "" || args[i] == "" || args[i][0] == '-' {
				return "", "", false, "", false
			}
			inputPath = args[i]
		}
	}
	if target != "hosted" && target != "freestanding" {
		return "", "", false, "", false
	}
	return inputPath, outPath, traceDefers, target, inputPath != "" && outPath != ""
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: pyritec [--trace-defer] [--target hosted|freestanding] <input.pyr> -o <output>")
}
