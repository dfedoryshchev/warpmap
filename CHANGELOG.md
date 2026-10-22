# Changelog

All notable changes to this project are documented in this file.

## Unreleased

### Added
- `warpmap.json` in the analysed directory: `ignore` globs that add to the built-in skip list,
  and `thresholds.blast` for where `risk` starts calling a change risky. A malformed file stops
  the command instead of being guessed around
- A GitHub Action that baselines the pull request's base commit, runs `diff` and `risk` against
  the branch, and writes the result as one pull request comment that later pushes edit
- `dashboard`: the hotspot ranking as a treemap on one self-contained html page (`-o` to write
  it to a file)
- `report --json`: the same audit as json, for a reader that is a program
- tsconfig `baseUrl` / `paths` aliases resolve in the import graph (`extends` chains are not
  followed)
- README: an install section and a first-audit walkthrough, the configuration file, the GitHub
  Action, driving warpmap from an agent over MCP, and what talks to the network
- `examples/worked-audit.md`: the audit loop run end to end against a real project
- `examples/agent-parks-a-hot-edit.md`: an agent that checks `risk` before an edit and hands a
  risky change to a person, with warpmap's output as the reason
- `hotspots` and `ownership` take cyclomatic complexity from `lizard` when it is on `PATH`
  (its `--csv` output, summed over each file's functions, printed as `ccn=`), and the internal
  measure (`cx=`) otherwise. A line on stderr names which one produced the numbers, and says so
  when lizard is there but fails. The binary still has no dependencies; lizard is optional
- `risk` and `testgap` as MCP tools. `risk` takes `dir` and a `files` list and answers with the
  text `warpmap risk` prints, verdict included, under the project's `thresholds.blast`;
  `testgap` lists up to ten untested files, widest blast radius first, as the path, two spaces,
  then the count
- Every command that ranks by churn prints how many commits its window holds on stderr, and
  warns under 20, where churn differences are mostly noise, or when it ranked by complexity
  alone. `--months` sets the window, 6 by default. The MCP tool text is unchanged; the line goes
  to the server's stderr
- `collect <dir> -o <folder>`: the analysis written to one new or empty folder for the person who
  ran it to read and send on: `report.json` and `analyze.json` (what `report --json` and
  `analyze --json` print), `tools.txt` (each tool's version, or `absent`), and the findings of
  gitleaks (`--redact`), lizard and jscpd when they are on `PATH`. Those three keep only listed
  fields: no matched text, duplicated code, signature, author, email or commit message goes in.
  A missing tool is listed, never fatal, and nothing is sent anywhere

### Changed
- `brief` lists the dependents it shows in alphabetical order, and its label says how many of
  the total it shows
- The MCP `trace` tool answered with the count alone; it now follows the count with every
  dependent on its own line, relative to `dir` and in alphabetical order
- `hotspots` and `ownership` rank Go, C#, Java, Ruby, Rust, PHP, Swift, Kotlin, Scala and C/C++
  files as well as TypeScript, JavaScript and Python. They read only the parsed languages before,
  so on a Go or C# repository the ranking was a stray config file or nothing. The import graph
  and the commands built on it still read only TypeScript, JavaScript and Python, and `_test.go`
  files count as tests
- `god` lists only files whose fan-in + fan-out is 20 or more, the line the report's god-module
  finding and the `baseline` / `diff` count already used, and says so when no file reaches it.
  It ranked every file before, so it could name a god module the report and the ratchet did not
  count. `--all` gives the old ranking
- `report`'s cycle and dead-code findings gave a count alone, so acting on one meant running
  `cycles` or `dead` by hand. Each finding's detail now names its files, relative to the project:
  the cycle in import order, the dead code in path order, the first ten and then `and <n> more`.
  `report --json` carries the same detail plus a `files` list on every finding, god modules
  included, that holds every file

### Fixed
- `baseline` overwrote `.warpmap/baseline.json` without a word, so running it again on a worse
  tree moved the comparison point and every later `diff` passed. It now exits 2 when a snapshot
  exists, saying when that one was saved; `baseline --force <dir>` replaces it on purpose. The
  GitHub Action snapshots a throwaway export of the base commit and passes `--force`
- tsconfig `paths` aliases were dropped when warpmap ran from inside a package whose
  `tsconfig.json` sits in a parent directory (`analyze .` from `packages/app`): the lookup never
  went above the directory it was given. It now climbs to the repository root (the directory
  holding `.git`), or the filesystem root outside a repository, so a package gives the same alias
  edges from its root and from a subdirectory
- `trace` and `risk` accepted a file that is not in the graph: `risk . missing.ts` printed
  `blast=0`, `verdict: manageable` and exited 0, so a mistyped or renamed path passed the
  check. Both now name the file on stderr and exit 2, and the MCP `trace` and `risk` tools
  answer an error result. `risk --skip-missing` assesses the files the graph holds and names
  the others on stderr; the GitHub Action passes it, so a pull request that changes a file under
  an ignored path is not failed by it
- `hotspots`, `trace`, `dead`, `cycles` and `god` printed files as walked (`..\proj\src\a.ts` on
  windows) while `testgap` printed them relative to the project; they now name a file relative
  to the project, with forward slashes
- `analyze --json` and `--dot` encoded the walked path, including an absolute one; they now use
  the same project-relative names
- A flag written after the directory was silently ignored and the command ran with its
  defaults. Flags are now read wherever they are written, so `report . -o audit.md` and
  `report -o audit.md .` are the same command; an explicit `--` still ends the flags
- `dashboard` drew a directory label on top of its children when the tree nested deeply
- A `.js` or `.jsx` specifier naming a TypeScript source (the NodeNext / ESM style) resolved to
  nothing; it now resolves to the `.ts` or `.tsx` file, ahead of any compiled `.js` beside it
- The walk read Python virtual environments and build caches as project source, so installed
  packages turned up in `dead` and one large bundle could flatten the whole `hotspots` ranking.
  `.venv`, `venv`, `__pycache__`, `.tox`, `.next`, `.nx` and `.turbo` are now skipped by default
- `testgap`, `risk` and `brief` matched the test directories (`tests/`, `__tests__/`, `e2e/`)
  against the walked path, so a top-level `tests/` went unseen when the project was named `.`,
  and every file counted as a test when the project itself sat under a directory of that name.
  They now match the path relative to the project
- `ownership` and `brief` reported `authors=0` for every file when the project was named by a
  relative path, which reads as no owner at all; they now ask git about the path relative to the
  project, so the count matches the absolute spelling
- `hotspots` ranked test files alongside the code, so on a well-tested project the top of the
  list was the test suite, and one long test file set the scale every other score was read
  against. Test files (the ones `testgap` treats as tests) are now left out of the ranking, and
  so out of `ownership`, `brief`, `dashboard`, `report` and the MCP `hotspots` tool, which read
  the same ranking
- The MCP `hotspots` tool printed each line score first, so a client that reads the first field
  as the file and the last as the score skipped every line and got an empty ranking with no
  error. Each line is now the path, two spaces, then the score. The path was also the walked one,
  absolute when `dir` was, with backslashes on windows; it is now relative to `dir` with forward
  slashes, which is the `file` that `trace` takes with the same `dir`. The command line
  `hotspots` table is unchanged
- Python imports resolved only in the relative form (`from .mod import x`), so absolute
  `from pkg.mod import x` and every `import pkg.mod` were missing from the graph, along with
  everything built on it. Both now resolve to the project's own files, in a `src/` layout and
  in a package without `__init__.py` too; a name with no file in the project, like an
  installed package or the standard library, still adds no edge. `from pkg import mod`
  now points at `pkg/mod.py` when `mod` is a submodule rather than at `pkg/__init__.py`,
  which in a package whose modules did `from . import sibling` had shown up as import cycles
  through `__init__.py`
- The walk skipped only its own short list of directories, so anything else `.gitignore`
  drops - a virtual environment under another name, a tool cache, generated output - was read
  as source; on one Python repository with 516 tracked source files it read 6,266. Inside a git
  repo the file list now comes from `git ls-files` (tracked, plus untracked files `.gitignore`
  does not drop), with the built-ins and `warpmap.json` still applied on top. A nested repo or
  submodule is listed by its own git under its own `.gitignore`, and a folder no repo covers,
  such as a code drop or a folder of checkouts, is still walked, with any repo found in it
  listed the same way. Churn and ownership are read from the repo that holds each file, so a
  folder of checkouts and a nested repo no longer rank every one of their files at churn 0 with
  no authors
- With no churn anywhere - no git, or a repo whose last commit is older than the window - every
  hotspot scored 0.000 and the table kept walk order, so the most complex file could sit last
  with nothing to say the ranking was empty. The score is now complexity alone in that case, in
  `hotspots`, `ownership`, `brief`, `report`, `dashboard` and the MCP `hotspots` tool
- Pointed at a directory below the repo root, churn missed every file, because git names a
  changed file from the root and the lookup used the path from the directory. Churn is now read
  from that directory's own changes, named relative to it
- `ownership` and `brief` counted every distinct author name as a separate person, so one
  person who committed under several spellings raised the author count and could hide a bus
  factor of 1. Authors are now read through the repo's `.mailmap`, and commits sharing an email,
  or a name up to case, count once. Different names with different emails and no `.mailmap`
  line joining them still count separately
- A `tools/call` on a directory whose `warpmap.json` is malformed ended the MCP server, so the
  client lost the session and got no reply to that call or any later one. The call is now
  answered with the error as its text and `isError` set, and the server keeps running

### Deprecated
- `explain` will be removed in 0.3.0. `brief` and `mcp` already hand an agent the facts to
  narrate, and it is the one command that talks to a server

### Known limitations
- Imports are still extracted with regular expressions rather than a parser
- Package specifiers are skipped, as are aliases that come only through a tsconfig `extends`
  chain
- Python resolves an absolute import only under the source roots warpmap can see from the
  files (the parent of each top-level package, the deepest directory holding every analysed
  file and its `src/`, and a script's own directory); paths added at run time through `sys.path` or `importlib` are not
  followed
- TypeScript / JavaScript / Python only

## 0.1.0 - 2026-08-02

### Added
- Dependency graph for TypeScript / JavaScript / Python, resolving relative imports to files
  on disk (`analyze`, with `--json` and graphviz `--dot` output)
- Churn from git history, a complexity proxy, and the churn x complexity ranking (`hotspots`)
- Blast radius of a file, with a hop limit (`trace --depth`)
- Structural findings: orphans (`dead`), import cycles (`cycles`), high fan-in/out (`god`)
- Ownership and bus factor from git history (`ownership`)
- Untested files ranked by blast radius (`testgap`)
- Markdown audit report with a severity + recommendation findings model (`report`)
- The ratchet: a baseline snapshot in `.warpmap/baseline.json` (`baseline`) and a
  better-or-worse comparison that exits non-zero on regression (`diff`)
- Change risk for the files you are about to touch (`risk`)
- Agent context pack for a single file (`brief`)
- Hotspot narration through an LLM, with a deterministic offline fallback (`explain`)
- MCP server over stdio, exposing hotspots and trace to an agent (`mcp`)

### Known limitations
- Imports are extracted with regular expressions rather than a parser, so unusual forms
  (a specifier built at runtime, for instance) can be missed
- Only relative specifiers resolve; path aliases and package specifiers are skipped
- Markdown is the only report format; there is no config file and no CI action yet
