package metrics

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/dfedoryshchev/warpmap/internal/gitcmd"
)

// Owners returns the distinct authors who have touched a file, most commits first.
// a file only one person has ever touched is a bus-factor-1 knowledge risk.
// git is asked from the file's own directory, so the answer comes from the repo
// that holds the file - a nested repo or a submodule included, whose history the
// project's top repo does not have - and the pathspec is just the file's name,
// whatever the caller's working directory.
//
// one person is one author however many spellings they committed under: %aN and
// %aE apply the repo's .mailmap, then commits that share an email or a name
// (ignoring case) are the same person, named by the spelling they used most.
func Owners(file string) []string {
	owners := Ownership(file)
	names := make([]string, len(owners))
	for i, o := range owners {
		names[i] = o.Name
	}
	return names
}

// Owner is one person and how many commits they made.
type Owner struct {
	Name    string
	Commits int
}

// Ownership is Owners with each person's commit count.
func Ownership(file string) []Owner {
	out, err := gitcmd.Command(filepath.Dir(file), "log", "--no-ext-diff", "--no-textconv", "--format=%aN%x00%aE", "--", filepath.Base(file)).Output()
	if err != nil {
		return nil
	}
	var ids []identity
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, email, _ := strings.Cut(line, "\x00")
		ids = append(ids, identity{name, email})
	}
	return people(ids)
}

// Authors is the people behind commits, most commits first, one person counted
// once the way Owners counts them.
func Authors(commits []Commit) []Owner {
	ids := make([]identity, len(commits))
	for i, c := range commits {
		ids[i] = identity{c.Name, c.Email}
	}
	return people(ids)
}

// Held is a file one person made more than half the commits to.
type Held struct {
	File    string
	Author  string
	Commits int
	Total   int
}

// HeldByOne keeps, in the order given, the files whose leading author made more
// than half of their commits across the file's whole history.
func HeldByOne(files []string) []Held {
	var held []Held
	for _, f := range files {
		owners := Ownership(f)
		if len(owners) == 0 {
			continue
		}
		total := 0
		for _, o := range owners {
			total += o.Commits
		}
		if lead := owners[0]; lead.Commits*2 > total {
			held = append(held, Held{File: f, Author: lead.Name, Commits: lead.Commits, Total: total})
		}
	}
	return held
}

type identity struct{ name, email string }

func people(ids []identity) []Owner {
	parent := map[string]string{}
	find := func(k string) string {
		for parent[k] != k {
			parent[k] = parent[parent[k]]
			k = parent[k]
		}
		return k
	}
	join := func(a, b string) {
		for _, k := range []string{a, b} {
			if _, ok := parent[k]; !ok {
				parent[k] = k
			}
		}
		parent[find(a)] = find(b)
	}

	type commit struct{ name, key string }
	var commits []commit
	for _, id := range ids {
		name, email := strings.TrimSpace(id.name), strings.ToLower(strings.TrimSpace(id.email))
		if name == "" && email == "" {
			continue
		}
		nameKey, emailKey := "n:"+strings.ToLower(name), "e:"+email
		switch {
		case name == "":
			name, nameKey = email, emailKey
		case email == "":
			emailKey = nameKey
		}
		join(nameKey, emailKey)
		commits = append(commits, commit{name, nameKey})
	}

	counts := map[string]int{}
	spellings := map[string]map[string]int{}
	var order []string
	for _, c := range commits {
		p := find(c.key)
		if counts[p] == 0 {
			order = append(order, p)
			spellings[p] = map[string]int{}
		}
		counts[p]++
		spellings[p][c.name]++
	}
	names := map[string]string{}
	for _, c := range commits {
		p := find(c.key)
		if cur, ok := names[p]; !ok || spellings[p][c.name] > spellings[p][cur] {
			names[p] = c.name
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	owners := make([]Owner, len(order))
	for i, p := range order {
		owners[i] = Owner{Name: names[p], Commits: counts[p]}
	}
	return owners
}
