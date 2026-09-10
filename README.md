# dependalint

`dependalint` is a command-line linter for GitHub Dependabot configuration files,
written in Go. It checks YAML syntax as well as Dependabot-specific keys, types,
required values, enumerations, references, and option combinations.

## Install

```console
go install github.com/shogo82148/dependalint/cmd/dependalint@latest
```

## Usage

Run it without arguments to check `.github/dependabot.yml`:

```console
dependalint
```

You can also pass one or more paths:

```console
dependalint .github/dependabot.yml testdata/example.yml
```

Diagnostics use the familiar `file:line:column` format and the process exits
with status 1 when a problem is found:

```text
.github/dependabot.yml:5:24: updates[0].package-ecosystem: unsupported ecosystem "node"
```

The validation rules follow GitHub's
[Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference).

## Development

```console
go test ./...
go vet ./...
```
