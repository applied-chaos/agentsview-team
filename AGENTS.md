# agentsview-team

<!-- shared-rules:start (source: mintopia/home AGENTS.md; edit there, then copy) -->
## Shared rules

### Talking to me
- Be extremely concise; sacrifice grammar for concision.
- British English in prose, docs, UI copy and commits; identifiers follow the language's convention. No em dashes.
- Plain industry-standard terms.

### Working
- Finish the task. Own problems found on the way, including pre-existing failures.
- Fix root causes.
- Prove it works against the real artifact (run it, read the value, inspect the diff) before saying done.
- Match UI to its mockup exactly; raise a wrong or missing mockup on the ticket.
- Type-system discipline: illegal states unrepresentable, parse at boundaries, exhaust variants, derive types from the authoritative schema.
- Model the domain in data structures, not scattered conditionals.
- Make operations idempotent across partial runs, crashes and retries.

### Quality gate
- Red/green TDD for new behaviour and bug fixes.
- Before commit: formatter, linter, static analysis and tests pass at the repo's level. The repo's AGENTS.md names commands and any coverage bar.
- Comments: non-obvious why only, one or two lines. Run /comment-check before finishing.

### Subagents
- Delegate to subagents; run parallel teams that coordinate by messaging.
- Set model and subagent type explicitly:
  - Planning, reasoning, deep debugging: Opus / Sol
  - Explore, coding, review: Sonnet / Terra
  - Trivial edits, docs: Haiku / Luna
- Long results go to a scratchpad file; return one line plus the path.

### Git
- Work on the branch you started on; parallel work goes in a worktree off it.
- Rebase only onto your own base branch.
- Delete merged or abandoned branches and worktrees, locally and on the remote.

### Decisions


Before proposing architecture, tooling, style, testing, repo structure, or workflow changes in any project:

1. Check for a `docs/decisions/` directory in the project root.
2. If it exists, search it for relevant ADRs (Architecture Decision Records).
3. Treat accepted ADRs as binding unless explicitly overridden by the user.
4. If a decision is outdated, propose a new ADR that supersedes it rather than silently ignoring.
5. Do not re-litigate accepted decisions unless new constraints are introduced.

When creating new ADRs, use this format:

```markdown
# Decision: [Title]

Status: [proposed|accepted|deprecated|superseded]
Date: [YYYY-MM-DD]

## Context
[Why this decision is needed]

## Decision
[What was decided]

## Consequences
[Trade-offs and implications]

## Supersedes
[Reference to previous ADR, or "None"]
```

### Code exploration

Always use jCodeMunch-MCP for code navigation. Never fall back to Read, Grep, Glob, or Bash for code exploration.
**Exception:** use `Read` when you are about to edit a file — the harness requires a `Read` before `Edit`/`Write`. Use jCodeMunch to *find and understand* code, then `Read` only the file you are changing.

This server runs the **front door** surface: three tools reach every jCodeMunch capability, so the tool list stays small and the catalogue is fetched only when you need it.

**Start any session:**
1. `order { "action": "resolve_repo", "args": { "path": "." } }` — confirm the project is indexed. If it is not: `order { "action": "index_folder", "args": { "path": "." } }`

**Then, for any task:**
- Know what you want → `order { "action": "<name>", "args": { ... } }`
- Know the goal, not the tool → `route { "query": "your task in a sentence" }` picks the action and shapes the arguments
- Want to see what exists → `menu { "query": "what you are trying to do" }` returns matching actions with example arguments
- Want the whole catalogue and the usage rules → `jcodemunch_guide`

`menu` and `jcodemunch_guide` list every action this server can run, including ones absent from your tool list. That is expected: the front door is the way to call them.

**Interpreting results:**
- A `verdict` of `no_implementation_found` is evidence of absence. Report the gap; do not re-search with different wording.
- A `verdict` of `degraded` means a channel was unavailable, so absence is NOT proven. Read the note before relying on the result.
- `source: ""` alongside `source_status` means the body could not be read, not that the symbol is empty.

**After editing files:**
- With PostToolUse hooks installed (Claude Code), edited files are reindexed automatically.
- Otherwise `order { "action": "register_edit", "args": { "paths": [...] } }` after an edit, batched for bulk changes.

**Announce your model once per session** so the server can size its answers: `announce_model { "model": "<your-model-id>" }`.

<!-- shared-rules:end -->

## This repo


Go service plus Svelte SPA in one image, published to `ghcr.io/applied-chaos/agentsview-team`. Production Docker Compose example in `deploy/` (see README.md).

- `make check` is the gate (also run by `.github/workflows/ci.yml`): gofmt, prettier, go vet, golangci-lint, eslint, svelte-check, unit tests, compose e2e, Playwright. Individual targets: `fmt`, `fmt-check`, `lint`, `unit`, `e2e`, `ui`, `image`, `down`.
- Runs can overlap on one Docker daemon (parallel worktrees, CI push plus pull_request). `compose.yaml` has no `name:` and publishes no host ports; the Makefile exports `COMPOSE_PROJECT_NAME` (`avteam-<cksum of the checkout path>`; CI sets it from run id and attempt), and every compose call and `down` uses it. `make ui` runs Playwright as the `ui` compose service (`web/Dockerfile.ui`, official image pinned to the `@playwright/test` version) in `platform-prod`'s network namespace (the Platform's session cookies are Secure, which Chromium accepts over HTTP only for localhost: the app is `http://localhost:8080`, `fakeoidc:9000` resolves by name), and copies screenshots and failed-run results out with `compose cp` (no bind mounts, so it works from a CI job container). Keep new services off host ports.
- CI (`.github/workflows/ci.yml`): `check` on pushes to `main` and PRs; then multi-arch images to GHCR for `main` and `v*` tags (`v*` also gets `latest` when it is the newest). A weekly schedule rebuilds `main` and the newest release without cache, re-pushing their tags.
- `deploy/seccomp.json` is the one sandbox profile; the dev `compose.yaml` uses it too.
- e2e tests call `t.Parallel()` first (`-parallel 8`). Tests that change Platform-wide state (test clock, replay hooks, Admin set, secret key) stay serial and run before the parallel ones resume. Every signed-in User provisions an Instance (CREATE/DROP DATABASE), so e2e groups tests by area into suites (`TestHosts`, `TestInstallTokens`, `TestInstanceProxy`, `TestAdmin`, `TestSessions`, `TestReplay`) whose `t.Run` subtests share one User (`fx`), plus run-wide `plainUser`/`adminUser` (`fixtures_test.go`; a new session is free). Subtests use unique names and judge only what they add. A test that mutates its User (Disable, Delete, sabotage, clock, replay hooks) keeps its own.
- Tests sit at the Platform HTTP seam: `e2e` (Go, tag `e2e`) runs in compose against real Postgres, a fake OIDC issuer and a `testclock` build. No tests of internals unless a pure rule is unreachable through the seam.
- Install scripts live in `internal/hosts/install/` (`shellcheck` in `make lint`; `__PLATFORM_URL__` and `__VERSION__` are replaced when served). The Linux install is tested end to end by `e2e/install_test.go`: compose services `linux-systemd` and `linux-profile` (share the platform network namespace, so it is loopback to the raw-sync client) run the real command with the real pinned `agentsview`; test seams are `AV_PLATFORM_URL` and `AV_DOWNLOAD_BASE` (a `file://` release dir built into the image).
- Initial backfill (`run.sh`, all three OS scripts): `source raw backfill` rejects a provider if any one default root is missing, and only some providers support raw capture (v0.47.0: claude, codex, goose, forge, piebald, warp, zcode, evener, crush). So `run.sh` reads `agentsview source list --format json`, and per provider with an existing default root runs `--run-id initial-<provider>`; if a root is missing it re-runs per existing root with the provider's env override (`CODEX_SESSIONS_DIR=<root>`, run id `initial-<provider>-<n>`). Unsupported providers and unavailable roots are skipped. Done run ids sit in `backfilled.d/`; `backfilled` holds `2` only when every present provider finished (an empty marker from an older script is ignored, so re-running the install heals it). The watcher needs none of this: it tolerates missing roots.
- UI changes: `make ui` writes screenshots to `web/e2e/screens`; pair them with `docs/mockups/png` for the isolated review.

## Agent skills

### Issue tracker

Issues and specs live in this repo's GitHub Issues (`applied-chaos/agentsview-team`), via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default canonical labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` + `docs/decisions/` at the repo root. See `docs/agents/domain.md`.
