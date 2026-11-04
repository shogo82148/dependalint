package dependalint

import (
	"os"
	"path/filepath"
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
	root := t.TempDir()
	for _, dir := range []string{"frontend", "admin"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := LintAt(strings.NewReader(config), root); len(got) != 0 {
		t.Fatalf("Lint() returned diagnostics for valid config: %#v", got)
	}
}

func TestLintValidAgentPackageManager(t *testing.T) {
	config := `version: 2
registries:
  github-octocat:
    type: git
    url: https://github.com
    username: x-access-token
    password: ${{secrets.MY_GITHUB_PERSONAL_TOKEN}}
updates:
  - package-ecosystem: apm
    directory: /
    schedule:
      interval: weekly
    registries: [github-octocat]
`
	if got := Lint(strings.NewReader(config)); len(got) != 0 {
		t.Fatalf("Lint() returned diagnostics for valid config: %#v", got)
	}
}

func TestLintChecksDirectoriesExist(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "present"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: 2
updates:
  - package-ecosystem: npm
    directories: ["/present", "/missing"]
    schedule:
      interval: weekly
`
	ds := LintAt(strings.NewReader(config), root)
	if len(ds) != 1 || ds[0].Path != "updates[0].directories[1]" || ds[0].Message != "directory does not exist" {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintOnlyAllowsGlobPatternsInDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "packages", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "packages", "*?["), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: "/packages/*"
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directories: ["/packages/*"]
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directory: '/packages/\*\?\['
    schedule:
      interval: weekly
`
	ds := LintAt(strings.NewReader(config), root)
	if len(ds) != 1 || ds[0].Path != "updates[0].directory" || ds[0].Message != "does not support glob patterns" {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintRejectsParentDirectoryReferences(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: "/../outside"
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directories: ["/packages/../outside"]
    schedule:
      interval: weekly
`
	ds := LintAt(strings.NewReader(config), t.TempDir())
	if len(ds) != 2 {
		t.Fatalf("got %d diagnostics, want 2: %#v", len(ds), ds)
	}
	for _, d := range ds {
		if d.Message != `must not include ".."` {
			t.Errorf("got %#v", d)
		}
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

func TestLintEmptyRequiredStringReportedOnce(t *testing.T) {
	// An empty package-ecosystem or interval must not also be reported as an
	// invalid enumeration value.
	config := `version: 2
updates:
  - package-ecosystem: ""
    directory: /
    schedule:
      interval: ""
`
	ds := Lint(strings.NewReader(config))
	var got []string
	for _, d := range ds {
		got = append(got, d.String())
	}
	want := map[string]bool{
		"updates[0].package-ecosystem: must not be empty": true,
		"updates[0].schedule.interval: must not be empty": true,
	}
	if len(ds) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %#v", len(ds), len(want), got)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected diagnostic %q; got %#v", g, got)
		}
	}
}

func TestLintGroupIdentifiers(t *testing.T) {
	// Names accepted by Dependabot in the wild: digits and dots are allowed
	// (e.g. grafana/grafana uses "go.opentelemetry.io" and "k8s.io").
	for _, name := range []string{"go.opentelemetry.io", "k8s.io", "k8s-dependencies", "d3", "prod|dev", "a"} {
		config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    groups:
      ` + name + `:
        patterns: ["*"]
`
		if ds := Lint(strings.NewReader(config)); len(ds) != 0 {
			t.Errorf("group %q: got %#v, want no diagnostics", name, ds)
		}
	}

	// Names that break the resulting Git branch name are still rejected.
	for _, name := range []string{"GitHub Actions", ".leading", "trailing.", "-lead"} {
		config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    groups:
      "` + name + `":
        patterns: ["*"]
`
		ds := Lint(strings.NewReader(config))
		if len(ds) != 1 || !strings.Contains(ds[0].Message, "group identifier") {
			t.Errorf("group %q: got %#v, want a group identifier diagnostic", name, ds)
		}
	}
}

func TestLintRegistriesWildcard(t *testing.T) {
	// The scalar "*" wildcard selects every defined registry and is valid.
	config := `version: 2
registries:
  npm-private:
    type: npm-registry
    url: https://registry.example.com
    token: ${{secrets.NPM_TOKEN}}
updates:
  - package-ecosystem: npm
    directory: /
    registries: "*"
    schedule:
      interval: weekly
`
	if ds := Lint(strings.NewReader(config)); len(ds) != 0 {
		t.Fatalf("registries: \"*\" should be valid, got %#v", ds)
	}

	// A bare scalar that is not the wildcard must be a list of names.
	bad := `version: 2
registries:
  npm-private:
    type: npm-registry
    url: https://registry.example.com
    token: ${{secrets.NPM_TOKEN}}
updates:
  - package-ecosystem: npm
    directory: /
    registries: npm-private
    schedule:
      interval: weekly
`
	ds := Lint(strings.NewReader(bad))
	if len(ds) != 1 || ds[0].Path != "updates[0].registries" || !strings.Contains(ds[0].Message, `"*" wildcard`) {
		t.Fatalf("got %#v", ds)
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

func TestLintRejectsInvalidScheduleTimezone(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
      timezone: Asia/Not_A_Real_Place
`
	ds := Lint(strings.NewReader(config))
	if len(ds) != 1 || ds[0].Path != "updates[0].schedule.timezone" || ds[0].Message != "must be a valid time zone database identifier" {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintRejectsLocalScheduleTimezone(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
      timezone: Local
`
	ds := Lint(strings.NewReader(config))
	if len(ds) != 1 || ds[0].Path != "updates[0].schedule.timezone" || ds[0].Message != "must be a valid time zone database identifier" {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintValidScheduleCronjobs(t *testing.T) {
	for _, cronjob := range []string{
		"0 9 * * *",
		"0 9 * JAN,MAR MON-FRI",
		"every day at 5pm",
		"every weekday at 16:30",
		"every 24 hours",
		"@daily",
		"0 0 L * *",
		"0 0 last * *",
		"0 0 -7-L * *",
		"0 5 * * mon#1",
		"0 7 * * fri#L",
		"0 7 * * fri#-1",
		"0 9 * * sun%2",
		"0 9 * * sun%2+1",
		"0 0 1-7 * mon&",
		"~ 12 * * *",
		"every day at five",
		"every wed at 5 pm",
		"every tuesday and monday at 5pm",
		" 0 9 * * * ",
	} {
		config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: cron
      cronjob: "` + cronjob + `"
`
		if ds := Lint(strings.NewReader(config)); len(ds) != 0 {
			t.Errorf("cronjob %q: got %#v", cronjob, ds)
		}
	}
}

func TestLintRejectsScheduleCronjobsMoreFrequentThanDaily(t *testing.T) {
	for _, cronjob := range []string{
		"*/15 8-17 * JAN,MAR MON-FRI",
		"0 9,17 * * *",
		"every minute",
		"every 3 hours",
		"every 23 hours",
	} {
		config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: cron
      cronjob: "` + cronjob + `"
`
		ds := Lint(strings.NewReader(config))
		if len(ds) != 1 || ds[0].Path != "updates[0].schedule.cronjob" || ds[0].Message != "must have a minimum interval of 24 hours" {
			t.Errorf("cronjob %q: got %#v", cronjob, ds)
		}
	}
}

func TestLintRejectsInvalidScheduleCronjobs(t *testing.T) {
	for _, cronjob := range []string{
		"",
		"0 9 * *",
		"60 9 * * *",
		"0 24 * * *",
		"0 9 * * * UTC",
		"every nonsense",
	} {
		config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: cron
      cronjob: "` + cronjob + `"
`
		ds := Lint(strings.NewReader(config))
		if len(ds) != 1 || ds[0].Path != "updates[0].schedule.cronjob" || ds[0].Message != "must be a valid cron or natural expression" {
			t.Errorf("cronjob %q: got %#v", cronjob, ds)
		}
	}
}

func TestLintEcosystemSpecificOptions(t *testing.T) {
	tests := []struct {
		eco, option, path string
		supported         bool
	}{
		{"bundler", "vendor: true", "updates[0].vendor", true},
		{"gomod", "vendor: true", "updates[0].vendor", true},
		{"npm", "vendor: true", "updates[0].vendor", false},
		{"npm", "versioning-strategy: increase", "updates[0].versioning-strategy", true},
		{"helm", "versioning-strategy: increase", "updates[0].versioning-strategy", true},
		{"gomod", "versioning-strategy: increase", "updates[0].versioning-strategy", false},
		{"docker", "versioning-strategy: increase", "updates[0].versioning-strategy", false},
		{"pip", "insecure-external-code-execution: allow", "updates[0].insecure-external-code-execution", true},
		{"mix", "insecure-external-code-execution: allow", "updates[0].insecure-external-code-execution", true},
		{"npm", "insecure-external-code-execution: allow", "updates[0].insecure-external-code-execution", false},
		{"gomod", "allow: [{dependency-type: indirect}]", "updates[0].allow[0].dependency-type", true},
		{"uv", "allow: [{dependency-type: indirect}]", "updates[0].allow[0].dependency-type", true},
		{"npm", "allow: [{dependency-type: indirect}]", "updates[0].allow[0].dependency-type", false},
		{"npm", "allow: [{dependency-type: all}]", "updates[0].allow[0].dependency-type", true},
		{"docker", "allow: [{dependency-type: direct}]", "updates[0].allow[0].dependency-type", true},
		{"npm", "cooldown: {semver-major-days: 7}", "updates[0].cooldown.semver-major-days", true},
		{"mix", "cooldown: {semver-minor-days: 7}", "updates[0].cooldown.semver-minor-days", true},
		{"github-actions", "cooldown: {semver-patch-days: 7}", "updates[0].cooldown.semver-patch-days", false},
		{"docker", "cooldown: {semver-major-days: 7}", "updates[0].cooldown.semver-major-days", false},
		{"docker", "cooldown: {default-days: 7}", "updates[0].cooldown.default-days", true},
	}
	for _, tt := range tests {
		config := `version: 2
updates:
  - package-ecosystem: ` + tt.eco + `
    directory: /
    schedule:
      interval: weekly
    ` + tt.option + `
`
		ds := Lint(strings.NewReader(config))
		if tt.supported {
			if len(ds) != 0 {
				t.Errorf("%s with %q: got %#v, want no diagnostics", tt.eco, tt.option, ds)
			}
			continue
		}
		if len(ds) != 1 || ds[0].Path != tt.path || !strings.HasSuffix(ds[0].Message, `not supported by package-ecosystem "`+tt.eco+`"`) {
			t.Errorf("%s with %q: got %#v, want an unsupported diagnostic at %s", tt.eco, tt.option, ds, tt.path)
		}
	}
}

func TestLintEcosystemSpecificValueMessage(t *testing.T) {
	config := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    vendor: true
    allow:
      - dependency-type: indirect
`
	ds := Lint(strings.NewReader(config))
	want := []string{
		`updates[0].vendor: not supported by package-ecosystem "npm"`,
		`updates[0].allow[0].dependency-type: "indirect" is not supported by package-ecosystem "npm"`,
	}
	if len(ds) != len(want) {
		t.Fatalf("got %#v", ds)
	}
	for i, d := range ds {
		if d.String() != want[i] || d.Line == 0 {
			t.Errorf("got %#v, want %q", d, want[i])
		}
	}
}

func TestLintEcosystemSpecificOptionsSkipUnknownEcosystem(t *testing.T) {
	// An unknown ecosystem is reported once; its options are not also flagged.
	config := `version: 2
updates:
  - package-ecosystem: made-up
    directory: /
    schedule:
      interval: weekly
    vendor: true
`
	ds := Lint(strings.NewReader(config))
	if len(ds) != 1 || ds[0].Path != "updates[0].package-ecosystem" {
		t.Fatalf("got %#v", ds)
	}
}

func TestLintEcosystemSpecificOptionReportedAtAlias(t *testing.T) {
	// The first update defines anchors that are valid for gomod; the second
	// update reuses one of them. The diagnostic must point at the alias.
	tests := []struct {
		eco, use, path string
	}{
		{"npm", "vendor: *vendor", "updates[1].vendor"},
		{"npm", "allow: *allow", "updates[1].allow[0].dependency-type"},
		{"npm", "allow: [*rule]", "updates[1].allow[0].dependency-type"},
		{"npm", "allow: [{dependency-type: *kind}]", "updates[1].allow[0].dependency-type"},
		{"docker", "cooldown: *cooldown", "updates[1].cooldown.semver-major-days"},
	}
	for _, tt := range tests {
		config := `version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
    vendor: &vendor true
    allow: &allow
      - &rule
        dependency-type: &kind indirect
    cooldown: &cooldown
      semver-major-days: 7
  - package-ecosystem: ` + tt.eco + `
    directory: /
    schedule:
      interval: weekly
    ` + tt.use + `
`
		ds := Lint(strings.NewReader(config))
		if len(ds) != 1 || ds[0].Path != tt.path || ds[0].Line != 17 || !strings.HasSuffix(ds[0].Message, `not supported by package-ecosystem "`+tt.eco+`"`) {
			t.Errorf("%s: got %#v, want an unsupported diagnostic at %s on line 17", tt.use, ds, tt.path)
		}
	}
}
