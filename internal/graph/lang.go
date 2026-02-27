package graph

import "path/filepath"

// a language knows how to pull import specifiers out of a source file and resolve
// a relative specifier to a file on disk. adding a language = one map entry.
type language struct {
	extract func(src string) []string
	resolve func(fromFile, spec string) string
}

var languages = map[string]language{
	".ts":  {extractTs, resolveTs},
	".tsx": {extractTs, resolveTs},
	".js":  {extractTs, resolveTs},
	".jsx": {extractTs, resolveTs},
	".py":  {extractPy, resolvePy},
}

func langFor(path string) (language, bool) {
	l, ok := languages[filepath.Ext(path)]
	return l, ok
}
