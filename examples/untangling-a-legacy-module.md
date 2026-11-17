# untangling a legacy module

most codebases that have lived for a while have one: the file everybody edits and nobody wants
to, too big to read in a sitting, with every bug fix landing in it. before splitting a file like
that, four questions come up. is it really the one? how far does a cut reach? what catches a
mistake? and what has to move with it? this is warpmap answering those four on a public project,
with the numbers it returned.

the target is [react-hook-form](https://github.com/react-hook-form/react-hook-form) at commit
`003e4022`: 502 tracked files and 4,240 commits of history up to that commit. every warpmap block
below is output from warpmap at the changes listed under Unreleased in the
[changelog](../CHANGELOG.md), trimmed where it says so and otherwise unedited. nothing about the
project is assumed beyond what the commands print and what the quoted source lines say.

churn counts the commits of the last six months back from the day you run it, so the churn
columns and the hotspot scores drift with the calendar. the structural numbers - complexity,
blast radius, fan-in and fan-out - are fixed by the commit.

## get a full clone

```
$ git clone https://github.com/react-hook-form/react-hook-form
$ cd react-hook-form
$ git checkout 003e4022
```

churn comes out of `git log`, so a zip download or a `--depth 1` clone ranks on complexity alone.

## is it really the one

```
$ warpmap hotspots . -n 6
complexity: internal (cx = lines of code + 3 x branch keywords); lizard on PATH gives cyclomatic complexity instead
churn: 239 commits in the last 6 months
1.000  churn=94  cx=3368 src/logic/createFormControl.ts
0.039  churn=18  cx=694  src/useFieldArray.ts
0.018  churn=10  cx=563  src/types/form.ts
0.011  churn=7   cx=513  src/logic/validateField.ts
0.010  churn=10  cx=308  src/useController.ts
0.005  churn=7   cx=244  src/useWatch.ts
```

a hotspot list usually needs reading: three or four files near the top, and a judgement about
which pair of numbers matters. this one does not. `createFormControl.ts` was touched by 94 of the
239 commits in the window, five times the next file, and its complexity is almost five times the
next file's too. the second row scores 0.039 against its 1.000. widening the window to two years
(`--months 24`) does not change the picture: 174 of 552 commits, and the same file on top.

`hotspots --json` splits the 3,368:

```
{"file":"src/logic/createFormControl.ts","score":1,"churn":94,"complexity":3368,"loc":2138}
```

2,138 lines of code once blank lines and comments are left out, and (3,368 - 2,138) / 3 = 410
branch keywords. the file is 2,418 lines with two exports, `DEFAULT_FORM_STATE` and
`createFormControl`, and the function opens at line 143 and closes on the last line of the file.
nearly the whole module is one function body.

## how far a cut reaches

```
$ warpmap trace . src/logic/createFormControl.ts --depth 1
8 files depend on src/logic/createFormControl.ts (2 shipped, 6 tests)
  src/logic/index.ts
  src/useForm.ts
  tests:
    src/__tests__/logic/createFormControl.test.ts
    src/__tests__/logic/createFormControlDirtyFields.test.ts
    src/__tests__/performance.test.tsx
    src/__tests__/useForm/getErrors.test.tsx
    src/__tests__/useForm/handleSubmit.test.tsx
    src/__tests__/useForm/watch.test.tsx
```

```
$ warpmap trace . src/logic/createFormControl.ts
63 files depend on src/logic/createFormControl.ts (4 shipped, 59 tests)
  src/index.react-server.ts
  src/index.ts
  src/logic/index.ts
  src/useForm.ts
  tests:
    src/__tests__/controller.server.test.tsx
    ...
```

trimmed after the first test; the other 58 follow in path order.

63 dependents reads like a change that ripples through the whole library. 59 of them are tests,
5 of those compile-time type tests under `src/__typetest__/`. the shipped side is four files, and
they say more than the total does. two are the package's entry points, and both re-export the
logic barrel, which exports the function by name:

```
src/index.ts:7:export * from './logic';
src/index.react-server.ts:1:export * from './logic';
src/logic/index.ts:2:export { createFormControl } from './createFormControl';
```

so `createFormControl` is public api, not an internal detail of `useForm`. whatever the file
becomes, that export keeps its name and its shape. the fourth shipped dependent adds a second
constraint:

```
src/useForm.ts:3:import { DEFAULT_FORM_STATE } from './logic/createFormControl';
src/useForm.ts:9:import { createFormControl } from './logic';
```

`useForm` reaches past the barrel into the file itself for `DEFAULT_FORM_STATE`. moving that
constant out of the file changes this import too.

so the boundary of the cut is narrow and known: two named exports, read by the hook directly and
by the two entry points through the barrel. inside that boundary, the 2,138 lines can be reorganised freely, as long as those
two exports keep their names and shapes.

## what catches a mistake

```
$ warpmap risk . src/logic/createFormControl.ts
  src/logic/createFormControl.ts: blast=63, shipped=4, tested
combined blast radius: 63 files, 4 shipped
verdict: manageable
```

the verdict is `manageable` because a test imports the file, not because the blast radius is
small. `risk` only fails a change that is both wide and untested, and this file has six tests
that import it directly and 59 that reach it through some chain of imports. that is the safety
net for a split: those 59 files run against the public exports, so they keep checking the
behaviour while the inside moves.

read it as structure, not coverage. warpmap sees that a test imports the file; it does not see
which of the 410 branches the tests execute.

## what has to move with it

```
$ warpmap god . --all -n 4
files too many things depend on, or that depend on too much:
  in=91  out=11  src/types/index.ts
  in=55  out=10  src/useForm.ts
  in=29  out=30  src/useFieldArray.ts
  in=8   out=51  src/logic/createFormControl.ts
```

fan-in 8, fan-out 51. the 51 are the constants module, the types barrel, 27 helpers under
`src/utils/` and 22 sibling files in `src/logic/`. a lot of the logic already lives in other
files, and the function imports it from there.

the import graph does not show everything that moves with it. history does:

```
$ warpmap coupling .
coupling: 239 commits in the last 6 months, 1 of them not counted for changing more than 20 files
files that change together, 3 or more shared commits at a score of 0.30 or more (pairs an import joins left out):
  0.75  together=3/4   src/types/fieldArray.ts + src/useFieldArray.ts
  0.60  together=6/10  src/logic/createFormControl.ts + src/types/form.ts
  0.50  together=3/6   src/useFieldArray.ts + src/useForm.ts
  0.50  together=3/6   src/useFormState.ts + src/useWatch.ts
```

`src/types/form.ts` changed in 10 commits in the window, and 6 of them also changed
`createFormControl.ts`. no import joins the two directly: the module takes its types with
`import type { ... } from '../types'`, and `src/types/index.ts` re-exports `./form`. so the file
that declares the `Control` type changes with the module more often than not, and nothing in the
graph says so. a piece that leaves the module should take its part of `types/form.ts` with it,
or the next change will touch both files anyway.

## who knows it

```
$ warpmap ownership . -n 1
hotspots by knowledge spread (bus-factor-1 = only one person has touched it):
  authors=89 src/logic/createFormControl.ts
```

89 people have committed to this file over its history. on a team codebase, the dangerous
version of this file is the one only one person understands. that is not the problem here: the
file is hard to split because of its size, its public exports and the type file that moves with
it, not because the knowledge is in one person's head.

## what it could not tell me

warpmap works at the level of files. it said this file is the one, that four shipped files and
59 test files depend on it, and that `types/form.ts` moves with it; the source lines quoted above
turned the four shipped files into a boundary of two exports. it does not say where inside the 2,418 lines to make the first cut. that takes reading the
function, and the numbers above only tell you which constraints the cut has to respect. no line
coverage, no type checking, nothing about whether the code is correct.

## where that leaves the cut

- start here: one file holds 94 of the 239 commits in the window and complexity almost five times
  the next file's
- keep the boundary: `createFormControl` is public api through both entry points, and `useForm`
  imports `DEFAULT_FORM_STATE` from the file by path
- trust the net, within limits: 59 test files reach the module through its exports, which is a
  statement about imports, not about which branches run
- move the types with the code: `types/form.ts` shared 6 of its 10 commits with the module and
  has no direct import that would show it
