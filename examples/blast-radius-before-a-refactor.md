# blast radius before a refactor

a refactor is supposed to change how code is arranged and nothing else, and the usual way to
check that is to ask what depends on the code being moved. `trace` and `risk` answer that from
the import graph. this is both commands run on two merged refactor pull requests in a public
project, at the commit each one was branched from, and then held up against what the pull
request actually changed. one prediction holds; the other misses a file and raises a false alarm.

the target is [react-hook-form](https://github.com/react-hook-form/react-hook-form):

- [#13689](https://github.com/react-hook-form/react-hook-form/pull/13689), "remove unused
  abortEarly param from iterateFieldsByAction", merged 2026-08-30 23:05 UTC as `c3987dde`,
  parent `41bdde40`
- [#13615](https://github.com/react-hook-form/react-hook-form/pull/13615), "extract shared
  ErrorNamespacePath", merged 2026-07-25 09:45 UTC as `d349d082`, parent `e47b0902`

both were squash-merged, so each parent is the tree the change was reviewed against. every
warpmap block below is output from warpmap at the changes listed under Unreleased in the
[changelog](../CHANGELOG.md), trimmed where it says so and otherwise unedited. `trace` and `risk`
read only the import graph, which is fixed by the commit, so unlike churn these numbers do not
drift with the date you run them.

## get the commit before the change

```
$ git clone https://github.com/react-hook-form/react-hook-form
$ cd react-hook-form
$ git checkout 41bdde40
```

neither command reads history, so any clone that can check out the parent will do.

## a parameter leaves a function

#13689 drops the fourth parameter of `iterateFieldsByAction`. removing a parameter is a change
to the function's signature, so every place that calls it is a place that might pass it.

```
$ warpmap trace . src/logic/iterateFieldsByAction.ts --depth 1
3 files depend on src/logic/iterateFieldsByAction.ts (2 shipped, 1 test)
  src/logic/createFormControl.ts
  src/useFieldArray.ts
  tests:
    src/__tests__/logic/iterateFieldsByAction.test.ts
```

```
$ warpmap risk . src/logic/iterateFieldsByAction.ts
  src/logic/iterateFieldsByAction.ts: blast=62, shipped=6, tested
combined blast radius: 62 files, 6 shipped
verdict: manageable
```

before the change, that is three files to read. reading them at `41bdde40` gives ten calls
outside the function itself: three in `createFormControl.ts`, one in `useFieldArray.ts` and six
in the unit test. one passes a fourth argument, a literal `false` in `_disableForm` in
`createFormControl.ts`; the other nine pass two or three.

## what the change touched

```
$ git show --stat --format= c3987dde
 src/logic/createFormControl.ts     | 1 -
 src/logic/iterateFieldsByAction.ts | 5 ++---
 2 files changed, 2 insertions(+), 4 deletions(-)
```

the function itself and the one caller that passed the argument. the other two direct
dependents needed no edit, because they never passed it, and no test changed. nothing outside
the three-file list was touched, and the one caller that needed an edit was on it.

the full count is the one to read against the test run, not against the diff. 62 files reach
the function through some chain of imports, 56 of them tests, and `tested` means one of those
tests imports the file directly. the diff was six lines in two files; the reach is what runs it.

follow-up fixes are the other check. five later commits touched `iterateFieldsByAction.ts` up to
`8d7bd6c2`, and none put the parameter back. two came within ten days and were fixes, and both are mine, so they are not an independent read.
[#13705](https://github.com/react-hook-form/react-hook-form/pull/13705), six days later, edited
one of the two lines this change edited, but a different part of it: which name the callback is
handed for a checkbox or radio group, `key` before and after this change.
[#13718](https://github.com/react-hook-form/react-hook-form/pull/13718), nine days later,
changed how the recursive branches leave the loop, lines this change did not touch. neither
undoes or depends on the removed parameter.

so for a signature change the depth-1 list was the review list: two shipped callers and one test,
of which one needed the edit.

## a type is extracted

#13615 pulls a four-member union that two exported types in `src/types/form.ts` spelled out by
hand into one local type, `ErrorNamespacePath`. same command, at its parent:

```
$ git checkout e47b0902
$ warpmap trace . src/types/form.ts --depth 1
6 files depend on src/types/form.ts (6 shipped, 0 tests)
  src/types/fieldArray.ts
  src/types/index.ts
  src/types/resolvers.ts
  src/types/utils.ts
  src/types/validator.ts
  src/types/watch.ts
```

```
$ warpmap trace . src/types/form.ts
169 files depend on src/types/form.ts (76 shipped, 93 tests)
  app/src/app.tsx
  app/src/crossFrameForm.tsx
  app/src/main.tsx
  app/src/setError.tsx
  ...
```

trimmed after the fourth line; the rest follow in path order. the 76 shipped are 69 files under
`src/`, 5 under `app/`, the project's demo app, and 2 bench scripts under `scripts/`. warpmap
counts anything that is not a test as shipped, so the demo app and the scripts are in that
number.

```
$ warpmap risk . src/types/form.ts
  src/types/form.ts: blast=169, shipped=76, UNTESTED
combined blast radius: 169 files, 76 shipped
verdict: 1 changed file(s) are high-blast AND untested - add tests before changing
```

and it exits 1.

## what the change touched

```
$ git show --stat --format= d349d082
 reports/api-extractor.md.api.md | 10 ++++++----
 src/types/form.ts               | 14 ++++----------
 2 files changed, 10 insertions(+), 14 deletions(-)
```

none of the six direct dependents changed, and that one is right: the new type is not exported,
and the two exported types keep the same union, so nothing that imports them sees a difference.

the second file is the miss. `reports/api-extractor.md.api.md` is the committed API report, which
its own header says is generated by API Extractor and not to be edited, and a workflow that runs
on every pull request fails when the committed report does not match the exported API. so a
change to an exported type has to regenerate it and commit it. it is markdown, not source, so
it is not in the graph, and nothing `trace` printed points at it. history does: from the commit
that added the report (`03f9d727`) up to `e47b0902`, 39 commits changed `src/types/form.ts`, and
33 of them changed the report too.

```
$ git log --format=%h 03f9d727^..e47b0902 -- src/types/form.ts
```

lists the 39; the same command on the report file lists 68, and 33 hashes appear in both.

the verdict is the false alarm. `UNTESTED` means no test file imports `form.ts` directly. that
is true, and for a file of types it is the wrong question: 93 test files reach it through the
other type files and the `src/types/index.ts` barrel, 9 of them type tests under
`src/__typetest__/`, and the compiler checks a
type change across all of it. the pull request added no test and merged. a CI gate on `risk`
would have failed it.

## what it could not tell me

the graph sees imports between source files. it does not see a generated report that a type
change rewrites, a snapshot, a fixture, or a config file read at run time, and on #13615 the
only file the change touched besides its target was of that kind. it says which files depend
on the changed one, not which of them have to change: on #13689 that took reading three files
and finding one call that passed the argument. and `tested` and `UNTESTED` are statements about
direct imports from test files, not about whether any test runs the changed lines.

## where that leaves it

- a signature change: `trace --depth 1` is the list of files to read, and on #13689 the one edit
  outside the target was in it, with nothing outside it touched
- the full blast radius is what a change can break and what the test run should cover, not a
  forecast of the diff: 62 and 169 files reached, 2 files changed each time
- a type change: the direct dependents can all stay untouched when the exported shape does not
  move, and `risk` calls a pure type file `UNTESTED` because no test imports it by path, so read
  that verdict on a types file as structure, not as a missing test
- files outside the graph move too: the API report changed with `form.ts` in 33 of 39 commits,
  and neither command can show that
