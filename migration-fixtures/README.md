# Migration fixtures

Small projects that use werf v2 and Nelm v1 features changed in werf v3 and Nelm v2. They test the migration guides: a project is deployed with the old version, migrated by following only the guide, and then upgraded in place with the new version over the existing release, bundle and secrets.

| Fixture | What it uses |
|---|---|
| `werf-stapel` | `artifact`, Ansible, `docker`, `import.stage`, `fromImage`, untagged `from`, trailing slashes in `to`, overlapping imports, `stageDependencies`, `disableGitAfterPatch`, an unknown giterminism key |
| `werf-dockerfile` | `.containerignore` without `.dockerignore`, the build report |
| `werf-chart` | `werf helm` commands, removed flags, `HELM_DRIVER`, `$.Runtime`, `.Values.global.env`, external and legacy dependency annotations, non-string annotations, `null`, `werf.io/sensitive`, `.helmignore`, a draft-07 values schema, secret values, `export-values` |
| `werf-bundle` | `werf bundle publish` with values, secrets and a non-semver tag, `werf bundle apply`, `werf bundle copy` to an archive |
| `nelm-chart` | Nelm v1 feature gates, `release list/history` output, `--save-rollback-graph-to`, `--set-runtime-json`, plan exit codes, secret values |

`coverage.yaml` says which breaking entry each fixture exercises, or why none does.

## Running

Requires Docker, kind, kubectl, jq, and the binaries in `$MIGRATION_DIR/bin` (`werf-2.85.1`, `werf-3.13.1`, `nelm-1.31.3`, `nelm-2-main`; see `run.sh` for the variables).

```shell
./run.sh cluster-up                   # kind cluster "werf-migration" and a local registry
./run.sh baseline                     # deploy every fixture with werf v2 / Nelm v1
./run.sh upgrade werf-chart <dir>     # run a migrated copy with werf v3 / Nelm v2 over the baseline
./run.sh cluster-down
```

Secret files are kept as `*.plain` and encrypted with the old version when a fixture is prepared; keys stay in the work directory.
