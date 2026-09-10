package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "neither exists", want: []string{filepath.Join(".github", "dependabot.yml")}},
		{name: "yml", files: []string{"dependabot.yml"}, want: []string{filepath.Join(".github", "dependabot.yml")}},
		{name: "yaml", files: []string{"dependabot.yaml"}, want: []string{filepath.Join(".github", "dependabot.yaml")}},
		{name: "both", files: []string{"dependabot.yml", "dependabot.yaml"}, want: []string{
			filepath.Join(".github", "dependabot.yml"),
			filepath.Join(".github", "dependabot.yaml"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			githubDir := filepath.Join(root, ".github")
			if err := os.Mkdir(githubDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(githubDir, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if got := defaultFiles(root); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("defaultFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}
