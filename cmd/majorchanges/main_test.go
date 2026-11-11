package main

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeLedgerKeepsClassificationAndAppendsNewCommits(t *testing.T) {
	rows := []*ledgerRow{{Commit: "aaa", Subject: "old subject", Entries: []string{"some-entry"}}}
	commits := []gitCommit{
		{sha: "aaa", date: "2026-01-01", subject: "feat: a"},
		{sha: "bbb", date: "2026-01-02", subject: "chore(main): release 2.0.0"},
		{sha: "ccc", date: "2026-01-03", subject: "fix: c"},
		{sha: "ddd", date: "2026-01-04", subject: "fix: d"},
		{sha: "fff", date: "2026-01-05", subject: "Merge branch 'main' into 2", parents: 2},
		{sha: "ggg", date: "2026-01-06", subject: "Merge branch 'main' into 2", parents: 2},
		{sha: "hhh", date: "2026-01-07", subject: "chore(main): release 2.1.0", parents: 2},
	}

	merged, added, err := mergeLedger(rows, commits, map[string]string{"ccc": "same subject as eee in v1"},
		map[string]bool{"fff": true, "ggg": false, "hhh": false})
	require.NoError(t, err)

	assert.Equal(t, 6, added)
	assert.Equal(t, []*ledgerRow{
		{Commit: "aaa", Date: "2026-01-01", Subject: "feat: a", Entries: []string{"some-entry"}},
		{Commit: "bbb", Date: "2026-01-02", Subject: "chore(main): release 2.0.0", Skip: skipRelease},
		{Commit: "ccc", Date: "2026-01-03", Subject: "fix: c", Skip: skipInOld, Note: "same subject as eee in v1"},
		{Commit: "ddd", Date: "2026-01-04", Subject: "fix: d"},
		{Commit: "fff", Date: "2026-01-05", Subject: "Merge branch 'main' into 2", Skip: skipMerge},
		{Commit: "ggg", Date: "2026-01-06", Subject: "Merge branch 'main' into 2"},
		{Commit: "hhh", Date: "2026-01-07", Subject: "chore(main): release 2.1.0"},
	}, merged)
}

func TestMergeLedgerRejectsCommitsOutsideRange(t *testing.T) {
	rows := []*ledgerRow{{Commit: "zzz", Skip: skipTest}}

	_, _, err := mergeLedger(rows, []gitCommit{{sha: "aaa"}}, nil, nil)
	require.ErrorContains(t, err, "zzz")
}

func TestQuoteYAMLMatchesPrettier(t *testing.T) {
	assert.Equal(t, `"fix: a"`, quoteYAML("fix: a"))
	assert.Equal(t, `'Revert "feat: a"'`, quoteYAML(`Revert "feat: a"`))
	assert.Equal(t, `"it's \"a\""`, quoteYAML(`it's "a"`))
}

func TestValidateAccountsForEveryCommitAndEntry(t *testing.T) {
	breakingEntry := validEntry("breaking-entry")
	breakingEntry.Breaking = lo.ToPtr(true)

	cat := &catalogue{
		config: config{
			Products: []product{{Name: "nelm"}},
			Ranges:   []gitRange{{Name: "nelm"}},
		},
		ledgers: map[string][]*ledgerRow{"nelm": {
			{Commit: "aaa", Entries: []string{"good-entry"}},
			{Commit: "bbb"},
			{Commit: "ccc", Entries: []string{"missing-entry"}},
			{Commit: "ddd", Skip: "whatever"},
			{Commit: "eee", Entries: []string{"breaking-entry"}},
		}},
		entries: map[string]*entry{
			"good-entry":     validEntry("good-entry"),
			"orphan-entry":   validEntry("orphan-entry"),
			"breaking-entry": breakingEntry,
		},
	}

	problems := strings.Join(validate(cat), "\n")

	assert.Contains(t, problems, "1 commits are neither mapped to entries nor skipped: bbb")
	assert.Contains(t, problems, `commit ccc: unknown entry "missing-entry"`)
	assert.Contains(t, problems, `commit ddd: unknown skip reason "whatever"`)
	assert.Contains(t, problems, "entry orphan-entry: no ledger commit maps to it")
	assert.Contains(t, problems, "entry breaking-entry (breaking-entry.yaml): migration is required")
	assert.NotContains(t, problems, "entry good-entry")
}

func validEntry(id string) *entry {
	return &entry{
		ID:          id,
		Title:       "Something changed",
		Type:        typeChange,
		Breaking:    lo.ToPtr(false),
		Products:    []string{"nelm"},
		Domain:      "deploy",
		Areas:       []string{"plan"},
		Description: "Before: x. Now: y.",
		fileName:    id + ".yaml",
	}
}
