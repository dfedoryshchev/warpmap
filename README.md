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
one static binary, nothing to configure. one optional external tool widens what it measures:
with [lizard](https://github.com/terryyin/lizard) on your `PATH` (`pip install lizard`; warpmap
reads its `--csv` output as of lizard 1.24.0), `hotspots` and `ownership` take cyclomatic
complexity from it, in every language they rank. without it the internal measure runs, and
nothing else changes. check it landed:

```
$ warpmap version
warpmap 0.2.0-dev
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
complexity: internal (cx = non-blank lines + 3 x branch keywords); lizard on PATH gives cyclomatic complexity instead
1.000  churn=5   cx=25   src/store/session.ts
0.360  churn=3   cx=15   src/api/client.ts
0.096  churn=2   cx=6    src/ui/Widget.tsx
0.072  churn=1   cx=9    src/util/format.ts
```

churn is how many commits touched the file in the last 6 months. cx is a structural proxy -
non-blank lines plus branch keywords - not cyclomatic complexity. the score is normalised
against the worst churn and the worst complexity in this run, so it ranks files within one
repo and means nothing between two. test files are left out of the ranking (the same test-file
rule `testgap` uses), so a long, busy test suite does not crowd out the code it tests.

the first line goes to stderr and names the measure behind the complexity column. with lizard
on your `PATH` it reads `complexity: lizard` and the column is `ccn`: lizard's cyclomatic
complexity, summed over the functions in each file, so a file with no functions scores 0. the two
are different numbers, so a ranking taken with one does not compare with a ranking taken with
the other; if lizard is on your `PATH` but fails, the line says so and the internal measure runs.
churn needs no parser, so `hotspots` and `ownership` rank Go, C#, Java, Ruby, Rust, PHP, Swift,
Kotlin, Scala and C/C++ files as well as TypeScript, JavaScript and Python (`_test.go` files count
as tests).

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
to someone else. flags read the same wherever you put them, so
`warpmap report -o audit.md .` is that same command; an explicit `--` still ends the flags,
for a file whose name starts with a dash.

`warpmap report . --json` prints that same audit as json instead - the same counts, the same
findings, the same ranking - for a reader that is a program. `-o` writes it to a file the way
it does for markdown.

```
$ warpmap report . --json | head -12
{
  "files": 8,
  "edges": 7,
  "findings": [
    {
      "severity": "low",
      "kind": "dead-code",
      "detail": "1 files nothing imports",
      "recommendation": "confirm they are entry points, or delete them"
    }
  ],
  "hotspots": [
```

each hotspot carries its `file`, `score`, `churn` and `complexity`, down the same top ten the
markdown table prints. files are named relative to the project you pointed at, exactly as
every other command names them, so the document and the json agree file for file and neither
carries a path off the machine that ran the audit. `findings` and `hotspots` are always
lists, empty ones included, so nothing has to special-case a project with nothing to report.

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

the same loop run end to end against a real 95-file project, with the numbers it returned and
what they did and did not mean, is in [examples/worked-audit.md](examples/worked-audit.md).

## what it does

**understand**
- `analyze <dir>` - the dependency graph (`--json`, `--dot` for graphviz)
- `hotspots <dir>` - the files that change often AND are complex; start here
- `trace <dir> <file>` - blast radius: everything that depends on a file
- `dead` / `cycles` / `god` - orphans, import cycles, over-central modules
- `ownership <dir>` - knowledge risk: hotspots only one person has ever touched
- `dashboard <dir>` - the hotspot ranking as a treemap on one html page

**guard changes** (the ratchet)
- `baseline <dir>` - snapshot the current state
- `diff <dir>` - did the change make the codebase better or worse? exits non-zero on regression
- `risk <dir> <file>...` - blast radius + test gaps for the files you are about to change
- `testgap <dir>` - untested files, ranked by how much depends on them

**work with agents**
- `brief <dir> <file>` - a context pack to hand an agent before it touches a file
- `explain <dir>` - deprecated, removed in 0.3.0; `brief` and `mcp` give an agent the same
  facts to narrate
- `mcp` - run as an MCP server so an agent can query the risk map live

multi-language: the import graph, and every command built on it, reads TypeScript / JavaScript /
Python today. `hotspots` and `ownership` also rank Go, C#, Java, Ruby, Rust, PHP, Swift, Kotlin,
Scala and C/C++.

## the dashboard

a ranked list tells you the order. it does not tell you how much of the codebase the top of
that list actually is. `warpmap dashboard` answers that: the ranking as a treemap, one box per
source file, nested by directory. it ranks the TypeScript, JavaScript and Python files with the
internal measure, so on a project with other languages, or with lizard on your `PATH`, it is
not the list `hotspots` prints.

```
$ warpmap dashboard . > dashboard.html
```

a box's area is the file's complexity score and its colour is the hotspot score, so the large
red block is the file that is both hard to read and changing constantly, and the pale slivers
are the ones you can leave alone. hover a box for its path and the three numbers behind it;
the table under the map repeats the top ten, so the numbers are readable without a pointer.
a file with no code in it still gets a one-unit sliver rather than vanishing off the map.

the colour ramp runs on the square root of the score rather than the score itself. the score
is normalised churn times normalised complexity, which is heavily skewed: on a real repo
almost every file sits below 0.05, so a linear ramp paints one box red and everything else
white. the square root spreads that crowded low end without reordering anything.

the page is one file, and there is nothing in it but markup, css and inline svg. the layout
is computed before the file is written, so there is no javascript, no web font and no request
of any kind: it renders identically on a machine with no network, and it can be mailed or
committed next to the code it describes. the same project renders the same bytes twice, so
two of them diff.

`warpmap dashboard . -o dashboard.html` writes the page to a file instead of stdout, the same
way `report` does.

## configuration (optional)

drop a `warpmap.json` in the directory you analyse. every project works without one, and a
project that has none behaves exactly as it did before the file was supported.

```json
{
  "ignore": ["vendor", "src/generated"],
  "thresholds": { "blast": 10 }
}
```

`ignore` adds to the directories always skipped (`node_modules`, `.git`, `dist`, `build`,
`.venv`, `venv`, `__pycache__`, `.tox`, `.next`, `.nx`, `.turbo`); it
does not replace them, so listing your own does not bring `.git` back. a pattern with no slash
matches a directory of that name at any depth, the way the built-ins do; a pattern with a slash
is anchored where you wrote it, so `src/generated` skips that one and leaves `lib/generated`
alone.

`thresholds.blast` is where `risk` starts calling a change risky: a file with more than this
many dependents, and no test, is what it reports and exits non-zero on. the default is 10.

a malformed `warpmap.json` stops the command rather than being worked around. the numbers decide
what the tool reports, so guessing them is worse than saying so.

## on a pull request

the ratchet earns its keep in CI, so warpmap ships as a github action. it snapshots the commit
your pull request targets, compares the branch against that snapshot, and writes the result to
the pull request.

```yaml
name: warpmap
on: pull_request

permissions:
  contents: read
  pull-requests: write

jobs:
  ratchet:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - uses: dfedoryshchev/warpmap@main
```

`fetch-depth: 0` is not optional. the action exports the base commit of the pull request to take
the baseline from, and a shallow checkout does not contain it; without it the action stops and
says which commit it could not find. it builds warpmap from its own source rather than
downloading a release, so the runner needs a go toolchain, which is what `setup-go` is for. pin
`@main` to a commit if you want it to stop moving under you.

the comment carries two blocks. the first is `diff` against that baseline. the second is `risk`
over the source files the pull request adds or changes, which is where `thresholds.blast` decides
what counts as risky:

~~~
### warpmap: risk up

the ratchet, against `de4ccf0`:

```text
since baseline: edges +0, cycles +0, orphans +0, god-modules +0
1 files got more complex:
  +50  src/core.ts
verdict: RISK UP - this change made the codebase harder to work on safely
```

the 1 source file(s) this pull request adds or changes:

```text
  src/core.ts: blast=4, UNTESTED
combined blast radius: 4 files
verdict: 1 changed file(s) are high-blast AND untested - add tests before changing
```
~~~

pushing again edits that comment rather than adding another one.

| input | default | what it does |
|---|---|---|
| `directory` | `.` | the directory to analyse, relative to the repository root |
| `base-sha` | the pull request's base commit | what the baseline is taken from |
| `fail-on-risk` | `true` | `false` still reports, and leaves the job green |
| `comment` | `true` | `false` gates without writing to the pull request |
| `token` | `github.token` | needs permission to write pull request comments |
| `pr-number` | the pull request's number | which pull request to comment on |

outputs: `ratchet` (`ok` or `risk-up`), `risk` (`manageable`, `risky`, or `skipped` when the pull
request changes no source file), `report` (the comment body) and `comment-url`.

two things worth knowing. the action analyses exported copies of the two commits, so it writes
nothing into your checkout - no stray `.warpmap/` afterwards. and commenting is best-effort: with
no token, without `curl` and `jq` on the runner, or against an api that refuses the write, it logs
why, leaves the report in the job log and the job summary, and gates exactly as it would have. a
build that was going to pass does not fail because a comment did not land.

## driving it from an agent

`warpmap mcp` puts the audit behind an MCP server, so an agent can ask what a file drags with it
rather than guess. it is stdio only: one JSON-RPC request per line on stdin, one reply per line
on stdout, no port and no daemon. it answers each line as it arrives and exits 0 when stdin
closes, so a client can hold one process open for a whole run or start one per question.

start it in the directory you want analysed. the session below is the whole surface, against the
same sample project as the audit above:

```
$ warpmap mcp
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"demo","version":"0.0.0"}}}
{"id":1,"jsonrpc":"2.0","result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"warpmap","version":"0.2.0-dev"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"id":2,"jsonrpc":"2.0","result":{"tools":[{"description":"rank the riskiest files (churn x complexity)","inputSchema":{"properties":{"dir":{"type":"string"}},"required":["dir"],"type":"object"},"name":"hotspots"},{"description":"blast radius: files that depend on a given file","inputSchema":{"properties":{"dir":{"type":"string"},"file":{"type":"string"}},"required":["dir","file"],"type":"object"},"name":"trace"},{"description":"blast radius + test gaps for the files a change touches, with a verdict","inputSchema":{"properties":{"dir":{"type":"string"},"files":{"items":{"type":"string"},"type":"array"}},"required":["dir","files"],"type":"object"},"name":"risk"},{"description":"untested files ranked by blast radius","inputSchema":{"properties":{"dir":{"type":"string"}},"required":["dir"],"type":"object"},"name":"testgap"}]}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"hotspots","arguments":{"dir":"."}}}
{"id":3,"jsonrpc":"2.0","result":{"content":[{"text":"src/store/session.ts  1.000\nsrc/api/client.ts  0.360\nsrc/ui/Widget.tsx  0.096\nsrc/util/format.ts  0.072\nsrc/util/uuid.ts  0.024\nsrc/api/index.ts  0.008\nsrc/store/index.ts  0.008\n","type":"text"}]}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"trace","arguments":{"dir":".","file":"src/store/session.ts"}}}
{"id":4,"jsonrpc":"2.0","result":{"content":[{"text":"3 files depend on src/store/session.ts\nsrc/store/index.ts\nsrc/ui/Widget.tsx\ntests/session.test.ts\n","type":"text"}]}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"risk","arguments":{"dir":".","files":["src/store/session.ts"]}}}
{"id":5,"jsonrpc":"2.0","result":{"content":[{"text":"  src/store/session.ts: blast=3, tested\ncombined blast radius: 3 files\nverdict: manageable\n","type":"text"}]}}
```

the handshake advertises `tools` and nothing else: there are no resources and no prompts. the
server answers three methods, `initialize`, `tools/list` and `tools/call`. anything else,
including a line that is not valid JSON, is skipped with no reply at all, so a client must not
block waiting for one. a `tools/call` naming a tool it does not have is answered, but as
ordinary text (`unknown tool: hotspot`) rather than a JSON-RPC error.

every tool returns a single text block. `hotspots` is the ranking, up to ten lines, without the
churn and complexity columns; it ranks the TypeScript, JavaScript and Python files with the
internal measure, so it is the order `warpmap hotspots` prints when the project has no other
languages and lizard is not on the `PATH`. each line is the path
first, then two spaces, then the score, so a client can take the first field as the file and the
last as the number:

```
src/store/session.ts  1.000
src/api/client.ts  0.360
src/ui/Widget.tsx  0.096
src/util/format.ts  0.072
src/util/uuid.ts  0.024
src/api/index.ts  0.008
src/store/index.ts  0.008
```

`trace` opens with the count, then names every dependent on a line of its own, relative to
`dir` and in alphabetical order, so the list is the same from one call to the next. `testgap`
is the files no test imports, widest blast radius first, up to ten lines, each the path, two
spaces, then how many files depend on it. `risk` takes the files a change is about to touch as
`files`, a list of paths relative to `dir`, and answers with exactly the text `warpmap risk`
prints: a line per file with its blast radius and whether a test imports it, the combined blast
radius, then the verdict, judged against `thresholds.blast` from the project's `warpmap.json`.
where the command exits 1 on a risky change, the tool says so only in the verdict line.
`warpmap brief <dir> <file>` still packs the hotspot rank and the ownership count alongside the
blast radius, so an agent that can also run a command gets more than the tools carry.

four things worth knowing before wiring it up:

- `dir` is resolved against the working directory the server was started in. an absolute path
  works too.
- the paths `hotspots`, `trace` and `testgap` return are relative to `dir`, with forward
  slashes on every platform, however `dir` was spelled. that is exactly what `trace` wants as
  its `file` and `risk` wants in its `files` with the same `dir`, so the tools compose.
- `warpmap.json` is read from the analysed directory, so a project's `ignore` globs apply to
  what the server reports.
- churn comes from `git log`, so a directory with no history ranks every file 0.000, the same
  caveat as the first audit above.

what an agent does with this is the audit loop one file at a time: `hotspots` to find where the
risk sits, then `trace` on the file it is about to edit, then `risk` on the whole set of files
the change will touch. a high count is the signal to read the dependents first, or to insist on
a test, rather than to start typing; `testgap` says where that test would buy the most.

the runner i use is [agentweft](https://github.com/dfedoryshchev/agentweft), which starts an MCP
server as a subprocess and speaks the same line-delimited JSON-RPC. nothing above is specific to
it: the surface is four tools over stdio, and any MCP client can drive it.

## what talks to the network

one command, so start with it. the binary links `net/http` and carries an api endpoint as a
literal string, because `explain` can narrate a hotspot through a hosted model. it sends a
request only when both `ANTHROPIC_API_KEY` and `WARPMAP_MODEL` are set; with either one empty it
prints a fixed offline summary and makes no request. a request is one prompt per narrated
hotspot, naming the file's path and three integers - churn, complexity and blast radius - plus
the model name, with the key as a header. no file contents go into it. `explain` is
deprecated and goes in 0.3.0.

everything else stays on the machine. no package under `internal/` except `internal/explain`
links a network stack, directly or transitively, and a test holds that line:
`linkage_test.go` lists each package's dependencies with `go list` and fails on any `net` or
`net/...`, and fails again if more than one package is allowed the exception. history comes
from `git log`, run as a local subprocess against the directory you pointed at, and
`hotspots` and `ownership` also start `lizard --csv` when lizard is on your `PATH`.

two limits worth knowing. the test is about what warpmap links, not what a subprocess does:
warpmap starts `git log`, and `lizard` when it finds one, and nothing else, and what those do from
there is outside it. and
the github action is a separate script, `ci/pr-ratchet.sh`, that runs on your runner: it calls
the github api with `curl` to write the pull request comment, and runs `git fetch` for the base
commit when the checkout does not already have it.

the receipt is one test:

```
$ go test -run TestAnalysisPackagesLinkNoNetworkStack .
```

## why

most bugs in unfamiliar code come from not seeing what a change will ripple into. warpmap makes
"look before you leap" a command - and a CI gate.

## license

MIT. see [LICENSE](LICENSE); release notes are in [CHANGELOG.md](CHANGELOG.md).
