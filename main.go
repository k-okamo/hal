package main

import (
	"fmt"
	"os"
)

var (
	debug bool
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: 9ccgo <code>\n")
		os.Exit(1)
	}

	//debug = true

	// Tokenize and parse.
	tokens = tokenize(os.Args[1])
	print_tokens(tokens)
	node := parse(tokens)

	irv := gen_ir(node)
	print_irs(irv)
	alloc_regs(irv)

	fmt.Printf(".intel_syntax noprefix\n")
	fmt.Printf(".global main\n")
	fmt.Printf("main:\n")
	gen_X86(irv)
}
