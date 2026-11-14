package graph

import "path/filepath"

// a language knows how to pull import specifiers out of a source file and resolve
// a specifier to a file on disk. adding a language = one map entry.
type language struct {
	extract func(src string) []Import
	resolve func(p *project, fromFile, spec string) string
}

func resolveTsIn(_ *project, fromFile, spec string) string { return resolveTs(fromFile, spec) }

func extractPyIn(src string) []Import {
	var imports []Import
	for _, s := range extractPy(src) {
		imports = append(imports, Import{Spec: s})
	}
	return imports
}

var languages = map[string]language{
	".ts":  {extractTs, resolveTsIn},
	".tsx": {extractTs, resolveTsIn},
	".js":  {extractTs, resolveTsIn},
	".jsx": {extractTs, resolveTsIn},
	".py":  {extractPyIn, resolvePy},
}

func langFor(path string) (language, bool) {
	l, ok := languages[filepath.Ext(path)]
	return l, ok
}
