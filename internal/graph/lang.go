package graph

import "path/filepath"

// a language knows how to pull import specifiers out of a source file and resolve
// a specifier to a file on disk. adding a language = one map entry.
type language struct {
	extract func(src string) []string
	resolve func(p *project, fromFile, spec string) string
}

func resolveTsIn(_ *project, fromFile, spec string) string { return resolveTs(fromFile, spec) }

var languages = map[string]language{
	".ts":  {extractTs, resolveTsIn},
	".tsx": {extractTs, resolveTsIn},
	".js":  {extractTs, resolveTsIn},
	".jsx": {extractTs, resolveTsIn},
	".py":  {extractPy, resolvePy},
}

func langFor(path string) (language, bool) {
	l, ok := languages[filepath.Ext(path)]
	return l, ok
}
