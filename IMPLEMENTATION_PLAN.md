# Implementation Plan

## Delivery and branch policy

Deliver usable vertical phases from the agreed specification. Start each phase from clean `main` on its named branch. Commit related changes in order, verify the complete phase, then merge with `git merge --ff-only`. Rebase and repeat checks if `main` changes; never create merge commits or rewrite an already merged phase. Keep phase branches as an audit trail. No push, release publication, deployment or paid model execution is included.

## Phases

| Phase / branch | Depends | Demo and acceptance boundary | Status |
| --- | --- | --- | --- |
| P01 `phase/01-local-bootstrap` | — | Build one Go binary; explicit init/login/serve, seeded manual/agent workflows, persistent local credentials and API-only CLI. Repeat init preserves identity. A32/A33/A34/A165. | Pending |
| P02 `phase/02-human-tickets` | P01 | Browser/CLI project creation with protected Unassigned hierarchy; human Todo/Doing/Done, immutable history, versions and rollups. A01–A05/A09/A11/A69. | Pending |
| P03 `phase/03-work-controls` | P02 | Dependencies, cancellation/reopen, visibility and dual-owner subtree transfers, enforced in API and UI. A10/A25/A31/A63–A68/A147–A150. | Pending |
| P04 `phase/04-workflow-editor` | P02 | Visual/list workflow editing, outcome conditions and backward routes, read-only preview, explicit immutable activation and new-work pins. A40/A97/A102–A105/A163/A164. | Pending |
| P05 `phase/05-agent-orchestration` | P03/P04 | Deterministic adapter proves refinement acceptance, phase attempts/claims, question continuation, separate Review and human Approval guards; no invented delivery evidence. A07/A60/A61/A99/A166. | Pending |
| P06 `phase/06-git-delivery` | P05 | Retained owned worktrees, pinned candidates, actual checks, rebase/review and serialized human-approved fast-forward integration. A11/A35/A78. | Pending |
| P07 `phase/07-codex-execution` | P06 | Local Codex capability spike, scoped credentials, verified owned-process recovery, safe tool-result continuation and command-policy enforcement. Live paid evidence remains separately authorized. A12/A28/A85–A88/A154. | Pending |
| P08 `phase/08-terminal-console` | P07 | PTY viewers, private independent console, explicit interrupts and existing-session multiplexing. A12/A21/A80–A82/A127/A158. | Pending |
| P09 `phase/09-planning-views` | P03 | Home activity/upcoming work, Ideas, 3D Graph, goal reviews, finder/calendar and synchronized appearance. A22/A23/A89–A96/A142–A145/A151/A152/A157. | Pending |
| P10 `phase/10-postgres-parity` | P05 | Run the same domain/concurrency suite against PostgreSQL; document tested migration and maintenance boundaries. A30/A32/A34. | Pending |
| P11 `phase/11-multi-user-hosts` | P07/P10 | Verified per-project isolation, host grants/revocation, guarded registration, memberships and remote-host transport. A71–A82/A130–A140. | Pending |
| P12 `phase/12-auth-clients-release` | P08/P11 | Google/passkeys/OAuth clients, TUI, isolated JavaScript jobs, packaging, documented compatibility and assembled integration evidence. A121–A124/A153/A161/A162. | Pending |

## Proof strategy

Each phase freezes the affected data/API/event contracts before implementation and adds behavior tests through public services or HTTP. Run unit/integration/race/vet checks, documentation verification and an actual CLI/browser path for shipped UI. Fake adapters make zero paid calls and are labelled; they do not prove Codex or Git acceptance. PostgreSQL claims require a real temporary PostgreSQL instance. Keep failed/unknown process and delivery states honest. Record exact commands, results, remaining scope and commit evidence below as work progresses.

The hardest boundaries are process ownership/recovery, human acceptance of exact Git candidates, command-policy enforcement, database concurrency and isolation. Stop the dependent feature when its proof is unavailable; continue independent phases. Parked analysis-budget and price-publication decisions remain parked.

## Engineering contracts and boundaries

P01 supplies durable installation/authentication and bounded authenticated tool calls. P02 supplies project membership, stable hierarchy IDs, versioned mutations and immutable revisions. P03 supplies admission/dependency/transfer guards. P04 supplies pinned workflow definitions and deterministic routing. P05 consumes those contracts to reserve stage visits/attempts without bypassing human gates. P06 adds real Git evidence. P07/P08 add verified process/PTY authority. P10 proves backend parity; P11 adds isolation before enabling multi-user execution. P12 integrates clients and packaging without changing domain policy.

## Execution evidence

Initial repository: Git initialized on `main`, no commits, three planning documents and two documentation tools. Application implementation starts after the documentation baseline commit. Deferred capabilities must remain explicit in API discovery and the usage guide rather than appearing as functioning controls.
