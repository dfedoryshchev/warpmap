# a worked audit

the readme shows the shape of an audit on a small sample project. this is the same loop run end
to end against a real repository, with the numbers it actually returned.

the target is [react-formkit](https://github.com/dfedoryshchev/react-formkit), a react form
library i also wrote: 131 tracked files at commit `45bcba0`, 95 of them typescript, 157 commits
of history. every warpmap block below is output from `warpmap 0.1.0` against that commit,
trimmed where it says so and otherwise unedited.

two of the numbers move under you. churn counts the commits of the last six months, so it drifts
with the calendar, and the hotspot score is normalised against the worst file in the same run, so
it drifts when churn does; these were taken in september 2026. the structural numbers - edges,
blast radius, cycles, fan-in - are fixed by the commit: run the same way, they come back the
same.

## get a full clone

```
$ git clone https://github.com/dfedoryshchev/react-formkit
$ cd react-formkit
$ git checkout 45bcba0
```

churn comes out of `git log`, so warpmap wants the history: a zip download or a `--depth 1`
clone ranks every file 0.000 and the ranking tells you nothing. the checkout pins the commit
these numbers were taken at, since the project keeps moving. everything below is run from the
project root, spelled `.`.

## how big is it

```
$ warpmap analyze .
95 files, 170 import edges
```

95 of the 131 tracked files. warpmap parses typescript, javascript and python, so the 20
stylesheets, the json and the markdown are not in the graph and nothing below is a statement
about them.

this project imports across its own modules with the `@/...` alias declared in the `paths` table
of its `tsconfig.json`. warpmap reads that table, so those are edges like any other; the nineteen
`@/` imports in the tree all resolve. the consequence shows up further down rather than in a
flag.

## what to be careful with

```
$ warpmap hotspots . -n 8
0.545  churn=12  cx=297  src/controls/Control.tsx
0.333  churn=4   cx=545  tests/validators/async.validators.test.ts
0.224  churn=4   cx=366  tests/components/AsyncValidation.test.tsx
0.198  churn=4   cx=323  src/validation/validators/async.validators.ts
0.171  churn=5   cx=224  dev/App.tsx
0.129  churn=4   cx=211  src/config/buildSchema.ts
0.109  churn=5   cx=142  src/form/BasicForm.tsx
0.082  churn=2   cx=269  tests/components/FormFieldArray.test.tsx
```

three things to read out of that.

the top file is the one to be careful with, and the reason is the pair rather than either
number: `Control.tsx` is 222 non-blank lines holding an 18-case switch that maps a control type
to a component, and twelve commits landed in it during the window. big and quiet would be fine.
small and busy would be fine. this is both.

two of the top four are test files. warpmap ranks every file it walks, tests included, and a
long test file that keeps changing is a real maintenance cost - but if what you want is the
ranking of the shipped code, read past them.

`cx` is not cyclomatic complexity. it is non-blank lines plus three per branch keyword, so
`Control.tsx` scores 222 + 25 * 3 = 297. it is a proxy for "how much is going on in here",
normalised inside one run, and it cannot be compared against another repository's numbers.

## what the top file drags with it

```
$ warpmap trace . src/controls/Control.tsx
22 files depend on src/controls/Control.tsx
```

```
$ warpmap trace . src/controls/Control.tsx --depth 1
4 files depend on src/controls/Control.tsx
  src/field/Field.tsx
  tests/components/Control.test.tsx
  src/config/config.types.ts
  src/controls/index.ts
```

four files import it directly and twenty-two reach it once the imports are followed all the way
out. the direct number is who to talk to; the full number is how much to test before you touch
it. the
dependents print in no particular order, so two runs list the same files differently.

the alias table earns its place here:

```
$ warpmap trace . src/validation/messages.ts --depth 1
14 files depend on src/validation/messages.ts
```

three of those fourteen - `src/config/buildSchema.ts`, `src/form/BasicForm.tsx` and
`src/form/Form.tsx` - name it as `@/validation/messages` and nothing else. a tool that resolved
only relative imports would put them nowhere near this file.

## where the tests are not

```
$ warpmap testgap . -n 6
56 untested files; the riskiest (highest blast radius) first - test these before you change them:
  blast=36   src/controls/control.types.ts
  blast=28   src/controls/utils/select.utils.ts
  blast=25   src/validation/schema.utils.ts
  blast=25   src/controls/toggles/RadioInput.tsx
  blast=24   src/controls/toggles/withWrappingLabel.tsx
  blast=24   src/controls/selects/AutocompleteInput.tsx
```

this is structural, not line coverage: a file counts as exercised when a test file imports it,
and as untested when none does. so read the list as "files no test names", which is a different
sentence from "files that need a test".

the first row proves the point. `control.types.ts` is 60 lines of type declarations. there is
nothing in it to assert on, and it sits at the top because 36 files are downstream of it. the
fourth row is the kind that is worth acting on: `RadioInput.tsx` is a real component, no test
names it, and it is only ever reached through `RadioGroup` and the toggles barrel - so every
check it currently gets is incidental.

## dead code, cycles, central modules

```
$ warpmap dead .
4 files nothing imports (candidate dead code):
  dev/vite.config.ts
  tests/setup.ts
  vite.config.ts
  vitest.config.ts
```

all four are tool entry points: vite and vitest load them by name, and `tests/setup.ts` is
listed in `vitest.config.ts` as a setup file rather than imported by anything. that is the
correct answer and it is also the edge of the signal - `dead` sees imports, not tool
configuration, so its own advice to confirm they are entry points is the whole job here.

```
$ warpmap cycles .
0 import cycles:
```

```
$ warpmap god . -n 5
files too many things depend on, or that depend on too much:
  in=11  out=5   src/field/index.ts
  in=1   out=14  src/validation/index.ts
  in=14  out=0   src/validation/messages.ts
  in=13  out=0   src/controls/control.types.ts
  in=8   out=4   src/form/index.ts
```

the `index.ts` rows are barrels, which is what a library that re-exports its public surface
looks like from the graph; they are central by design. the row worth a second look is
`src/validation/messages.ts` at `in=14 out=0` - fourteen files import it and it imports nothing,
so it is a leaf the whole project leans on. note that its fan-in of 14 and the 32 that `trace`
returns for it are answers to different questions: direct importers against everything
transitively downstream.

## the whole audit in one file

```
$ warpmap report .
# warpmap audit

- files: 95
- import edges: 170
- findings: 1

## findings

- **[low] dead-code** - 4 files nothing imports
  - confirm they are entry points, or delete them

## hotspots

| score | churn | complexity | file |
| ----- | ----- | ---------- | ---- |
| 0.545 | 12 | 297 | src/controls/Control.tsx |
| 0.333 | 4 | 545 | tests/validators/async.validators.test.ts |
| 0.224 | 4 | 366 | tests/components/AsyncValidation.test.tsx |
```

trimmed after three of the ten hotspot rows. `warpmap report . -o audit.md` writes the same
markdown to a file instead of stdout, which is the form to hand to someone else.

the agent-facing version of the same material is `brief`, one file at a time:

```
$ warpmap brief . src/controls/Control.tsx
# brief: src/controls/Control.tsx

- hotspot rank: 1 of 95 (churn 12, complexity 297)
- blast radius: 22 files depend on this - changing it can ripple widely
- authors who have touched it: 1 (bus factor 1)
- has a test importing it: true
```

trimmed: it goes on to list up to eight of the dependents.

## then keep it from getting worse

the audit above is a photograph. the ratchet is the part that keeps working after you close the
terminal: snapshot a state, do some work, ask whether it got better or worse. run against real
history, that is nine commits of this project's own development.

```
$ git checkout 4024bd6
$ warpmap baseline .
baseline saved: 86 files, 149 edges, 0 cycles, 0 god-modules

$ git checkout 45bcba0
$ warpmap diff .
since baseline: edges +21, cycles +0, orphans +0, god-modules +0
7 files got more complex:
  +62  tests/hooks/useIsFieldRequired.test.tsx
  +46  src/form/BasicForm.tsx
  +7  src/validation/useFormConfig.ts
  +5  src/form/Form.tsx
  +2  src/field/index.ts
verdict: RISK UP - this change made the codebase harder to work on safely
```

nine files and 21 edges arrived, no cycle and no god-module did, and seven files got heavier -
the five biggest are shown, which is all `diff` prints, and two files that moved by the same
amount can trade places between runs. `diff` exited 1.

be clear about what that verdict counts: files that got more complex against files that got
simpler. it has no idea that the complexity bought a feature, so a branch that adds code will
usually read this way, and the list under the headline is the part to actually read. on an
existing codebase start the github action with `fail-on-risk: false`, which reports the same
delta on the pull request and leaves the job green.

## what it could not tell me

```
$ warpmap ownership . -n 3
hotspots by knowledge spread (bus-factor-1 = only one person has touched it):
  authors=1  src/controls/Control.tsx  <- bus factor 1
  authors=1  tests/validators/async.validators.test.ts  <- bus factor 1
  authors=1  tests/components/AsyncValidation.test.tsx  <- bus factor 1
```

every row, all the way down. one person wrote this repository, so the knowledge-risk command has
nothing to find; it is a question worth asking of a codebase with a team behind it and a waste
of a command here. reporting that honestly is cheaper than pretending the output means
something.

and the standing limits, none of which this run changes: no line coverage, no security scanning,
no type checking, nothing about whether the code is correct. warpmap maps structure and change.
every question above was a question about structure or change, which is why it could answer
them.
