# Product principles

Recorded 2026-11-01 from the author's notes after an architecture
consultation; the original Russian text is kept verbatim in
[notes/2026-11-01-principles.ru.md](notes/2026-11-01-principles.ru.md).

Status: these are accepted principles and candidate requirements. They are
not approved scope and not tasks for immediate implementation. The second
half of this page maps them onto the current architecture and the roadmap
so that the next discussion starts from facts; changing the implementation
in their name needs a separate decision (an ADR or a roadmap entry).

## Concept

A local AI forum for engineering discussions with several independent
models (Claude Code, Codex, potentially Gemini and others). It is not
another autonomous coding agent but an engineering council in which the
user is the coordinator, the moderator and the person who makes the final
decisions. The goal is not just to obtain code but to understand why a
decision was made and to be able to verify it independently.

## Principles

1. **Prove it.** Every technical claim or architectural decision must be
   verifiable. A model's explanation is not, by itself, evidence.
2. **Consensus is not verification.** Two or more models agreeing does not
   make them right. Independent verification relies on code, tests,
   documentation, reproducible results or other objective evidence.
3. **Human in command.** The user controls the discussion, the context, the
   permissions and the final decisions. Agents propose, explain, implement
   and check; they do not replace engineering responsibility.
4. **Independence by design.** Independence between models must be real.
   One model's first answer must not, even unintentionally, influence
   another model's independent assessment.
5. **Safety by architecture, not by prompt.** Reliability comes from
   architecture, tests, access restrictions, transactions, backups and
   checks, not from an agent's promise not to make mistakes.

## Interaction modes

- **Blind Review**: models answer the same task independently without
  seeing each other's answers.
- **Open Discussion**: models see each other's messages and can discuss,
  criticise and refine.
- **Adversarial Review**: one model proposes a solution, another
  deliberately looks for errors, weak points and unfounded assumptions.
- **Moderator Mode**: the user decides who answers and when, and which
  messages and context each participant can see.

Modes should be switchable within one discussion while keeping its history.

## Capabilities worth having

- **Disagreement Tracking**: keep the models' disagreements, their
  arguments and the reasons positions changed.
- **Explanation Verification**: one model explains complex code or a
  concept to the user, another independently checks the explanation.
- **Evidence-Based Review**: distinguish claims, assumptions and facts
  confirmed by checks.
- **Diff Review**: discuss specific parts of a change, explain why decisions
  were needed, propose simplifications.
- **Context Visibility Control**: explicit control over what each agent
  sees, including isolated discussion branches.
- **Anti-Sycophancy**: require substantive criticism; do not reward
  automatic agreement between models.
- **Human Approval**: decisions with consequences for files, infrastructure
  or production go through the appropriate checks and confirmations.

Three states of a result must be kept apart. They are independent
properties, not stages of an automatic approval pipeline:

1. **Model consensus**: the models agree.
2. **Verified**: the claim is confirmed by objective checks within a stated
   scope.
3. **Human approved**: the user made the decision.

## Technical constraints

- Local-first, keyboard-first.
- Primary integration through the official Claude Code and Codex CLIs with
  the user's existing personal subscriptions; the main scenario must not
  depend on paid APIs, whose cost is unacceptable under heavy use.
- Extensible adapters for other providers.
- No extraction of authentication tokens, no unofficial APIs; the terms of
  the CLI integrations are respected.
- Shared discussion history with controlled isolation of each model's
  context.
- Limits on autonomous rounds, to prevent endless discussions and the
  burning of plan limits.
- Separation of discussion from action; safe isolation of working
  directories during parallel editing.
- Public GitHub repository under MIT.

## Philosophy

The reliability of the system comes from its architecture, not from the
intelligence of whoever writes the code. The more autonomy the AI gets,
the stricter the engineering constraints must be. Consensus is not
verification. The forum must help an engineer think, understand and verify,
not create an illusion of correctness out of convincing answers.

---

# Mapping onto the current architecture

Legend: **have** = enforced by the implementation today; **partly** = the
mechanism exists but does not cover the idea; **new** = nothing in the
code or the contract yet. "Where" names the package or document that
would own it.

## Principles

| Principle | State | Where it lives / would live |
| --- | --- | --- |
| Prove it | new | Needs a representation of claims and evidence (see Evidence-Based Review). Today a decision in `ContextDoc` has a text and a reason, nothing that says how it was checked. |
| Consensus is not verification | new | The three result states are not modelled. Candidate: flags on `domain.Decision` (`consensus`, `verified` with evidence references, `approved`), set by different actors and never promoted automatically. |
| Human in command | have, partly | Have: only the human posts, chooses modes, reveals, retries, cancels; agent context proposals wait for acceptance (`RevisionProposed`). Partly: tool approvals are not offered yet (`Capabilities.ToolApprovals=false`); roadmap step 3. |
| Independence by design | have, partly | Have: independent rounds freeze inputs, seal results, reveal transactionally, use soft isolation by flags (ADR 0004, ADR 0003). Partly: strict isolation waits for the canary experiment (docs/EXPERIMENTS.md §5). |
| Safety by architecture | have, partly | Have: transactions, event log, crash recovery without repeated paid calls, no credential handling, process supervision, race-tested core. Partly: backups and export are roadmap step 2/4. |

## Modes

| Mode | State | Notes |
| --- | --- | --- |
| Blind Review | have | `RoundIndependent` with `AskIndependent` in the composer. |
| Open Discussion | partly | `RoundJoint` runs participants once, in order, each seeing earlier answers; a real back-and-forth needs follow-up rounds started by the human. A bounded multi-turn joint round would be a new round option with an explicit turn limit (see "limits"). |
| Adversarial Review | partly | `Participant.Role` can instruct a critic, but roles are per participant, not per round. Candidate: round-level roles (proposer / critic) passed through `prompt.Material`, plus an "adversarial" entry in `AskMode`. |
| Moderator Mode | partly | Mentions and `ParticipateManual` decide who answers; frozen inputs decide what a round sees, but the same visible history goes to every participant. Per-participant visibility is new (next table). |
| Mode switching with shared history | have | Modes are chosen per message/round; the topic history is shared and immutable. |

## Capabilities

| Capability | State | Notes |
| --- | --- | --- |
| Disagreement Tracking | new | Candidate entity: a disagreement with positions per participant, arguments as message references, and a resolution; or tags on messages derived by the human. Needs a design decision before storage changes. |
| Explanation Verification | partly | `EvaluateRound` already runs an independent judgement over revealed answers; a verification task template (explain, then verify) would reuse it. |
| Evidence-Based Review | new | Candidate: typed references on claims (test run, document, reproduction) and a `verified` state scoped by what was checked. `domain.Reference` exists but carries only labels and message/artifact IDs. |
| Diff Review | new | Depends on artifacts (schema exists in `ArtifactVersion`, no operations) and a diff attachment format; roadmap step 2. |
| Context Visibility Control | new | Today visibility is binary (`visible`/`sealed`) and round-level. Per-participant visibility rules and isolated branches would change `storage.MessageQuery`, `prompt.Material` assembly and the UI; this is the largest change in the list and needs an ADR. |
| Anti-Sycophancy | partly | `prompt.Build` asks for brevity, not for criticism; `EvaluationPrompt` asks what is wrong or missing. Candidate: a critique requirement in evaluation and adversarial prompts, and a UI that shows disagreement explicitly rather than hiding it. |
| Human Approval | partly | Runs with `workspace_write` edit only inside the private run directory. Claude: shell commands that need approval are denied (`--permission-prompts none`); Codex: commands run inside its workspace-write sandbox without asking, because `exec` cannot ask. Approvals routed to the UI are roadmap step 3 (MCP permission-prompt tool or stream-json control protocol for Claude, app-server for Codex). |

## Constraints

| Constraint | State | Notes |
| --- | --- | --- |
| Local-first | have | SQLite in the user config dir, no server. |
| Keyboard-first | partly | Cmd/Ctrl+Enter sends; no other shortcuts yet. |
| Official CLIs with personal subscriptions | have | `subscription` profile; README states the policy caveat from Anthropic's Agent SDK terms. `api_key` profile is optional, never the default. |
| Extensible adapters | have | `runner.Runner` + `Capabilities`; a Gemini adapter would be a new package under `internal/runners` reporting its own capabilities. |
| No token extraction, no unofficial APIs | have | ADR 0003; adapters never read credentials. |
| Shared history, controlled isolation | partly | Round-level today; per-participant control is the "Context Visibility Control" item. |
| Limits on autonomous rounds | have, partly | No automatic loops exist: one round, one optional evaluation, both started by the human. Explicit per-topic budgets (max rounds, max estimated cost) are new. |
| Discussion separated from action; safe parallel editing | partly | Tool policy per participant separates discussion (`none`) from action (`workspace_write`); each run has a private directory. Parallel editing of a real repository needs per-run git worktrees and merge handling: new. |
| MIT on GitHub | have | |

## Open questions for the next discussion

1. Should "verified" be a property of a decision, of a message, or of a
   separate claim entity? The answer decides the storage schema.
2. Is a disagreement an entity the human creates, or a view derived from
   messages and evaluation rounds?
3. How far should per-participant visibility go: hidden branches only, or
   per-message rules? The first is cheap; the second changes prompt
   assembly and the UI.
4. Which limits are enforced by the orchestrator (rounds per topic, cost
   per round) versus shown as metrics?
