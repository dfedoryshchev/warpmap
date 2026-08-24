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

or from a clone, `go build -o warpmap .`. pure Go standard library - no runtime dependencies,
one static binary, nothing to configure. check it landed:

```
$ warpmap version
warpmap 0.1.0
```

`warpmap help` lists every command.

## your first audit

point it at a repo you did not write and work from the project root. churn comes out of
`git log`, so the directory has to be a git checkout - without history every hotspot scores
0.000 and the ranking tells you nothing.

the numbers below are from a small sample project, so they are small. the shape is the point.

start with the size of the thing:

```
$ warpmap analyze .
8 files, 7 import edges
```

then ask what to be careful with. `hotspots` ranks by churn x complexity - the files that keep
changing AND are hard to read:

```
$ warpmap hotspots .
1.000  churn=5   cx=25   src/store/session.ts
0.360  churn=3   cx=15   src/api/client.ts
0.096  churn=2   cx=6    src/ui/Widget.tsx
0.072  churn=1   cx=9    src/util/format.ts
```

churn is how many commits touched the file in the last 6 months. cx is a structural proxy -
non-blank lines plus branch keywords - not cyclomatic complexity. the score is normalised
against the worst churn and the worst complexity in this run, so it ranks files within one
repo and means nothing between two.

before you touch the top file, find out what it drags with it:

```
$ warpmap trace . src/store/session.ts
3 files depend on src/store/session.ts
  src/store/index.ts
  src/ui/Widget.tsx
  tests/session.test.ts
```

and where the tests are not:

```
$ warpmap testgap .
6 untested files; the riskiest (highest blast radius) first - test these before you change them:
  blast=5    src/api/client.ts
  blast=4    src/api/index.ts
  blast=4    src/util/format.ts
```

that is the whole audit loop: what is risky, what depends on it, what is untested.
`warpmap report . -o audit.md` writes the same findings to one markdown file you can hand
to someone else.

### then keep it from getting worse

snapshot the state you inherited, change something, and ask whether it got better or worse:

```
$ warpmap baseline .
baseline saved: 8 files, 7 edges, 0 cycles, 0 god-modules

$ warpmap diff .
since baseline: edges +0, cycles +0, orphans +0, god-modules +0
1 files got more complex:
  +18  src/store/session.ts
verdict: RISK UP - this change made the codebase harder to work on safely
```

`diff` exits non-zero on a net degradation, which is what makes it usable as a CI gate. the
snapshot is a single file, `.warpmap/baseline.json`.

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

## license

MIT. see [LICENSE](LICENSE); release notes are in [CHANGELOG.md](CHANGELOG.md).
