# Roadmap

Order follows the concept: a working vertical slice on the author's
machine first, breadth later. Dates are not promised.

## Step 0: verification on the author's machine (next)

- Run docs/EXPERIMENTS.md with the probe; record results there.
- Fix the adapters where a flag or format differs from what the
  documentation said; keep the tests in step with the fix.
- Decide `Capabilities.Streaming` for Codex from what exec actually emits.

## Step 1: vertical slice (in progress)

- [x] Domain, storage, event log, orchestrator, app services, bindings.
- [x] Claude and Codex adapters (flags and parsers, unit-tested).
- [ ] React UI: topics, thread with streaming, composer with mentions and
      modes, context panel, participant settings.
- [ ] `wails dev` on macOS with both real CLIs answering in one topic.
- [ ] Isolation canary passes or the UI labels rounds "soft".

## Step 2: daily use

- Replies as threads (parentId is stored; the UI flattens for now).
- Virtualised message list and measured memory for long topics.
- Agent-proposed context revisions (`proposed` status exists; no adapter
  produces proposals yet).
- [x] Export of a topic (Markdown + JSON) without sealed messages or
      secrets: window buttons and `forumctl export`.
- [x] Verified backups (`forumctl backup`), automatic backup before
      migrations, refusal of databases from newer builds.
- Artifacts: store files in the managed directory and reference versions
  from messages (schema exists).

## Step 3: capabilities

- Tool approvals: Claude through `--permission-prompt-tool` with an MCP
  bridge or the stream-json control protocol; Codex through app-server.
- Strict isolation: Claude sandbox settings (`denyRead` outside the run
  directory), Codex sandbox verification; report `strict` only when the
  canary experiment passes.
- One shared skill and one shared connector with visible permissions.

## Step 4: release

- Windows and Linux builds, each verified separately. The core already
  cross-compiles for windows/amd64, darwin/arm64 and linux/arm64 in CI;
  the Windows process tree kill (job objects) is still missing.
- Signed macOS build, release workflow.
- Public repository hygiene: [x] AGENTS.md, contributing guide, security
  policy, issue and PR templates; [x] dependency licence check
  (docs/LICENSES.md, scripts/licenses.sh); [ ] a demo recording of a real
  round.

## Candidates from the product principles (not scheduled)

docs/PRINCIPLES.md records the principles and maps them onto this plan.
Items it marks as new, none of them scheduled yet:

- Three result states (model consensus, verified, human approved) on
  decisions or claims, with evidence references.
- Disagreement tracking.
- Adversarial round roles and a bounded multi-turn open discussion.
- Per-participant context visibility and isolated branches.
- Diff review on attached changes.
- Explicit limits per topic: rounds and estimated cost.
- Per-run git worktrees for parallel editing of a real repository.

