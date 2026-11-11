package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/lo"
)

const (
	skipCI          = "ci"
	skipDependency  = "dependency"
	skipDocs        = "docs"
	skipInOld       = "in-old"
	skipInternal    = "internal"
	skipMerge       = "merge"
	skipRelease     = "release"
	skipReverted    = "reverted"
	skipTest        = "test"
	typeChange      = "change"
	typeDeprecation = "deprecation"
	typeFeature     = "feature"
	typeFix         = "fix"
	typeRemoval     = "removal"
)

var (
	idRegexp    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	skipReasons = []string{skipCI, skipDependency, skipDocs, skipInOld, skipInternal, skipMerge, skipRelease, skipReverted, skipTest}
	types       = []string{typeFeature, typeChange, typeRemoval, typeDeprecation, typeFix}
	domains     = []string{"general", "build", "deploy", "cleanup", "go-api"}
	areas       = []string{
		"annotations", "buildah", "build-report", "bundles", "cache", "charts", "ci-env", "cli", "cleanup",
		"compose", "config", "deploy-report", "dependencies", "distribution", "dockerfile", "git", "giterminism",
		"go-api", "host-cleanup", "images", "kube-run", "kubectl", "locking", "logging", "oci", "patches", "performance",
		"plan", "platform", "registry", "release-storage", "render", "resources", "rollback", "secrets", "stapel",
		"templates", "tracking", "typescript", "uninstall", "validation", "values", "other",
	}
)

func validate(cat *catalogue) []string {
	var problems []string

	referenced := map[string]bool{}

	for _, r := range cat.config.Ranges {
		rows := cat.ledgers[r.Name]
		if len(rows) == 0 {
			problems = append(problems, fmt.Sprintf("ledger %s is empty, run the sync", r.Name))
		}

		var unclassified []string

		seen := map[string]bool{}

		for _, row := range rows {
			where := fmt.Sprintf("ledger %s, commit %s", r.Name, row.Commit)

			if seen[row.Commit] {
				problems = append(problems, where+": listed more than once")
			}

			seen[row.Commit] = true

			switch {
			case len(row.Entries) == 0 && row.Skip == "":
				unclassified = append(unclassified, row.Commit)
			case len(row.Entries) > 0 && row.Skip != "":
				problems = append(problems, where+": has both entries and skip")
			case row.Skip != "" && !lo.Contains(skipReasons, row.Skip):
				problems = append(problems, fmt.Sprintf("%s: unknown skip reason %q", where, row.Skip))
			}

			for _, id := range row.Entries {
				referenced[id] = true

				if _, ok := cat.entries[id]; !ok {
					problems = append(problems, fmt.Sprintf("%s: unknown entry %q", where, id))
				}
			}
		}

		if len(unclassified) > 0 {
			problems = append(problems, fmt.Sprintf("ledger %s: %d commits are neither mapped to entries nor skipped: %s",
				r.Name, len(unclassified), strings.Join(unclassified, ", ")))
		}
	}

	productNames := lo.Map(cat.config.Products, func(p product, _ int) string { return p.Name })

	for _, e := range sortedEntries(cat) {
		problems = append(problems, validateEntry(e, productNames)...)

		if !referenced[e.ID] {
			problems = append(problems, fmt.Sprintf("entry %s: no ledger commit maps to it", e.ID))
		}
	}

	return problems
}

func logStats(cat *catalogue) {
	for _, r := range cat.config.Ranges {
		counts := lo.CountValuesBy(cat.ledgers[r.Name], func(row *ledgerRow) string {
			switch {
			case len(row.Entries) > 0:
				return "mapped"
			case row.Skip != "":
				return "skip:" + row.Skip
			default:
				return "unclassified"
			}
		})

		keys := lo.Keys(counts)
		slices.Sort(keys)

		fmt.Fprintf(os.Stderr, "Ledger %s: %s\n", r.Name, strings.Join(lo.Map(keys, func(k string, _ int) string {
			return fmt.Sprintf("%s=%d", k, counts[k])
		}), " "))
	}

	fmt.Fprintf(os.Stderr, "Entries: %d\n", len(cat.entries))
}

func validateEntry(e *entry, productNames []string) []string {
	var problems []string

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("entry %s (%s): ", e.ID, e.fileName)+fmt.Sprintf(format, args...))
	}

	if !idRegexp.MatchString(e.ID) {
		add("id must be lowercase kebab-case")
	}

	if e.fileName != e.ID+".yaml" {
		add("file must be named after the id")
	}

	if strings.TrimSpace(e.Title) == "" || strings.Contains(e.Title, "\n") {
		add("title must be a single non-empty line")
	}

	if !lo.Contains(types, e.Type) {
		add("type %q is not one of %v", e.Type, types)
	}

	if e.Breaking == nil {
		add("breaking must be set explicitly")
	}

	if len(e.Products) == 0 {
		add("products must not be empty")
	}

	for _, p := range e.Products {
		if !lo.Contains(productNames, p) {
			add("unknown product %q", p)
		}
	}

	if !lo.Contains(domains, e.Domain) {
		add("domain %q is not one of %v", e.Domain, domains)
	}

	if len(e.Areas) == 0 {
		add("areas must not be empty")
	}

	for _, a := range e.Areas {
		if !lo.Contains(areas, a) {
			add("unknown area %q", a)
		}
	}

	if strings.TrimSpace(e.Description) == "" {
		add("description must not be empty")
	}

	needsMigration := (e.Breaking != nil && *e.Breaking) || e.Type == typeRemoval || e.Type == typeDeprecation
	if needsMigration && strings.TrimSpace(e.Migration) == "" {
		add("migration is required for breaking changes, removals and deprecations")
	}

	for _, ref := range e.Refs {
		if !strings.HasPrefix(ref, "https://") {
			add("ref %q must be an https URL", ref)
		}
	}

	return problems
}
