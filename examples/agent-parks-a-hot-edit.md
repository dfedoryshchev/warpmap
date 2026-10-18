# an agent that parks a hot edit for a person

an agent asked to change code it did not write has two easy answers when the change looks
dangerous, and both are wrong. it can make the edit anyway, or it can refuse and leave nothing
behind. the third answer is the useful one: stop before the edit, and hand the change to a person
together with the reason it stopped. this is that loop, with `warpmap risk` as the reason.

the target is the same one as [the worked audit](worked-audit.md):
[react-formkit](https://github.com/dfedoryshchev/react-formkit) at commit `45bcba0`. every
warpmap block below is output from `warpmap 0.2.0-dev` against that commit, unedited. none of
these numbers depend on history - `risk` and `trace` read the import graph and which files a test
imports - so a zip of that commit returns the same blocks as a clone.

## the check, before the edit

the agent is asked to make the radio group's options keyboard-navigable. its plan names the files
it will touch, and before it touches them it asks:

```
$ warpmap risk . src/controls/toggles/RadioInput.tsx src/controls/toggles/RadioGroup.tsx
  src/controls/toggles/RadioInput.tsx: blast=25, UNTESTED
  src/controls/toggles/RadioGroup.tsx: blast=24, UNTESTED
combined blast radius: 25 files
verdict: 2 changed file(s) are high-blast AND untested - add tests before changing
```

`risk` exited 1. both files have more dependents than `thresholds.blast` in `warpmap.json` allows
(10 when the file says nothing), and no test imports either of them. that pairing is the whole
rule: a wide file with a test passes, and so does an untested leaf.

```
$ warpmap risk . src/validation/validators/charset.validators.ts
  src/validation/validators/charset.validators.ts: blast=4, tested
combined blast radius: 4 files
verdict: manageable
```

exit 0. this one the agent edits without asking anybody.

## three exits, three answers

the gate is the exit code, and it has three values, not two:

- `0` - manageable. the edit goes ahead.
- `1` - risky. the run parks: the edit is not made, and `risk`'s own output goes to a person as
  the reason.
- `2` - warpmap could not assess the change, usually because a path is not in the graph. that is
  not a pass either, so it parks too.

the third case is the one an agent gets wrong by being helpful. a stylesheet is not something
warpmap parses, so it has no blast radius to report:

```
$ warpmap risk . src/controls/toggles/RadioInput.scss
src/controls/toggles/RadioInput.scss is not in the graph of .
```

on stderr, exit 2. a wrapper that treats "not 1" as safe would wave that through; one that treats
"not 0" as a stop does not. as a shell function, with warpmap on `PATH`:

```sh
preflight() {
  verdict=$(warpmap risk . "$@" 2>&1)
  case $? in
    0) return 0 ;;
    1) printf 'handed to a person:\n%s\n' "$verdict" ;;
    *) printf 'handed to a person, warpmap could not assess it:\n%s\n' "$verdict" ;;
  esac
  return 1
}
```

```
$ preflight src/controls/toggles/RadioInput.tsx src/controls/toggles/RadioGroup.tsx && echo "edit goes ahead"
handed to a person:
  src/controls/toggles/RadioInput.tsx: blast=25, UNTESTED
  src/controls/toggles/RadioGroup.tsx: blast=24, UNTESTED
combined blast radius: 25 files
verdict: 2 changed file(s) are high-blast AND untested - add tests before changing
```

what the person receives is warpmap's text, not the agent's summary of it. the numbers are the
reason, and they can be re-run.

## the same check over mcp

an agent holding a `warpmap mcp` session open, started in the project root and past the
`initialize` handshake the readme shows, asks the same question as a tool call:

```
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"risk","arguments":{"dir":".","files":["src/controls/toggles/RadioInput.tsx","src/controls/toggles/RadioGroup.tsx"]}}}
{"id":2,"jsonrpc":"2.0","result":{"content":[{"text":"  src/controls/toggles/RadioInput.tsx: blast=25, UNTESTED\n  src/controls/toggles/RadioGroup.tsx: blast=24, UNTESTED\ncombined blast radius: 25 files\nverdict: 2 changed file(s) are high-blast AND untested - add tests before changing\n","type":"text"}]}}
```

there is no exit code here, and the result is not marked as an error: a risky change is an answer,
not a failure. the verdict line is the signal, so a client parks on any verdict other than
`verdict: manageable`. a path the graph does not hold does come back with `isError` set, which
maps to the exit-2 case above:

```
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"risk","arguments":{"dir":".","files":["src/controls/toggles/RadioInput.scss"]}}}
{"id":4,"jsonrpc":"2.0","result":{"content":[{"text":"src/controls/toggles/RadioInput.scss is not in the graph","type":"text"}],"isError":true}}
```

## what the person does with it

the change it hands over carries enough to decide without re-deriving anything. the person can look at
what the first file actually drags with it:

```
$ warpmap trace . src/controls/toggles/RadioInput.tsx --depth 1
2 files depend on src/controls/toggles/RadioInput.tsx
  src/controls/toggles/RadioGroup.tsx
  src/controls/toggles/index.ts
```

two direct importers, and twenty-five once the imports are followed all the way out. then one of
three things happens. the person makes the edit, since a person who owns the blast radius is what
the park was for. or they write the test first and let the agent carry on, and `risk` says
`tested` for each file a test imports directly. or they shrink the change to files that pass.
the decision stays with the person.

## what it does not know

`tested` means a test file imports the file, not that a test would fail if the edit broke it.
and the rule is wide AND untested, so a wide file with a test passes:

```
$ warpmap risk . src/controls/Control.tsx
  src/controls/Control.tsx: blast=22, tested
combined blast radius: 22 files
verdict: manageable
```

that is the file the worked audit ranks first, and the verdict lets it through. an agent that should
stop on every wide file reads the `blast=` numbers rather than the verdict line.

any runner that can run a command, or speaks mcp, can sit in front of an edit this way. the one i
use is [agentweft](https://github.com/dfedoryshchev/agentweft); nothing above depends on it.
