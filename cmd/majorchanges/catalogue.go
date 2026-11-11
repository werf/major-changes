package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"sigs.k8s.io/yaml"
)

const (
	configFileName = "config.yaml"
	entriesDirName = "entries"
	ledgerDirName  = "ledger"
	shortSHALength = 12
)

type catalogue struct {
	config  config
	entries map[string]*entry
	ledgers map[string][]*ledgerRow
}

type config struct {
	Products []product  `json:"products"`
	Ranges   []gitRange `json:"ranges"`
}

type product struct {
	Name   string `json:"name"`
	Output string `json:"output"`
	Title  string `json:"title"`
}

type gitRange struct {
	Name string `json:"name"`
	New  pin    `json:"new"`
	Old  []pin  `json:"old"`
	URL  string `json:"url"`
}

type pin struct {
	Commit string `json:"commit"`
	Ref    string `json:"ref"`
}

type ledgerRow struct {
	Commit  string   `json:"commit"`
	Date    string   `json:"date"`
	Entries []string `json:"entries,omitempty"`
	Note    string   `json:"note,omitempty"`
	Skip    string   `json:"skip,omitempty"`
	Subject string   `json:"subject"`
}

type entry struct {
	Areas       []string `json:"areas"`
	Breaking    *bool    `json:"breaking"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	ID          string   `json:"id"`
	Migration   string   `json:"migration,omitempty"`
	Products    []string `json:"products"`
	Refs        []string `json:"refs,omitempty"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`

	fileName string
}

func loadCatalogue(dataDir string) (*catalogue, error) {
	cat := &catalogue{
		entries: map[string]*entry{},
		ledgers: map[string][]*ledgerRow{},
	}

	if err := readYAML(filepath.Join(dataDir, configFileName), &cat.config); err != nil {
		return nil, err
	}

	for _, r := range cat.config.Ranges {
		var rows []*ledgerRow

		path := ledgerPath(dataDir, r.Name)
		if _, err := os.Stat(path); err == nil {
			if err := readYAML(path, &rows); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %q: %w", path, err)
		}

		cat.ledgers[r.Name] = rows
	}

	paths, err := filepath.Glob(filepath.Join(dataDir, entriesDirName, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob entries: %w", err)
	}

	for _, path := range paths {
		e := &entry{fileName: filepath.Base(path)}
		if err := readYAML(path, e); err != nil {
			return nil, err
		}

		if _, ok := cat.entries[e.ID]; ok {
			return nil, fmt.Errorf("entry id %q is used by more than one file, one of them %q", e.ID, path)
		}

		cat.entries[e.ID] = e
	}

	return cat, nil
}

// One row per line keeps the ledger greppable and its diffs readable, in the style prettier keeps as is.
// Quoted Go strings are valid YAML double-quoted scalars.
func writeLedgers(cat *catalogue, dataDir string) error {
	if err := os.MkdirAll(filepath.Join(dataDir, ledgerDirName), 0o755); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}

	for _, r := range cat.config.Ranges {
		var b strings.Builder

		fmt.Fprintf(&b, "# Every commit of %s reachable from %s and not from %s, oldest first.\n",
			r.Name, r.New.Ref, strings.Join(lo.Map(r.Old, func(p pin, _ int) string { return p.Ref }), ", "))
		b.WriteString("# Each row either maps the commit to catalogue entries or says why it is skipped. See ../README.md.\n")

		for _, row := range cat.ledgers[r.Name] {
			fmt.Fprintf(&b, "- { commit: %s, date: %s, subject: %s", row.Commit, row.Date, quoteYAML(row.Subject))

			if len(row.Entries) > 0 {
				fmt.Fprintf(&b, ", entries: [%s]", strings.Join(row.Entries, ", "))
			}

			if row.Skip != "" {
				fmt.Fprintf(&b, ", skip: %s", row.Skip)
			}

			if row.Note != "" {
				fmt.Fprintf(&b, ", note: %s", quoteYAML(row.Note))
			}

			b.WriteString(" }\n")
		}

		path := ledgerPath(dataDir, r.Name)
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			return fmt.Errorf("write %q: %w", path, err)
		}
	}

	return nil
}

func ledgerPath(dataDir, rangeName string) string {
	return filepath.Join(dataDir, ledgerDirName, rangeName+".yaml")
}

// quoteYAML picks the quotes prettier would: single quotes when that avoids escaping double quotes.
func quoteYAML(s string) string {
	if strings.Contains(s, `"`) && !strings.Contains(s, `'`) && strconv.CanBackquote(s) {
		return "'" + s + "'"
	}

	return strconv.Quote(s)
}

func readYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	if err := yaml.UnmarshalStrict(data, target); err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}

	return nil
}

func sortedEntries(cat *catalogue) []*entry {
	entries := lo.Values(cat.entries)
	slices.SortFunc(entries, func(a, b *entry) int { return strings.Compare(a.ID, b.ID) })

	return entries
}
