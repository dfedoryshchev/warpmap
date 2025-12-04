package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, "warpmap: map a codebase before you change it")
	fmt.Fprintln(os.Stderr, "usage: warpmap <command> [args]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hotspots":
		fmt.Fprintln(os.Stderr, "hotspots: not implemented yet")
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}
