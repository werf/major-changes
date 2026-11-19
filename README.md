# Changes between werf v2 → v3 and Nelm v1 → v2

A catalogue of every user-visible change between the last werf v2 and Nelm v1 releases and the current werf v3 and Nelm v2, breaking or not. It is the source for the migration guides, release announcements and release notes, and is meant to be queried by scripts as well as read.

Read the rendered views: [werf.md](werf.md) (werf v2 → v3, including the Nelm changes werf users see) and [nelm.md](nelm.md) (Nelm v1 → v2).

## Layout

| Path | What it is | Edited by |
|---|---|---|
| `config.yaml` | The products and the git ranges the catalogue covers, pinned to commits. | hand |
| `ledger/<repo>.yaml` | One row per commit of the range: the entries it maps to, or why it is skipped. | sync adds rows, hand classifies them |
| `entries/<id>.yaml` | One file per user-visible change. | hand |
| `guides/<product>.yaml` | Which section of the product's migration guide covers each breaking entry. | hand |
| `werf.md`, `nelm.md` | Rendered views. | generated, never edit |
| `cmd/majorchanges` | The tool that syncs, validates and renders. | hand |

The tool lives in `cmd/majorchanges` and runs through [Task](https://taskfile.dev):

- `task generate` validates the catalogue and renders the views. Offline. CI runs it on every push and pull request.
- `task sync` first adds the commits of the pinned ranges missing from the ledgers. Needs git and network; clones go to the user cache dir unless `-- -repos-dir <dir>` points to a directory with a clone per range name.

## How completeness is ensured

The ranges in `config.yaml` define the commit set exactly: `git log <new> ^<old>...` for each repository, merges included: a merge can carry changes of its own, such as conflict resolutions or edits made while merging. Sync appends every commit of that set to the ledger, and validation fails while any of them is neither mapped to an entry nor skipped with a reason, while a ledger row points outside the range or to a missing entry, and while an entry has no commit mapped to it. So every commit is accounted for exactly once, and every entry can be traced to its commits and pull requests.

Sync classifies only what is mechanical: release commits (`skip: release`), changes the old version already has (`skip: in-old`), detected as a patch-equivalent commit or a commit with the same subject on the old side (the `note` names which), and merges whose `git show --remerge-diff` is empty (`skip: merge`). Everything else is classified by hand; for a merge, from its remerge diff.

Besides werf and Nelm, the ranges include the libraries whose behavior users see through them: kubedog (resource tracking), common-go (secrets, CLI helpers), lockgate (locking) and logboek (logging), pinned to the versions the old and new releases use in `go.mod`. The Helm fork is part of the Nelm repository (`pkg/helm`), so its changes are Nelm commits.

## Migration guides

The migration guides themselves live with the products: werf's in its docs, Nelm's in its repository. `guides/<product>.yaml` names the guide in `path` and lists its `sections`, each with the guide's heading `anchor` and the breaking entries it covers; `omitted` maps an entry left to the catalogue to the reason. Validation fails while a breaking entry of the product is in neither, so a guide can't fall behind the catalogue.

```yaml
path: https://github.com/werf/nelm/blob/main/docs/migration-v1-to-v2.md
sections:
  - anchor: before-upgrading
    entries: [cli-repo-commands-under-chart]
omitted:
  go-api-legacy-packages-removed: Go API, covered by the API section's link to the catalogue
```

## Go API

The `go-api` entries were checked against a mechanical diff of Nelm's exported API, made with `apidiff` (`go install golang.org/x/exp/cmd/apidiff@latest`) on checkouts of the old and new versions. Whole-module mode (`apidiff -m`) fails on v1.31.3, because `pkg/helm/pkg/repo/repotest` does not build there, so it is done per package present in both versions:

```shell
# In the v1.31.3 checkout, and likewise in the new one with github.com/werf/nelm/v2/<pkg>.
apidiff -w /tmp/old/<pkg>.export github.com/werf/nelm/<pkg>
# Incompatible changes only.
apidiff -incompatible /tmp/old/<pkg>.export /tmp/new/<pkg>.export
```

Type changes that only reflect the `/v2` module path are covered by `go-api-module-path-v2`, `pkg/helm` by `go-api-helm-fork-packages-restructured`, and packages that exist in only one version are compared by hand. Every other reported change must be described by an entry, naming the identifier.

## Ledger rows

```yaml
- {commit: 98d48dbbadbb, date: 2026-11-08, subject: "fix: ...", entries: [deploy-delete-after-failed-revision]}
- {commit: 3ef6dc9bd0a1, date: 2026-04-07, subject: "refactor: ...", skip: internal}
- {commit: af5bb537bc89, date: 2026-06-23, subject: "fix(build): ...", skip: in-old, note: "same subject as 9bec3e6109b6 in v2.84.1"}
```

A commit maps to one or more entries, or is skipped with one of these reasons:

| `skip` | Use for |
|---|---|
| `in-old` | The old version already has this change, for example as a backport. `note` names the old commit. |
| `release` | Release automation: version bumps, changelogs, release channels. |
| `ci` | CI workflows and repository automation. |
| `test` | Tests only. |
| `docs` | Documentation only: the website, READMEs, reference pages. |
| `internal` | No user-visible effect: refactoring, build tooling, linters, agent instructions. |
| `dependency` | Dependency bumps without a user-visible effect, or whose effect is catalogued from another ledger (`note` says which). A bump that is how this product delivers a feature or fix (a `feat` or `fix` commit) maps to that entry instead, so its pull request is referenced. |
| `merge` | A merge that adds no change of its own. Sync sets it when `git show --remerge-diff` is empty; set it by hand when the remerge diff only resolves conflicts in a way that keeps what both sides did. |
| `reverted` | Reverted within the range, and the revert itself. `note` names the counterpart. |

A commit that fixes or refines something new in this major maps to that feature's entry rather than to a fix entry: users of the old version never saw the bug.

## Entries

```yaml
id: deploy-delete-propagation-background
title: Resources are deleted with Background propagation by default
type: change
breaking: true
products: [nelm, werf]
domain: deploy
areas: [resources]
description: |
  Before: dependents of deleted resources were removed with `Foreground` propagation.
  Now: `Background` propagation is used.
migration: |
  To keep the previous behavior, pass `--delete-propagation=Foreground`.
refs:
  - https://werf.io/docs/v3/resources/migration_from_v2_to_v3.html#resource-deletion-and-recreation
```

| Field | Values |
|---|---|
| `id` | Lowercase kebab-case, permanent, equal to the file name. |
| `title` | One line in the present tense, describing the new state. |
| `type` | `feature` — new capability; `change` — existing behavior or default changed; `removal` — something removed; `deprecation` — still works, will be removed; `fix` — wrong behavior of the old version corrected. |
| `breaking` | `true` when a user of the old version may have to act, or gets a different result for the same input: a changed default, removed or renamed command, flag, option or key, changed output or exit code, stricter validation, an incompatible Go API change. Set explicitly. |
| `products` | Where users see the change: `werf`, `nelm`, or both. Nelm deploy-engine changes werf users see list both. |
| `domain` | `general`, `build`, `deploy`, `cleanup` or `go-api` (the Nelm Go library). |
| `areas` | One or more of the list in `cmd/majorchanges/validate.go`. |
| `description` | Markdown, user-facing: what was before and what is now, with exact names of commands, flags, keys, annotations and environment variables. |
| `migration` | Required when `breaking`, and for removals and deprecations: what to do. |
| `refs` | Optional extra links: issues, docs. Pull requests and commits are derived from the ledger. |

## Querying

```shell
# All breaking changes werf users see, as JSON.
yq -o json ea '[.] | map(select(.breaking and (.products | contains(["werf"]))))' entries/*.yaml

# The deploy-related part of werf v3, which is effectively the Nelm v2 part.
yq -N ea 'select(.domain == "deploy" and (.products | contains(["werf"]))) | .id' entries/*.yaml

# Changes werf users get through Nelm, whatever the domain.
yq -N ea 'select(.products | contains(["nelm", "werf"])) | .id' entries/*.yaml

# Commits and pull requests of an entry.
grep -h 'entries: \[[^]]*\bdeploy-delete-propagation-background\b' ledger/*.yaml

# Commits not yet classified.
grep -h -v -e 'entries:' -e 'skip:' ledger/*.yaml | grep '^-'
```

## Updating

1. Move the `new` pin of a range in `config.yaml` (or an `old` pin when a newer old release ships a backport).
2. Run `task sync`. It adds the new commits to the ledgers and lists the unclassified ones.
3. Classify each new commit: map it to an existing entry, create an entry, or skip it with a reason. A new breaking entry also needs a place in `guides/<product>.yaml`, and usually in the guide.
4. Run `task generate` until it reports no problems, and commit the result.
