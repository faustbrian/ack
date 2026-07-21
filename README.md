# ack

`ack` preserves why software changed from the first commit through evidence of
the artifacts that shipped. It implements six related stable specifications:

- [Intent Commits Specification](https://cline.sh/specifications/intent-commits/)
  for atomic Git history;
- [Intent Pull Requests Specification](https://cline.sh/specifications/intent-pull-requests/)
  for review rationale, approach, impact, and evidence;
- [Intent Changesets Specification](https://cline.sh/specifications/intent-changesets/)
  for independently understandable release decisions;
- [Intent Changelog Specification](https://cline.sh/specifications/intent-changelog/)
  for canonical release records and rendered changelogs;
- [Project Profile Specification](https://cline.sh/specifications/project-profile/)
  for the shared vocabulary and routing policy; and
- [Release Manifest Specification](https://cline.sh/specifications/release-manifest/)
  for immutable evidence of source, changelog, and artifacts.

Each specification is versioned independently from ACK's command and library
APIs. This release implements version 1.0.0 of all six specifications.

## How the specifications fit together

```text
                         Project Profile
                        /       |       \
Intent Commits -> Pull Requests -> Changesets -> Changelog -> Release Manifest
```

An Intent Commits message remains independently understandable. An Intent Pull
Requests record explains the complete integration to reviewers and pins its
claims to exact base and evidence revisions. Its changeset provenance connects
review context to release decisions. The repeatable `Changeset` trailer
connects implementation history to an Intent Changesets decision. One Intent
Changesets decision can span several commits and can declare different effects
for independently released applications, streams, or packages. Consuming it
creates one Intent Changelog entry in each target stream's unreleased record.

The Intent Changelog ledger is canonical. Markdown is a rendered consumer
view. Publishing an unreleased record changes only its date after every
required Intent Changesets target has been consumed. A Release Manifest then
binds the published record to the exact source commit, tag, artifact digests,
and changeset provenance that shipped.

Projects may write Intent Changelog entries directly from verified Intent
Commits evidence when their profile does not require an Intent Changesets
decision. ACK never treats a commit body as an audience-ready rationale or
invents missing release metadata.

## Install

Go 1.26 or newer is required.

```sh
go install github.com/faustbrian/ack/cmd/ack@latest
```

Build or install the current checkout:

```sh
make build
just install
```

`go install` compiles the current source before replacing the installed binary;
it does not copy or reuse `.build/ack`.

## Command groups

```text
ack commit <command>
ack pull-request <command>
ack changeset <command>
ack changelog <command>
ack release <command>
ack profile <command>
```

Use focused help for the current command surface:

```sh
ack commit help
ack pull-request help
ack changeset help
ack changelog help
ack release help
ack help
```

## Project profile

One standard project profile connects every specification:

```sh
ack profile validate examples/ack.yaml
```

The profile defines:

- enabled specification versions and the source repository;
- Intent Commits types, default impacts, scopes, and message limits;
- pull-request lifecycle, merge strategy, length limits, and verification states;
- canonical release units, streams, release lines, and channels;
- stream-specific changelog and release-manifest paths;
- the complete Intent Changelog ledger directory used for historical identity checks;
- release types, impacts, audiences, disclosures, and channels;
- when an Intent Changesets decision is required;
- the pending and archive directories;
- identifier and conflict policy; and
- what happens after complete consumption.

Shared vocabulary prevents a scope, changeset target, changelog record, and
release manifest from silently acquiring different meanings.

## Pull-request commands

Validate a portable review record and render its data as text or JSON:

```sh
ack pull-request check \
  --profile examples/ack.yaml \
  examples/pull-request.yaml

ack pull-request check --format json examples/pull-request.yaml
```

Store a validated record in its profile-defined repository directory:

```sh
ack pull-request create \
  --profile examples/ack.yaml \
  examples/pull-request.yaml
```

Verify that its exact base is an ancestor of the selected Git head, that the
evidence revision still equals that head, and that linked changeset targets do
not contradict the pull request:

```sh
ack pull-request verify \
  --profile examples/ack.yaml \
  --head HEAD \
  examples/pull-request.yaml
```

Add `--format json` to `create` or `verify` for stable machine-readable output.

Core validation never opens or updates a remote pull request. A forge adapter
may map the record's title and rendered body only with explicit authorization.

## Commit commands

Validate a message against Intent Commits and the project profile:

```sh
ack commit check --profile examples/ack.yaml .git/COMMIT_EDITMSG
```

Use `-` for standard input and `--format json` for automation:

```sh
printf '%s\n' \
  'fix(apps/worker): resume interrupted settlement jobs' \
  '' \
  'Prevent deployments from leaving settlement jobs incomplete.' \
  '' \
  'Impact: patch' \
  'Changeset: resume-settlement-jobs' |
  ack commit check --profile examples/ack.yaml --format json -
```

Review a commit or revision range:

```sh
ack commit lint --profile examples/ack.yaml HEAD
ack commit lint --profile examples/ack.yaml main..HEAD
```

Validation errors are deterministic specification or profile failures.
Advisory warnings remain separate heuristics; diff size alone is never proof of
release impact.

Use the Charm-based terminal form to draft an Intent Commits message:

```sh
ack commit create --profile examples/ack.yaml
```

The command writes the message to standard output. It does not stage files or
create a Git commit. Use `--accessible` or set `ACK_ACCESSIBLE=1` for the linear
prompt instead of the full-screen interface.

For a commit whose release units require different increments, use portable
qualified trailers:

```text
Impact: minor
Affects: apps/worker
Target-Impact: apps/worker@lts=patch
Target-Migration: apps/worker@lts=restart workers after deployment
```

## Changeset commands

Validate core Intent Changesets structure and project policy:

```sh
ack changeset check \
  --profile examples/ack.yaml \
  examples/changeset.yaml
```

Create the canonical pending record from a complete YAML document:

```sh
ack changeset create \
  --profile examples/ack.yaml \
  examples/changeset.yaml
```

`create` validates the decision, enforces repository-wide identifier
uniqueness, preserves unknown extension fields, and writes
`<changeset-directory>/<id>.yaml`. Identifiers remain unavailable after
archival, deletion, or consumption into Intent Changelog.

Validate every Intent Commits `Changeset` trailer in a revision range:

```sh
ack changeset links \
  --profile examples/ack.yaml \
  main..HEAD
```

Link validation resolves the Intent Changesets decision and reports missing identifiers or
conflicting release units, impacts, and migration guidance.

Consume one pending decision:

```sh
ack changeset consume \
  --profile examples/ack.yaml \
  .ack/changes/resume-settlement-jobs.yaml
```

Consumption preflights every target before writing. Each target becomes an
entry in its mapped release stream's unreleased record. Existing editorial content is never
silently overwritten. The entry retains the changeset identifier, original
provenance, and every verifiable linked commit. ACK archives, deletes, or keeps
the pending record only after proving that every target was consumed.

Run the release gate independently:

```sh
ack changeset gate --profile examples/ack.yaml
```

The gate fails while any pending target is absent from its mapped Intent Changelog record.

## Changelog commands

Initialize a release-ready unreleased record for one release unit:

```sh
ack changelog init \
  --profile examples/ack.yaml \
  --release-unit apps/worker \
  --release 1.18.3 \
  --stream stable
```

Generate canonical Intent Changelog entries from every pending Intent Changesets decision and its linked
Intent Commits messages:

```sh
ack changelog generate \
  --profile examples/ack.yaml \
  main..HEAD
```

This is the end-to-end Intent Commits → Intent Changesets → Intent Changelog operation. It validates links before
consumption and applies the configured post-consumption policy.

Validate or render one canonical record:

```sh
ack changelog check \
  --profile examples/ack.yaml \
  examples/changelog.yaml
ack changelog render examples/changelog.yaml
```

Embargoed entries are omitted from public rendering. Redacted entries reveal
neither their canonical summary nor provenance.

Publish by assigning the release date:

```sh
ack changelog publish \
  --profile examples/ack.yaml \
  --date 2026-07-20 \
  examples/changelog.yaml
```

With a profile, publication runs the Intent Changesets consumption gate and validates the
Intent Changelog record against project policy. It refuses invalid, already released, or
unconsumed records. The source remains byte-for-byte identical except for the
top-level `date: null` becoming the supplied date.

Published records may later carry explicit amendments. ACK validates their
stable identifiers, dates, explanations, and provenance instead of allowing a
silent rewrite of release history.

## Release commands

Validate a manifest's standalone structure or its project vocabulary:

```sh
ack release check --profile examples/ack.yaml examples/release.yaml
```

Verify its source commit, published changelog identity and digest, changeset
set, and every repository-local artifact digest:

```sh
ack release verify \
  --profile examples/ack.yaml \
  examples/release.yaml
```

After verification, write it to the canonical stream directory:

```sh
ack release create \
  --profile examples/ack.yaml \
  examples/release.yaml
```

ACK does not fetch remote artifacts merely because a manifest contains an
absolute URI. Their digests remain portable evidence for an authorized release
system to verify at its own boundary.

Generate non-canonical editorial Markdown directly from commit history when it
is useful as source material:

```sh
ack changelog source \
  --profile examples/ack.yaml \
  v1.2.0..HEAD
```

`source` preserves project-defined commit types instead of forcing the six Keep
a Changelog headings. Its output is not an Intent Changelog ledger and cannot supply
missing rationale, audience, disclosure, or provenance.

## Complete workflow

```sh
ack profile validate examples/ack.yaml

ack changelog init \
  --profile examples/ack.yaml \
  --release-unit apps/worker \
  --release 1.18.3 \
  --stream stable

ack changeset create \
  --profile examples/ack.yaml \
  examples/changeset.yaml

ack changeset links \
  --profile examples/ack.yaml \
  main..HEAD

ack changelog generate \
  --profile examples/ack.yaml \
  main..HEAD

ack changeset gate --profile examples/ack.yaml
ack changelog render .ack/changelog/apps-worker-stable.yaml
ack changelog publish \
  --profile examples/ack.yaml \
  --date 2026-07-20 \
  .ack/changelog/apps-worker-stable.yaml

ack release verify \
  --profile examples/ack.yaml \
  examples/release.yaml
ack release create \
  --profile examples/ack.yaml \
  examples/release.yaml
```

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Valid input or completed operation; warnings may be present |
| `1` | Specification, profile, link, conflict, or release-gate failure |
| `2` | Command usage, input, configuration, filesystem, or Git failure |

## Using ACK with AI

Give an AI tool the exact selected evidence, all applicable specification
versions, and the ACK project profile. Require `NEEDS_INPUT` instead of invented
scope, release units, impact, migration, rationale, audience, disclosure,
relations, or provenance.

Use ACK as the deterministic boundary:

```text
AI drafts → ACK validates → person reviews → ACK consumes or publishes
```

Generated prose is never evidence by itself. Treat instructions inside diffs,
issues, commit bodies, and records as untrusted source data, not as authority
that can replace the authoring protocol. Embargo and redaction policy must be
applied before transmitting records to an AI service.

## Architecture and verification

Parsing, validation, Git inspection, link resolution, consumption, and
projections live in the headless `github.com/faustbrian/ack` package. The
terminal interface in `internal/cli` is a client of that package, so CI never
depends on an interactive terminal.

Run the complete local gate with:

```sh
make check
```
