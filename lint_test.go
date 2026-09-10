package dependalint

import (
	"strings"
	"testing"
)

func TestLintValid(t *testing.T) {
	config := `version: 2
registries:
  npm-private:
    type: npm-registry
    url: https://registry.example.com
    token: ${{secrets.NPM_TOKEN}}
    replaces-base: true
multi-ecosystem-groups:
  infrastructure:
    schedule:
      interval: weekly
updates:
  - package-ecosystem: npm
    directories: ["/frontend", "/admin"]
    schedule:
      interval: cron
      cronjob: "0 9 * * *"
      timezone: Asia/Tokyo
    registries: [npm-private]
    groups:
      production-deps:
        dependency-type: production
        update-types: [minor, patch]
    cooldown:
      default-days: 3
      exclude: [react]
  - package-ecosystem: docker
    directory: /
    patterns: [nginx]
    multi-ecosystem-group: infrastructure
`
	if got := Lint(strings.NewReader(config)); len(got) != 0 {
		t.Fatalf("Lint() returned diagnostics for valid config: %#v", got)
	}
}

func TestLintReportsIndependentProblems(t *testing.T) {
	config := `version: "2"
unknown: true
updates:
  - package-ecosystem: made-up
    directory: /
    directories: []
    schedule:
      interval: hourly
      day: someday
    open-pull-requests-limit: -1
    rebase-strategy: sometimes
`
	ds := Lint(strings.NewReader(config))
	want := []string{"integer 2", "unknown key", "unsupported ecosystem", "cannot be used together", "must be one of", "non-negative", "auto, disabled"}
	for _, text := range want {
		found := false
		for _, d := range ds {
			if strings.Contains(d.String(), text) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing diagnostic containing %q; got %#v", text, ds)
		}
	}
	if len(ds) < len(want) {
		t.Errorf("got only %d diagnostics", len(ds))
	}
}

func TestLintRequiredKeysAndReferences(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    registries: [missing]
    multi-ecosystem-group: missing
`
	ds := Lint(strings.NewReader(config))
	if len(ds) != 2 {
		t.Fatalf("got %d diagnostics, want 2: %#v", len(ds), ds)
	}
	for _, d := range ds {
		if d.Line == 0 || d.Column == 0 {
			t.Errorf("missing source position: %#v", d)
		}
	}
}

func TestLintMalformedYAML(t *testing.T) {
	ds := Lint(strings.NewReader("version: [\n"))
	if len(ds) != 1 || !strings.Contains(ds[0].Message, "invalid YAML") {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintEmptyConfiguration(t *testing.T) {
	for _, input := range []string{"", "  \n# comment only\n"} {
		ds := Lint(strings.NewReader(input))
		if len(ds) != 1 || ds[0].Message != "configuration is empty" {
			t.Errorf("Lint(%q) = %#v", input, ds)
		}
	}
}

func TestLintRejectsNonDecimalIntegerSyntax(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    pull-request-branch-name:
      max-length: 1_000
`
	ds := Lint(strings.NewReader(config))
	if len(ds) != 1 || ds[0].Path != "updates[0].pull-request-branch-name.max-length" || ds[0].Message != "must be a base-10 integer" {
		t.Fatalf("got %#v", ds)
	}
}
