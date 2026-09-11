# dependalint

`dependalint` is a command-line linter for GitHub Dependabot configuration files,
written in Go. It checks YAML syntax as well as Dependabot-specific keys, types,
required values, enumerations, references, and option combinations.

## Install

### Install script

Download a prebuilt binary with the install script. It detects your OS/arch,
verifies the download against an embedded checksum, and installs the binary.

```console
curl -sSfL https://raw.githubusercontent.com/shogo82148/dependalint/main/install.sh | sh -s -- -b /usr/local/bin
```

By default the binary is installed to `${HOME}/.local/bin`. Pass `-b <dir>` to
choose another directory, and append a tag (e.g. `v0.1.0`) to pin a version
instead of the latest release:

```console
curl -sSfL https://raw.githubusercontent.com/shogo82148/dependalint/main/install.sh | sh -s -- -b /usr/local/bin v0.1.0
```

### go install

```console
go install github.com/shogo82148/dependalint/cmd/dependalint@latest
```

## Usage

Run it without arguments to check any existing `.github/dependabot.yml` and
`.github/dependabot.yaml` files:

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
