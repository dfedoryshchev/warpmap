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

### Changed
- `brief` lists the dependents it shows in alphabetical order, and its label says how many of
  the total it shows

### Fixed
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

### Deprecated
- `explain` will be removed in 0.3.0. `brief` and `mcp` already hand an agent the facts to
  narrate, and it is the one command that talks to a server

### Known limitations
- Imports are still extracted with regular expressions rather than a parser
- Package specifiers are skipped, as are aliases that come only through a tsconfig `extends`
  chain
- Python resolves relative imports only (`from .mod import x`)
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
