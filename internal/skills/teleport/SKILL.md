---
name: teleport
description: Use the tk CLI to inspect and manage teleport projects, Goal/Epic/Task tickets, workflows, questions and supervised agent delivery through the authenticated server API.
metadata:
  version: "2"
---

# Teleport through `tk`

Use this skill when the task involves an existing teleport installation. Work within the user's authorized scope and the current actor's permissions. Skill instructions do not supply human approval or authorize model spending, resets, publication or deployment.

## Discover the installation

`tk skill` prints this standalone skill from the binary, without a server or login. Save it as a harness skill's `SKILL.md` when requested. Bare `tk` or `tk help` lists top-level commands; `tk NOUN` or `tk help NOUN` lists its actions. `tk commands` lists the full offline catalogue and `tk commands -json` provides it as JSON. Command-specific help (`tk agent ls -h`) lists typed field flags from the bundled request schemas. `tk tools` and `tk spec` describe the running server, which may differ from the local build.

```sh
tk version
tk help
tk commands -json
tk help agent
tk tools
tk spec
tk getSession
tk listProjects -data '{"limit":50}'
```

`tk tools` returns registered operation names and summaries. `tk spec` returns OpenAPI request/response schemas, required fields and limits. Consult them before using an unfamiliar operation; do not invent commands or assume an operation from another version exists.

Noun/action commands wrap existing API operations: `tk project ls`, `tk ticket show ID`, `tk workflow ls`, `tk idea ls` and `tk agent ls`. Agent listing reads accessible harness sessions; it neither launches agents nor claims that a standalone remote agent daemon exists. `list/create/get/update/grep` are aliases for `ls/new/show/edit/search` where those actions exist. Use `-project`, `-id`, `-version`, `-limit`, `-cursor` and other discovered field flags; IDs may be positional for commands whose schema has an `id`. Do not combine conflicting JSON and field selectors. `-data JSON` and `-f FILE`/`-f -` remain available; complex fields take JSON and sensitive fields use file/stdin input. Raw `tk OPERATION -data JSON` and `tk api OPERATION -data JSON` remain supported.

`TELEPORT_SERVER` selects the server (default `http://localhost:9999`); `-server URL` overrides it per call. `TELEPORT_CONFIG` or `-config PATH` selects the private client credential file. Use the supplied configuration and identity; never print tokens or switch to a more privileged account to bypass a refusal. If sign-in is required, `tk login` prompts for a username and a hidden password on a terminal. For unattended use, provide `TELEPORT_USERNAME` and `TELEPORT_PASSWORD` through the authorized secret-input mechanism, or use `tk login -username NAME -password-stdin`; never put a password in command arguments.

An unavailable server is a blocker for API operations. Do not open its PostgreSQL database as a fallback. `tk init` initializes configured storage and `tk serve` runs the server; neither is an implicit repair step. Forced initialization is unsupported. Offline snapshot export/restore is administrator maintenance and requires explicit authorization.

## Read the exact work before changing it

The hierarchy is **Goal → Epic → Task**. A Task belongs to an Epic; an Epic belongs to a Goal, all in the same real project. Protected Unassigned records supply valid defaults. Do not fabricate IDs, edit protected defaults, assign work to the All projects view or assume IDs identify the same records on another installation.

```sh
tk listTickets -data '{"projectId":1,"kind":"task","limit":50}'
tk getTicket -data '{"id":123}'
tk listTicketHistory -data '{"id":123,"limit":50}'
tk listWorkflows
tk getTicketAgreements -data '{"id":123}'
```

Replace example IDs with values read from this installation. Lists return `items` and `nextCursor`; request subsequent pages with the returned `cursor` and the same filters when more results are needed. Project access, hidden records and actor scope affect results; an empty page does not grant access to other projects.

Read the ticket's original request, criteria, parent/project context, pinned workflow, refinement, readiness basis and current execution state. Goal/container progress is derived from children; do not write it manually. A completed delivery rollup does not prove the Goal's business outcome.

## Make bounded, versioned changes

Call registered operations as `tk NOUN ACTION [flags]` or `tk OPERATION -data JSON`. Successful API calls print human-readable output by default; pass `-json` for pretty-printed JSON. Failures print diagnostics to stderr and return a nonzero exit status. Build JSON as data with proper escaping rather than interpolating untrusted text into shell commands.

Use the record's current `version` as `expectedVersion` where the schema requires it. On a version conflict, re-read the record and reconcile the requested change; do not blindly replace another actor's work or loop retries. On authentication or permission errors, retain the failure and resolve the actual identity/access issue.

For creation and other operations with `requestId`, use a unique ID for that logical request. Noun/action commands generate a required request ID when omitted and print it on stderr; pass `-request-id` to repeat that exact request after reconciling an uncertain response. Raw API calls require the ID in their JSON. Reuse the ID only for the same request, never distinct work. Re-read state before replaying any mutation whose result is uncertain.

For an authorized new agent Task, the request shape is:

```sh
tk createTicket -data '{"projectId":1,"kind":"task","title":"Describe the requested change","description":"Bound the work","criteria":"State observable acceptance","agent":true,"requestId":"REPLACE-WITH-A-UNIQUE-REQUEST-ID"}'
```

Choose human or agent intent from the actual request. Omit `parentId` only when the project's protected Unassigned parent is appropriate. Agent work has no human execution assignee. Original title, description and criteria are editable only while the ticket is already Todo; returning to Todo is a separate allowed operation, not a way to discard an implementation awaiting review.

`tk project rm NAME` is a destructive, administrator-only operation. Use it only when removal has been explicitly requested. It previews affected records and retained artifacts and requires the exact project name for confirmation. Active work and foreign references block removal; never bypass a refusal.

## Capture freeform text

`tk import -project NAME FILE` captures a regular UTF-8 text file as one Idea. `tk import -project NAME` or `tk import -project NAME -i` reads multiline stdin until EOF (Ctrl-D in a terminal). Preserve the text exactly, up to the current 20,000-byte Idea limit; oversized or invalid text is rejected without saving. The required project name must match an accessible project. `-title` overrides the file basename or first nonblank input line. Stdin capture verifies authentication before consuming a long paste.

Capture does not classify, create goals or start a harness. The classifier/refinement session is separate later work. This is not the structured Copy/Update interchange importer. Human output reports the captured Idea; `-json` emits the full Idea with diagnostics and the retry request ID on stderr. After an uncertain response, reconcile the Idea/request identity before repeating with the same `-request-id`.

## Preserve supervision and execution ownership

Follow the ticket's pinned directed workflow, including declared backward routes and finite attempt budgets. A phase's display label is not permission to skip readiness, dependencies, Review or human Approval. Agent progress of 100% never completes agent work.

Human scope/readiness/code decisions remain human decisions. Do not invoke `acceptRefinement`, `markReady` or `approveTicket` as a substitute for the required human assessment, even when credentials technically permit the call. Present the exact current proposal/basis or code candidate and evidence for the appropriate human gate. Do not raise budgets, clear handbrakes, change execution mode or weaken workflow/native permissions merely to make blocked work run.

Read structured questions and their exact IDs before responding. `answerQuestion` uses the ticket ID, current version, `questionId` and answer; answering a ticket question does not send terminal input. Native harness permission prompts retain their own authority and are not answered by a ticket response.

Independent local consoles run in the selected project root; ticket execution uses its owned worktree. Use existing owned sessions and worktrees. Idle output, a stopped browser viewer or an existing directory does not prove process exit. Unknown/stopping ownership blocks replacement until reconciled; never start a duplicate harness to work around uncertainty. Read-only Review must stay read-only. `getPrompt` can inspect current/saved managed prompts only when the current actor is permitted; a saved prompt belongs to its exact session, and a current preview does not prove what a past session received.

When explicitly tasked with implementation, use the authorized codebase/worktree, make the agreed change and run relevant checks. Capture real delivery through the discovered `prepareTaskWorktree`/`recordTaskDelivery` contracts as applicable. Report the exact candidate, validation and unresolved blockers; retain the human code-approval gate before integration. Accepting code does not itself authorize restarting, rebuilding or deploying an application.
