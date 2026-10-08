package graph

import (
	"path/filepath"
	"strings"
	"testing"
)

// pyProject writes files (slash paths relative to a temp root) and returns the
// root plus the walked file list Build would receive.
func pyProject(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var list []string
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		writeFile(t, p, src)
		list = append(list, p)
	}
	return root, list
}

func edgeSet(root string, g Graph) map[string]bool {
	out := map[string]bool{}
	for _, e := range g.Edges {
		from, _ := filepath.Rel(root, e.From)
		to, _ := filepath.Rel(root, e.To)
		out[filepath.ToSlash(from)+" -> "+filepath.ToSlash(to)] = true
	}
	return out
}

func TestExtractPyReadsEveryImportForm(t *testing.T) {
	src := "import a.b as c, d\n" +
		"    import e  # indented\n" +
		"from f import *\n" +
		"from g.h import i as j, k\n" +
		"from l import m, \\\n    n\n" +
		"from . import o\n" +
		"from ..p import q\n" +
		"x = 1; import r\n"
	got := strings.Join(extractPy(src), " ")
	want := "f g.h\x00i g.h\x00k l\x00m l\x00n .\x00o ..p\x00q a.b d e"
	if got != want {
		t.Fatalf("extractPy =\n%q\nwant\n%q", got, want)
	}
}

func TestPythonAbsoluteImportsResolveInsideTheProject(t *testing.T) {
	root, files := pyProject(t, map[string]string{
		"app/shop/__init__.py": "from shop import cart\n",
		"app/shop/cart.py": "import os, json\n" +
			"from requests import get\n" +
			"from shop.pricing import total\n" +
			"import shop.tax as tax\n",
		"app/shop/pricing.py":  "from shop import tax, discount\n",
		"app/shop/discount.py": "from shop.cart import (\n    Cart,  # the basket\n    Line,\n)\n",
		"app/shop/tax.py":      "from json import loads\nimport shop\n",
		"app/shop/json.py":     "",
		"app/shop/promo.py":    "from . import tax\n",
	})
	es := edgeSet(root, Build(files))

	for _, want := range []string{
		"app/shop/__init__.py -> app/shop/cart.py",
		"app/shop/cart.py -> app/shop/pricing.py",
		"app/shop/cart.py -> app/shop/tax.py",
		"app/shop/pricing.py -> app/shop/tax.py",
		"app/shop/pricing.py -> app/shop/discount.py",
		"app/shop/discount.py -> app/shop/cart.py",
		"app/shop/tax.py -> app/shop/__init__.py",
		"app/shop/promo.py -> app/shop/tax.py",
	} {
		if !es[want] {
			t.Errorf("missing edge %s; got %v", want, es)
		}
	}
	for _, bad := range []string{
		"app/shop/tax.py -> app/shop/json.py",
		"app/shop/cart.py -> app/shop/json.py",
		"app/shop/pricing.py -> app/shop/__init__.py",
		"app/shop/promo.py -> app/shop/__init__.py",
	} {
		if es[bad] {
			t.Errorf("unexpected edge %s", bad)
		}
	}
}

func TestPythonAbsoluteImportsInASrcLayout(t *testing.T) {
	root, files := pyProject(t, map[string]string{
		"src/ledger/__init__.py":        "",
		"src/ledger/books/__init__.py":  "",
		"src/ledger/books/journal.py":   "import ledger.books.entry\n",
		"src/ledger/books/entry.py":     "",
		"tests/test_journal.py":         "from ledger.books.journal import post\n",
		"tests/conftest.py":             "",
		"src/ledger/books/unrelated.py": "import entry\n",
	})
	es := edgeSet(root, Build(files))

	for _, want := range []string{
		"src/ledger/books/journal.py -> src/ledger/books/entry.py",
		"tests/test_journal.py -> src/ledger/books/journal.py",
	} {
		if !es[want] {
			t.Errorf("missing edge %s; got %v", want, es)
		}
	}
	if es["src/ledger/books/unrelated.py -> src/ledger/books/entry.py"] {
		t.Error("a bare `import entry` inside a package resolved against the package's own directory, which python 3 never does")
	}
}

func TestPythonScriptImportsASiblingModule(t *testing.T) {
	root, files := pyProject(t, map[string]string{
		"tools/run.py":    "import helper\nimport yaml\n",
		"tools/helper.py": "",
	})
	es := edgeSet(root, Build(files))
	if !es["tools/run.py -> tools/helper.py"] {
		t.Errorf("a script's sibling import did not resolve; got %v", es)
	}
	if len(es) != 1 {
		t.Errorf("want exactly one edge, got %v", es)
	}
}

func TestPythonNamespacePackageUnderTheProjectRoot(t *testing.T) {
	root, files := pyProject(t, map[string]string{
		"plugins/csv_out.py": "",
		"core/main.py":       "from plugins.csv_out import write\n",
		"core/other.py":      "",
	})
	es := edgeSet(root, Build(files))
	if !es["core/main.py -> plugins/csv_out.py"] {
		t.Errorf("a namespace package (no __init__.py) did not resolve; got %v", es)
	}
}
