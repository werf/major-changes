// Command majorchanges maintains the catalogue of changes between major releases of werf and Nelm
// in this repository: it syncs the per-repository commit ledgers with the pinned git ranges,
// validates ledgers and entries, and renders the Markdown views. See README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const defaultDataDir = "."

type options struct {
	dataDir  string
	reposDir string
	sync     bool
}

func run(ctx context.Context) error {
	opts, err := parseFlags()
	if err != nil {
		return err
	}

	cat, err := loadCatalogue(opts.dataDir)
	if err != nil {
		return fmt.Errorf("load catalogue: %w", err)
	}

	if opts.sync {
		if err := syncLedgers(ctx, cat, opts.reposDir); err != nil {
			return fmt.Errorf("sync ledgers: %w", err)
		}

		if err := writeLedgers(cat, opts.dataDir); err != nil {
			return fmt.Errorf("write ledgers: %w", err)
		}
	}

	problems := validate(cat)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}

	if err := render(cat, opts.dataDir); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	logStats(cat)

	if len(problems) > 0 {
		return fmt.Errorf("%d problems found", len(problems))
	}

	return nil
}

func parseFlags() (options, error) {
	var opts options

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return opts, fmt.Errorf("get user cache dir: %w", err)
	}

	flag.StringVar(&opts.dataDir, "data-dir", defaultDataDir, "Catalogue directory")
	flag.StringVar(&opts.reposDir, "repos-dir", filepath.Join(cacheDir, "werf-major-changes"),
		"Directory with the git clones of the ranged repositories, one per range name; missing clones are created")
	flag.BoolVar(&opts.sync, "sync", false,
		"Add the commits of the pinned ranges missing from the ledgers before validating, which needs git and network")
	flag.Parse()

	return opts, nil
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
