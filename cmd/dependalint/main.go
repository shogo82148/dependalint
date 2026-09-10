package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shogo82148/dependalint"
)

func main() { os.Exit(run()) }

func run() int {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: dependalint [FILE ...]\n\nLint Dependabot configuration files. With no FILE, .github/dependabot.yml is used.\n")
	}
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		files = []string{".github/dependabot.yml"}
	}
	bad := false
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			bad = true
			continue
		}
		root := "."
		if filepath.Base(filepath.Dir(name)) == ".github" {
			root = filepath.Dir(filepath.Dir(name))
		}
		ds := dependalint.LintAt(f, root)
		f.Close()
		for _, d := range ds {
			line, col := d.Line, d.Column
			if line < 1 {
				line = 1
			}
			if col < 1 {
				col = 1
			}
			fmt.Printf("%s:%d:%d: %s\n", name, line, col, d.String())
		}
		bad = bad || len(ds) > 0
	}
	if bad {
		return 1
	}
	return 0
}
