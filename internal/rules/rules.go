// Package rules loads the YAML knowledge base that both the CLI and the future
// Android app share: protected packages, bloatware, leftovers, cleanable paths.
package rules

import (
	"embed"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var files embed.FS

// Bloat is one preinstalled app that can be removed for the current user.
type Bloat struct {
	Package string `yaml:"package" json:"package"`
	Vendor  string `yaml:"vendor" json:"vendor"`
	Risk    string `yaml:"risk" json:"risk"`
	Note    string `yaml:"note" json:"note"`
}

// Leftover maps a shared-storage folder to the package that owns it.
type Leftover struct {
	Path    string `yaml:"path" json:"path"`
	Package string `yaml:"package" json:"package"`
}

// Cleanable is one disposable target.
type Cleanable struct {
	Name         string `yaml:"name" json:"name"`
	Glob         string `yaml:"glob" json:"glob"`
	Kind         string `yaml:"kind" json:"kind"`
	MinAgeDays   int    `yaml:"min_age_days" json:"min_age_days"`
	RunningGuard bool   `yaml:"running_guard" json:"running_guard"`
	Note         string `yaml:"note,omitempty" json:"note,omitempty"`
}

type nameList struct {
	Exact    []string `yaml:"exact"`
	Prefixes []string `yaml:"prefixes"`
}

func (l nameList) matches(pkg string) bool {
	for _, e := range l.Exact {
		if pkg == e {
			return true
		}
	}
	for _, p := range l.Prefixes {
		if strings.HasPrefix(pkg, p) {
			return true
		}
	}
	return false
}

// DB is the loaded knowledge base.
type DB struct {
	protected nameList
	dataProt  nameList
	Bloat     []Bloat
	Leftovers []Leftover
	Cleanable []Cleanable
	bloatIdx  map[string]Bloat
	names     map[string]string
}

var (
	once   sync.Once
	loaded *DB
	loadEr error
)

// Load parses the embedded YAML once.
func Load() (*DB, error) {
	once.Do(func() {
		db := &DB{bloatIdx: map[string]Bloat{}}
		loadEr = firstErr(
			read("data/protected_packages.yaml", &db.protected),
			read("data/data_protected.yaml", &db.dataProt),
			read("data/bloatware.yaml", &db.Bloat),
			read("data/leftovers.yaml", &db.Leftovers),
			read("data/cleanable.yaml", &db.Cleanable),
			read("data/app_names.yaml", &db.names),
		)
		for _, b := range db.Bloat {
			db.bloatIdx[b.Package] = b
		}
		loaded = db
	})
	return loaded, loadEr
}

// MustLoad panics on a broken embedded file, which is a build defect.
func MustLoad() *DB {
	db, err := Load()
	if err != nil {
		panic(err)
	}
	return db
}

func read(name string, into any) error {
	b, err := files.ReadFile(name)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, into); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// IsProtected reports whether the package must never be uninstalled or disabled.
func (db *DB) IsProtected(pkg string) bool { return db.protected.matches(pkg) }

// IsDataProtected reports whether the package's shared-storage folders hold personal data.
func (db *DB) IsDataProtected(pkg string) bool { return db.dataProt.matches(pkg) }

// NameFor returns the display name of a package, or "" when it is not in app_names.yaml.
func (db *DB) NameFor(pkg string) string { return db.names[pkg] }

// Label is "Name (package)" when the name is known, else the package id.
func (db *DB) Label(pkg string) string {
	if n := db.names[pkg]; n != "" {
		return n + " (" + pkg + ")"
	}
	return pkg
}

// BloatFor returns the bloatware entry for a package, if known.
func (db *DB) BloatFor(pkg string) (Bloat, bool) {
	b, ok := db.bloatIdx[pkg]
	return b, ok
}

// Validate checks the data files for contradictions; run by tests and `sapu rules check`.
func (db *DB) Validate() []string {
	var problems []string
	for _, b := range db.Bloat {
		if db.IsProtected(b.Package) {
			problems = append(problems, "bloatware entry is also protected: "+b.Package)
		}
		if b.Risk != "safe" && b.Risk != "medium" {
			problems = append(problems, "bloatware risk must be safe|medium: "+b.Package)
		}
	}
	for _, l := range db.Leftovers {
		if strings.HasPrefix(l.Path, "/") || strings.Contains(l.Path, "..") || l.Path == "" {
			problems = append(problems, "leftover path must be relative and clean: "+l.Path)
		}
	}
	for _, c := range db.Cleanable {
		if c.Kind != "dir" && c.Kind != "file" {
			problems = append(problems, "cleanable kind must be dir|file: "+c.Glob)
		}
		if strings.HasPrefix(c.Glob, "/") || strings.Contains(c.Glob, "..") || c.Glob == "" || c.Glob == "*" {
			problems = append(problems, "cleanable glob must be relative and specific: "+c.Glob)
		}
	}
	return problems
}
