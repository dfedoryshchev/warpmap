package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// a cheap structural proxy: lines of code plus branching keywords, comments left out.
// not cyclomatic complexity, but good enough to rank files against each other.
var branchRe = regexp.MustCompile("\\b(if|for|while|case|catch)\\b|&&|\\|\\||\\?\\?")

type Complexity struct {
	LOC      int
	Branches int
	Score    int
}

func FileComplexity(path string) (Complexity, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Complexity{}, err
	}
	code := string(stripComments(src, filepath.Ext(path)))
	loc := 0
	for _, line := range strings.Split(code, "\n") {
		if strings.TrimSpace(line) != "" {
			loc++
		}
	}
	branches := len(branchRe.FindAllString(code, -1))
	return Complexity{LOC: loc, Branches: branches, Score: loc + branches*3}, nil
}

func stripComments(src []byte, ext string) []byte {
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx":
		return stripJs(src)
	case ".py":
		return stripPy(src)
	}
	return src
}

func blankOut(out []byte, from, to int) {
	for k := from; k < to; k++ {
		if out[k] != '\n' {
			out[k] = ' '
		}
	}
}

func lineEnd(src []byte, i int) int {
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

// skipQuoted returns the index just past the string opened at i. A quote that
// cannot span lines stops at the newline, so a stray apostrophe in JSX text only
// costs the rest of its own line.
func skipQuoted(src []byte, i int, q byte, oneLine bool) int {
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == '\\':
			j++
		case src[j] == q:
			return j + 1
		case src[j] == '\n' && oneLine:
			return j
		}
	}
	return len(src)
}

func stripJs(src []byte) []byte {
	out := append([]byte(nil), src...)
	var prev byte
	for i := 0; i < len(src); {
		c := src[i]
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}
		switch {
		case c == '/' && next == '/':
			j := lineEnd(src, i)
			blankOut(out, i, j)
			i = j
		case c == '/' && next == '*':
			j := len(src)
			if k := strings.Index(string(src[i+2:]), "*/"); k >= 0 {
				j = i + 2 + k + 2
			}
			blankOut(out, i, j)
			i = j
		case c == '\'' || c == '"':
			i = skipQuoted(src, i, c, true)
			prev = c
		case c == '`':
			i = skipQuoted(src, i, c, false)
			prev = c
		case c == '/' && regexCanStart(src, i, prev):
			i = skipRegex(src, i)
			prev = '/'
		default:
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
				prev = c
			}
			i++
		}
	}
	return out
}

var regexAfterWord = map[string]bool{
	"return": true, "typeof": true, "case": true, "throw": true, "in": true,
	"of": true, "delete": true, "void": true, "yield": true, "await": true, "else": true,
}

// a slash after a value is division; anywhere an expression can start it opens a
// regex literal, whose quotes and slashes must not be read as strings or comments.
func regexCanStart(src []byte, i int, prev byte) bool {
	if prev == 0 || strings.IndexByte("(,=:[!&|?{};+-*%<>~^", prev) >= 0 {
		return true
	}
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	end := j + 1
	for j >= 0 && (src[j] >= 'a' && src[j] <= 'z') {
		j--
	}
	return regexAfterWord[string(src[j+1:end])]
}

func skipRegex(src []byte, i int) int {
	inClass := false
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '\n':
			return i + 1
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				return j + 1
			}
		}
	}
	return i + 1
}

// stripPy drops # comments and docstrings: a triple-quoted string standing alone as
// a statement documents the code rather than being part of it.
func stripPy(src []byte) []byte {
	out := append([]byte(nil), src...)
	depth := 0
	first := -1
	for i := 0; i < len(src); {
		c := src[i]
		switch c {
		case '#':
			j := lineEnd(src, i)
			blankOut(out, i, j)
			i = j
			continue
		case '\'', '"':
			if first < 0 {
				first = i
			}
			if i+2 < len(src) && src[i+1] == c && src[i+2] == c {
				j := skipTriple(src, i, c)
				if depth == 0 && strings.Trim(string(src[first:i]), "rRuU") == "" && restIsBlank(src, j) {
					blankOut(out, first, j)
				}
				i = j
			} else {
				i = skipQuoted(src, i, c, true)
			}
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '\n':
			if depth == 0 && (i == 0 || src[i-1] != '\\') {
				first = -1
			}
		}
		if first < 0 && c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			first = i
		}
		i++
	}
	return out
}

func skipTriple(src []byte, i int, q byte) int {
	for j := i + 3; j < len(src); j++ {
		if src[j] == '\\' {
			j++
			continue
		}
		if src[j] == q && j+2 < len(src) && src[j+1] == q && src[j+2] == q {
			return j + 3
		}
	}
	return len(src)
}

func restIsBlank(src []byte, j int) bool {
	for ; j < len(src) && src[j] != '\n'; j++ {
		switch src[j] {
		case ' ', '\t', '\r', ';':
		case '#':
			return true
		default:
			return false
		}
	}
	return true
}
