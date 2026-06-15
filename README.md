# ena

A command line for ena.

`ena` is a single pure-Go binary. It reads public ena data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
ena as `ena://` URIs.

## Install

```bash
go install github.com/tamnd/ena-cli/cmd/ena@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/ena-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/ena:latest --help
```

## Usage

```bash
ena page <path>                      # fetch one page as a record
ena page <path> -o json              # as JSON, ready for jq
ena page <path> --template '{{.Body}}'  # just the readable body text
ena links <path>                     # the pages it links to, one per line
ena --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

This is a fresh scaffold. It ships one example resource type, `page`, wired end
to end. Model the real ena records in `ena/` and declare their
operations in `ena/domain.go`; each one becomes a command, an HTTP
route, and an MCP tool at once.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
ena serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
ena mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`ena` registers a `ena` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/ena-cli/ena"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `ena://` URIs without knowing anything about ena:

```bash
ant get ena://page/<path>   # fetch the record
ant cat ena://page/<path>   # just the body text
ant ls  ena://page/<path>   # the pages it links to, each addressable
ant url ena://page/<path>   # the live https URL
```

## Development

```
cmd/ena/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the ena domain
ena/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/ena
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
