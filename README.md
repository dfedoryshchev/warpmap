# warpmap

map a codebase before you change it.

warpmap is a Go CLI that makes a legacy or complex codebase legible and safe to change - for
humans and for the AI agents working on it. it builds a dependency graph, ranks the risk, and
guards changes against a baseline. extracted and sanitized from a private production toolkit
i have used on real codebases since 2025.

## install

```
go install github.com/dfedoryshchev/warpmap@latest
```

or build from source with `go build -o warpmap .`. pure Go standard library - no runtime
dependencies, one static binary.

## what it does

**understand**
- `analyze <dir>` - the dependency graph (`--json`, `--dot` for graphviz)
- `hotspots <dir>` - the files that change often AND are complex; start here
- `trace <dir> <file>` - blast radius: everything that depends on a file
- `dead` / `cycles` / `god` - orphans, import cycles, over-central modules
- `ownership <dir>` - knowledge risk: hotspots only one person has ever touched

**guard changes** (the ratchet)
- `baseline <dir>` - snapshot the current state
- `diff <dir>` - did the change make the codebase better or worse? exits non-zero on regression
- `risk <dir> <file>...` - blast radius + test gaps for the files you are about to change
- `testgap <dir>` - untested files, ranked by how much depends on them

**work with agents**
- `brief <dir> <file>` - a context pack to hand an agent before it touches a file
- `explain <dir>` - narrate the top hotspot in plain language (LLM; runs offline without a key)
- `mcp` - run as an MCP server so an agent can query the risk map live

multi-language: TypeScript / JavaScript / Python today.

## why

most bugs in unfamiliar code come from not seeing what a change will ripple into. warpmap makes
"look before you leap" a command - and a CI gate.
