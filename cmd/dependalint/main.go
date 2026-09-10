package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	// Embed the IANA time zone database so validation works without system zoneinfo files.
	_ "time/tzdata"

	"github.com/shogo82148/dependalint"
)

func main() { os.Exit(run()) }

func run() int {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: dependalint [FILE ...]\n\nLint Dependabot configuration files. With no FILE, .github/dependabot.yml and .github/dependabot.yaml are checked.\n")
	}
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		files = defaultFiles(".")
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

func defaultFiles(root string) []string {
	candidates := []string{
		filepath.Join(".github", "dependabot.yml"),
		filepath.Join(".github", "dependabot.yaml"),
	}
	var files []string
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		// Keep the previous error message when no default configuration exists.
		return candidates[:1]
	}
	return files
}
