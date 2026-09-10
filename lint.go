package dependalint

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Diagnostic describes one problem in a dependabot.yml file.
type Diagnostic struct {
	Line, Column  int
	Path, Message string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

type validator struct {
	diags      []Diagnostic
	registries map[string]bool
	groups     map[string]bool
}

var ecosystems = set("bazel", "bun", "bundler", "cargo", "composer", "conda", "deno", "devcontainers", "docker", "docker-compose", "dotnet-sdk", "elm", "github-actions", "gitsubmodule", "gomod", "gradle", "helm", "julia", "maven", "mix", "nix", "npm", "nuget", "opentofu", "pip", "pre-commit", "pub", "rust-toolchain", "sbt", "swift", "terraform", "uv", "vcpkg")
var intervals = set("daily", "weekly", "monthly", "quarterly", "semiannually", "yearly", "cron")
var weekdays = set("monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday")
var updateKeys = set("package-ecosystem", "directory", "directories", "schedule", "allow", "assignees", "commit-message", "cooldown", "groups", "ignore", "insecure-external-code-execution", "labels", "milestone", "multi-ecosystem-group", "open-pull-requests-limit", "patterns", "exclude-patterns", "pull-request-branch-name", "rebase-strategy", "registries", "target-branch", "exclude-paths", "vendor", "versioning-strategy")
var timePattern = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
var groupNamePattern = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z_|-]*[A-Za-z])?$`)

// Lint reads and validates a Dependabot configuration. It returns all problems it can find.
func Lint(r io.Reader) []Diagnostic {
	var doc yaml.Node
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return []Diagnostic{{Message: "invalid YAML: " + err.Error()}}
	}
	if len(doc.Content) == 0 {
		return []Diagnostic{{Message: "configuration is empty"}}
	}
	v := &validator{registries: map[string]bool{}, groups: map[string]bool{}}
	root := deref(doc.Content[0])
	if !v.kind(root, yaml.MappingNode, "", "mapping") {
		return v.diags
	}
	v.duplicates(root, "")
	v.unknown(root, "", set("version", "updates", "registries", "multi-ecosystem-groups", "enable-beta-ecosystems"))
	version := value(root, "version")
	if version == nil {
		v.add(root, "version", "required key is missing")
	} else if version.Kind != yaml.ScalarNode || version.Tag != "!!int" || version.Value != "2" {
		v.add(version, "version", "must be the integer 2")
	}
	if regs := value(root, "registries"); regs != nil {
		v.registryDefinitions(regs)
	}
	if groups := value(root, "multi-ecosystem-groups"); groups != nil {
		v.multiGroups(groups)
	}
	if beta := value(root, "enable-beta-ecosystems"); beta != nil {
		v.boolean(beta, "enable-beta-ecosystems")
	}
	updates := value(root, "updates")
	if updates == nil {
		v.add(root, "updates", "required key is missing")
	} else if v.kind(updates, yaml.SequenceNode, "updates", "sequence") {
		if len(updates.Content) == 0 {
			v.add(updates, "updates", "must contain at least one update configuration")
		}
		for i, n := range updates.Content {
			v.update(n, fmt.Sprintf("updates[%d]", i))
		}
	}
	sort.SliceStable(v.diags, func(i, j int) bool {
		if v.diags[i].Line == v.diags[j].Line {
			return v.diags[i].Column < v.diags[j].Column
		}
		return v.diags[i].Line < v.diags[j].Line
	})
	return v.diags
}

func (v *validator) update(n *yaml.Node, p string) {
	n = deref(n)
	if !v.kind(n, yaml.MappingNode, p, "mapping") {
		return
	}
	v.duplicates(n, p)
	v.unknown(n, p, updateKeys)
	eco := v.requiredString(n, "package-ecosystem", p)
	if eco != nil && !ecosystems[eco.Value] {
		v.add(eco, p+".package-ecosystem", "unsupported ecosystem "+quote(eco.Value))
	}
	dir, dirs := value(n, "directory"), value(n, "directories")
	if dir == nil && dirs == nil {
		v.add(n, p, "one of directory or directories is required")
	}
	if dir != nil && dirs != nil {
		v.add(dirs, p+".directories", "cannot be used together with directory")
	}
	if dir != nil {
		v.string(dir, p+".directory")
	}
	if dirs != nil {
		v.stringList(dirs, p+".directories", true)
	}
	sched := value(n, "schedule")
	if sched == nil && value(n, "multi-ecosystem-group") == nil {
		v.add(n, p+".schedule", "required key is missing")
	} else if sched != nil {
		v.schedule(sched, p+".schedule")
	}
	for _, k := range []string{"assignees", "labels", "exclude-paths", "patterns", "exclude-patterns"} {
		if x := value(n, k); x != nil {
			v.stringList(x, p+"."+k, false)
		}
	}
	if x := value(n, "open-pull-requests-limit"); x != nil {
		v.nonNegativeInt(x, p+".open-pull-requests-limit")
	}
	if x := value(n, "milestone"); x != nil {
		v.nonNegativeInt(x, p+".milestone")
	}
	if x := value(n, "vendor"); x != nil {
		v.boolean(x, p+".vendor")
	}
	if x := value(n, "insecure-external-code-execution"); x != nil {
		v.enum(x, p+".insecure-external-code-execution", set("allow"))
	}
	if x := value(n, "rebase-strategy"); x != nil {
		v.enum(x, p+".rebase-strategy", set("auto", "disabled"))
	}
	if x := value(n, "versioning-strategy"); x != nil {
		v.enum(x, p+".versioning-strategy", set("auto", "increase", "increase-if-necessary", "lockfile-only", "widen"))
	}
	if x := value(n, "target-branch"); x != nil {
		v.string(x, p+".target-branch")
	}
	if x := value(n, "registries"); x != nil {
		v.registryRefs(x, p+".registries")
	}
	if x := value(n, "multi-ecosystem-group"); x != nil {
		if v.string(x, p+".multi-ecosystem-group") && !v.groups[x.Value] {
			v.add(x, p+".multi-ecosystem-group", "references an undefined multi-ecosystem group")
		}
	}
	if x := value(n, "commit-message"); x != nil {
		v.object(x, p+".commit-message", set("prefix", "prefix-development", "include"), func(k string, z *yaml.Node, q string) {
			if k == "include" {
				v.enum(z, q, set("scope"))
			} else if v.string(z, q) && len([]rune(z.Value)) > 50 {
				v.add(z, q, "must not exceed 50 characters")
			}
		})
	}
	if x := value(n, "pull-request-branch-name"); x != nil {
		v.branchName(x, p+".pull-request-branch-name")
	}
	if x := value(n, "cooldown"); x != nil {
		v.cooldown(x, p+".cooldown")
	}
	if x := value(n, "allow"); x != nil {
		v.rules(x, p+".allow", true)
	}
	if x := value(n, "ignore"); x != nil {
		v.rules(x, p+".ignore", false)
	}
	if x := value(n, "groups"); x != nil {
		v.updateGroups(x, p+".groups")
	}
}

func (v *validator) schedule(n *yaml.Node, p string) {
	if !v.kind(n, yaml.MappingNode, p, "mapping") {
		return
	}
	v.duplicates(n, p)
	v.unknown(n, p, set("interval", "day", "time", "timezone", "cronjob"))
	i := v.requiredString(n, "interval", p)
	if i == nil {
		return
	}
	if !intervals[i.Value] {
		v.add(i, p+".interval", "must be one of "+keys(intervals))
	}
	if d := value(n, "day"); d != nil {
		v.enum(d, p+".day", weekdays)
		if i.Value != "weekly" {
			v.add(d, p+".day", "is only valid with interval weekly")
		}
	}
	if t := value(n, "time"); t != nil {
		if v.string(t, p+".time") && !timePattern.MatchString(t.Value) {
			v.add(t, p+".time", "must use HH:MM 24-hour format")
		}
	}
	c := value(n, "cronjob")
	if i.Value == "cron" && c == nil {
		v.add(n, p+".cronjob", "is required with interval cron")
	}
	if c != nil {
		v.string(c, p+".cronjob")
		if i.Value != "cron" {
			v.add(c, p+".cronjob", "is only valid with interval cron")
		}
	}
	if z := value(n, "timezone"); z != nil {
		v.string(z, p+".timezone")
	}
}

func (v *validator) rules(n *yaml.Node, p string, allow bool) {
	if !v.kind(n, yaml.SequenceNode, p, "sequence") {
		return
	}
	known := set("dependency-name", "update-types")
	if allow {
		known["dependency-type"] = true
	} else {
		known["versions"] = true
	}
	for i, x := range n.Content {
		q := fmt.Sprintf("%s[%d]", p, i)
		if !v.kind(x, yaml.MappingNode, q, "mapping") {
			continue
		}
		v.duplicates(x, q)
		v.unknown(x, q, known)
		if len(x.Content) == 0 {
			v.add(x, q, "rule must not be empty")
		}
		for j := 0; j < len(x.Content); j += 2 {
			k, z := x.Content[j].Value, x.Content[j+1]
			switch k {
			case "dependency-name":
				v.string(z, q+"."+k)
			case "dependency-type":
				v.enum(z, q+"."+k, set("direct", "indirect", "all", "production", "development"))
			case "versions":
				v.stringList(z, q+"."+k, false)
			case "update-types":
				v.enumList(z, q+"."+k, set("version-update:semver-patch", "version-update:semver-minor", "version-update:semver-major"))
			}
		}
	}
}

func (v *validator) updateGroups(n *yaml.Node, p string) {
	if !v.kind(n, yaml.MappingNode, p, "mapping") {
		return
	}
	v.duplicates(n, p)
	for i := 0; i < len(n.Content); i += 2 {
		k, x := n.Content[i], n.Content[i+1]
		q := p + "." + k.Value
		if !groupNamePattern.MatchString(k.Value) {
			v.add(k, q, "group identifier must start and end with a letter and contain only letters, |, _, or -")
		}
		v.object(x, q, set("applies-to", "dependency-type", "exclude-patterns", "group-by", "patterns", "update-types"), func(key string, z *yaml.Node, r string) {
			switch key {
			case "applies-to":
				v.enum(z, r, set("version-updates", "security-updates"))
			case "dependency-type":
				v.enum(z, r, set("development", "production"))
			case "group-by":
				v.enum(z, r, set("dependency-name"))
			case "patterns", "exclude-patterns":
				v.stringList(z, r, false)
			case "update-types":
				v.enumList(z, r, set("major", "minor", "patch"))
			}
		})
	}
}

func (v *validator) cooldown(n *yaml.Node, p string) {
	v.object(n, p, set("default-days", "semver-major-days", "semver-minor-days", "semver-patch-days", "include", "exclude"), func(k string, z *yaml.Node, q string) {
		if k == "include" || k == "exclude" {
			v.stringList(z, q, false)
			if len(z.Content) > 150 {
				v.add(z, q, "must contain at most 150 dependencies")
			}
		} else {
			v.nonNegativeInt(z, q)
		}
	})
}
func (v *validator) branchName(n *yaml.Node, p string) {
	v.object(n, p, set("separator", "prefix", "max-length", "word-separator", "branch-name-case", "template"), func(k string, z *yaml.Node, q string) {
		switch k {
		case "separator", "word-separator":
			v.enum(z, q, set("-", "_", "/"))
		case "branch-name-case":
			v.enum(z, q, set("lowercase", "uppercase"))
		case "max-length":
			if v.integer(z, q) {
				var x int
				fmt.Sscan(z.Value, &x)
				if x < 20 || x > 244 {
					v.add(z, q, "must be between 20 and 244")
				}
			}
		case "prefix":
			if v.string(z, q) && len([]rune(z.Value)) > 50 {
				v.add(z, q, "must not exceed 50 characters")
			}
		case "template":
			if v.string(z, q) && len([]rune(z.Value)) > 200 {
				v.add(z, q, "must not exceed 200 characters")
			}
		}
	})
}

func (v *validator) multiGroups(n *yaml.Node) {
	if !v.kind(n, yaml.MappingNode, "multi-ecosystem-groups", "mapping") {
		return
	}
	v.duplicates(n, "multi-ecosystem-groups")
	for i := 0; i < len(n.Content); i += 2 {
		k, x := n.Content[i], n.Content[i+1]
		v.groups[k.Value] = true
		q := "multi-ecosystem-groups." + k.Value
		v.object(x, q, set("schedule", "commit-message", "pull-request-branch-name", "labels", "assignees", "milestone"), func(key string, z *yaml.Node, r string) {
			if key == "schedule" {
				v.schedule(z, r)
			}
		})
	}
}
func (v *validator) registryDefinitions(n *yaml.Node) {
	if !v.kind(n, yaml.MappingNode, "registries", "mapping") {
		return
	}
	v.duplicates(n, "registries")
	for i := 0; i < len(n.Content); i += 2 {
		k, x := n.Content[i], n.Content[i+1]
		v.registries[k.Value] = true
		q := "registries." + k.Value
		if !v.kind(x, yaml.MappingNode, q, "mapping") {
			continue
		}
		typ := value(x, "type")
		if typ == nil {
			v.add(x, q+".type", "required key is missing")
		} else {
			v.string(typ, q+".type")
		}
		for j := 0; j < len(x.Content); j += 2 {
			key, field := x.Content[j].Value, x.Content[j+1]
			if key == "replaces-base" {
				v.boolean(field, q+"."+key)
			} else {
				v.string(field, q+"."+key)
			}
		}
	}
}
func (v *validator) registryRefs(n *yaml.Node, p string) {
	if !v.kind(n, yaml.SequenceNode, p, "sequence") {
		return
	}
	for i, x := range n.Content {
		q := fmt.Sprintf("%s[%d]", p, i)
		if v.string(x, q) && x.Value != "*" && !v.registries[x.Value] {
			v.add(x, q, "references undefined registry "+quote(x.Value))
		}
	}
}

func (v *validator) object(n *yaml.Node, p string, known map[string]bool, fn func(string, *yaml.Node, string)) {
	if !v.kind(n, yaml.MappingNode, p, "mapping") {
		return
	}
	v.duplicates(n, p)
	v.unknown(n, p, known)
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i].Value
		fn(k, n.Content[i+1], p+"."+k)
	}
}
func (v *validator) requiredString(n *yaml.Node, k, p string) *yaml.Node {
	x := value(n, k)
	if x == nil {
		v.add(n, p+"."+k, "required key is missing")
		return nil
	}
	if !v.string(x, p+"."+k) {
		return nil
	}
	if x.Value == "" {
		v.add(x, p+"."+k, "must not be empty")
	}
	return x
}
func (v *validator) unknown(n *yaml.Node, p string, known map[string]bool) {
	for i := 0; i < len(n.Content); i += 2 {
		if !known[n.Content[i].Value] {
			q := n.Content[i].Value
			if p != "" {
				q = p + "." + q
			}
			v.add(n.Content[i], q, "unknown key")
		}
	}
}
func (v *validator) duplicates(n *yaml.Node, p string) {
	seen := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		if seen[k.Value] {
			q := k.Value
			if p != "" {
				q = p + "." + q
			}
			v.add(k, q, "duplicate key")
		}
		seen[k.Value] = true
	}
}
func (v *validator) stringList(n *yaml.Node, p string, nonempty bool) bool {
	if !v.kind(n, yaml.SequenceNode, p, "sequence") {
		return false
	}
	if nonempty && len(n.Content) == 0 {
		v.add(n, p, "must not be empty")
	}
	for i, x := range n.Content {
		v.string(x, fmt.Sprintf("%s[%d]", p, i))
	}
	return true
}
func (v *validator) enumList(n *yaml.Node, p string, allowed map[string]bool) {
	if !v.kind(n, yaml.SequenceNode, p, "sequence") {
		return
	}
	for i, x := range n.Content {
		v.enum(x, fmt.Sprintf("%s[%d]", p, i), allowed)
	}
}
func (v *validator) enum(n *yaml.Node, p string, a map[string]bool) {
	if v.string(n, p) && !a[n.Value] {
		v.add(n, p, "must be one of "+keys(a))
	}
}
func (v *validator) string(n *yaml.Node, p string) bool {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		v.add(n, p, "must be a string")
		return false
	}
	return true
}
func (v *validator) integer(n *yaml.Node, p string) bool {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
		v.add(n, p, "must be an integer")
		return false
	}
	return true
}
func (v *validator) nonNegativeInt(n *yaml.Node, p string) {
	if v.integer(n, p) {
		var x int
		if _, e := fmt.Sscan(n.Value, &x); e != nil || x < 0 {
			v.add(n, p, "must be a non-negative integer")
		}
	}
}
func (v *validator) boolean(n *yaml.Node, p string) {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		v.add(n, p, "must be a boolean")
	}
}
func (v *validator) kind(n *yaml.Node, k yaml.Kind, p, want string) bool {
	if deref(n).Kind != k {
		v.add(n, p, "must be a "+want)
		return false
	}
	return true
}
func (v *validator) add(n *yaml.Node, p, msg string) {
	v.diags = append(v.diags, Diagnostic{Line: n.Line, Column: n.Column, Path: p, Message: msg})
}
func value(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return deref(n.Content[i+1])
		}
	}
	return nil
}
func deref(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}
func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}
func keys(m map[string]bool) string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return strings.Join(a, ", ")
}
func quote(s string) string { return fmt.Sprintf("%q", s) }
