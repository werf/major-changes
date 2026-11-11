package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

var (
	prSuffixRegexp     = regexp.MustCompile(`\s*\(#\d+\)$`)
	releaseRegexp      = regexp.MustCompile(`^chore(\([^)]*\))?: release v?\d`)
	unitSeparatorBytes = []byte{0x1f}
)

type gitCommit struct {
	date    string
	parents int
	sha     string
	subject string
}

func syncLedgers(ctx context.Context, cat *catalogue, reposDir string) error {
	for _, r := range cat.config.Ranges {
		dir := filepath.Join(reposDir, r.Name)

		if err := ensureClone(ctx, dir, r); err != nil {
			return fmt.Errorf("prepare clone of %s: %w", r.Name, err)
		}

		commits, err := rangeCommits(ctx, dir, r)
		if err != nil {
			return fmt.Errorf("list commits of %s: %w", r.Name, err)
		}

		inOld, err := findInOld(ctx, dir, r)
		if err != nil {
			return fmt.Errorf("find commits of %s already in the old version: %w", r.Name, err)
		}

		cleanMerges, err := findCleanMerges(ctx, dir, cat.ledgers[r.Name], commits)
		if err != nil {
			return fmt.Errorf("find merges of %s without changes of their own: %w", r.Name, err)
		}

		rows, added, err := mergeLedger(cat.ledgers[r.Name], commits, inOld, cleanMerges)
		if err != nil {
			return fmt.Errorf("merge ledger %s: %w", r.Name, err)
		}

		cat.ledgers[r.Name] = rows

		fmt.Fprintf(os.Stderr, "Ledger %s: %d commits in range, %d added\n", r.Name, len(rows), added)
	}

	return nil
}

func ensureClone(ctx context.Context, dir string, r gitRange) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, err := git(ctx, "", "clone", "--quiet", "--bare", r.URL, dir); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat %q: %w", dir, err)
	}

	pins := append([]pin{r.New}, r.Old...)
	for _, p := range pins {
		if _, err := git(ctx, dir, "cat-file", "-e", p.Commit+"^{commit}"); err == nil {
			continue
		}

		if _, err := git(ctx, dir, "fetch", "--quiet", "--tags", r.URL, "+refs/heads/*:refs/heads/*"); err != nil {
			return err
		}

		break
	}

	return nil
}

// findCleanMerges returns the new merges whose result is what git merges on its own: an empty remerge
// diff. The changes of such merges are the changes of the commits they bring in. Git remerges only
// two-parent merges, so octopus merges are left for manual review. Merge drivers from the
// user's git attributes are disabled so the result does not depend on the machine.
func findCleanMerges(ctx context.Context, dir string, rows []*ledgerRow, commits []gitCommit) (map[string]bool, error) {
	known := lo.SliceToMap(rows, func(row *ledgerRow) (string, bool) { return row.Commit, true })
	clean := map[string]bool{}

	for _, c := range commits {
		if c.parents != 2 || known[c.sha] {
			continue
		}

		diff, err := git(ctx, dir, "-c", "core.attributesFile="+os.DevNull, "show", "--remerge-diff", "--format=", c.sha)
		if err != nil {
			return nil, err
		}

		clean[c.sha] = len(bytes.TrimSpace(diff)) == 0
	}

	return clean, nil
}

// findInOld maps commits of the range to a note naming the old version that already has the same
// change: either a patch-equivalent commit (cherry-pick) or a commit with the same subject (a backport
// that had to be adapted). Same subject is a strong hint, not a proof, which is why it is noted.
func findInOld(ctx context.Context, dir string, r gitRange) (map[string]string, error) {
	notes := map[string]string{}

	for _, old := range r.Old {
		marks, err := git(ctx, dir, "log", "--no-merges", "--cherry-mark", "--right-only", "--format=%m%x1f%H",
			old.Commit+"..."+r.New.Commit)
		if err != nil {
			return nil, err
		}

		for line := range bytes.Lines(marks) {
			fields := bytes.Split(bytes.TrimRight(line, "\n"), unitSeparatorBytes)
			if len(fields) == 2 && string(fields[0]) == "=" {
				notes[string(fields[1][:shortSHALength])] = "patch-equivalent commit in " + old.Ref
			}
		}

		oldOnly, err := git(ctx, dir, "log", "--no-merges", "--format=%H%x1f%s", old.Commit, "^"+r.New.Commit)
		if err != nil {
			return nil, err
		}

		oldSubjects := map[string]string{}

		for line := range bytes.Lines(oldOnly) {
			fields := bytes.Split(bytes.TrimRight(line, "\n"), unitSeparatorBytes)
			if len(fields) == 2 {
				oldSubjects[normalizeSubject(string(fields[1]))] = string(fields[0][:shortSHALength])
			}
		}

		newOnly, err := git(ctx, dir, "log", "--no-merges", "--format=%H%x1f%s", r.New.Commit, "^"+old.Commit)
		if err != nil {
			return nil, err
		}

		for line := range bytes.Lines(newOnly) {
			fields := bytes.Split(bytes.TrimRight(line, "\n"), unitSeparatorBytes)
			if len(fields) != 2 {
				continue
			}

			sha := string(fields[0][:shortSHALength])
			if _, ok := notes[sha]; ok {
				continue
			}

			if oldSHA, ok := oldSubjects[normalizeSubject(string(fields[1]))]; ok && !releaseRegexp.Match(fields[1]) {
				notes[sha] = fmt.Sprintf("same subject as %s in %s", oldSHA, old.Ref)
			}
		}
	}

	return notes, nil
}

func rangeCommits(ctx context.Context, dir string, r gitRange) ([]gitCommit, error) {
	args := []string{"log", "--reverse", "--format=%H%x1f%cs%x1f%P%x1f%s", r.New.Commit}
	for _, p := range r.Old {
		args = append(args, "^"+p.Commit)
	}

	out, err := git(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	var commits []gitCommit

	for line := range bytes.Lines(out) {
		fields := bytes.Split(bytes.TrimRight(line, "\n"), unitSeparatorBytes)
		if len(fields) != 4 {
			return nil, fmt.Errorf("unexpected git log line %q", line)
		}

		commits = append(commits, gitCommit{
			date:    string(fields[1]),
			parents: len(bytes.Fields(fields[2])),
			sha:     string(fields[0][:shortSHALength]),
			subject: string(fields[3]),
		})
	}

	return commits, nil
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

// mergeLedger keeps the classification of commits already in the ledger and appends new commits in
// range order, classifying only what is mechanical: release commits, changes the old version has and
// merges without changes of their own.
func mergeLedger(rows []*ledgerRow, commits []gitCommit, inOld map[string]string, cleanMerges map[string]bool) ([]*ledgerRow, int, error) {
	existing := lo.KeyBy(rows, func(row *ledgerRow) string { return row.Commit })
	inRange := lo.SliceToMap(commits, func(c gitCommit) (string, bool) { return c.sha, true })

	stale := lo.Filter(rows, func(row *ledgerRow, _ int) bool { return !inRange[row.Commit] })
	if len(stale) > 0 {
		return nil, 0, fmt.Errorf("ledger has commits outside the range, remove or remap them: %s",
			strings.Join(lo.Map(stale, func(row *ledgerRow, _ int) string { return row.Commit }), ", "))
	}

	var (
		result []*ledgerRow
		added  int
	)

	for _, c := range commits {
		row, ok := existing[c.sha]
		if !ok {
			row = &ledgerRow{Commit: c.sha}
			added++

			switch {
			case cleanMerges[c.sha]:
				row.Skip = skipMerge
			case c.parents < 2 && releaseRegexp.MatchString(c.subject):
				row.Skip = skipRelease
			case inOld[c.sha] != "":
				row.Skip = skipInOld
				row.Note = inOld[c.sha]
			}
		}

		row.Date = c.date
		row.Subject = c.subject
		result = append(result, row)
	}

	return result, added, nil
}

func normalizeSubject(subject string) string {
	return prSuffixRegexp.ReplaceAllString(strings.TrimSpace(subject), "")
}
