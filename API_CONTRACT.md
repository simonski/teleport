# Implemented API Contract

## Transport

HTTP protocol 0.1.0. Domain calls use `POST /api/tools/OPERATION` with a single JSON object, `Content-Type: application/json`, and a complete body limit of 1 MiB. Authentication uses `Authorization: Bearer TOKEN` or the HttpOnly SameSite=Strict session cookie. Browser origins must match the server. Sessions expire after 12 hours and are stored as token hashes. Production TLS termination must preserve trusted transport semantics; this phase binds localhost by default.

Responses use the operation's JSON result. Failures use `{"error":{"code":"stable_code","message":"human-readable explanation"}}` with 400 invalid input, 401 authentication, 403 authorization, 404 unsupported/missing, 409 version/state conflict, 413 payload limit, 415 media type or 429 sign-in throttling. Unexpected failures return a generic 500 and a server log entry. Unknown fields are rejected on typed requests. No API returns password hashes.

## Operations

| Operation | Input | Result | Authority |
| --- | --- | --- | --- |
| `login` | `{username:string,password:string}` | `{token:string,account:{id,username,administrator,suspended}}` plus cookie | Public; password verified; throttled |
| `getSession` | `{}` | `{account,installationId:string,revision:integer,settings:{agents,registrations,handbrake}}` | Current session |
| `logout` | `{}` | `{signedOut:true}` | Revokes current session only |
| `listWorkflows` | `{}` | `{items:Workflow[],nextCursor:""}` | Current session; two seeded definitions in P01 |

`Workflow` is `{id:string,version:integer,title:string,family:"human"|"agent",published:boolean,phases:Phase[],rules:Rule[]}`. `Phase` has string `id,title,role,instructions` and boolean `agentic`. `Rule` has string `id,source,outcome`, optional string `target,action`, integer `priority` and optional string-map `conditions`. The human definition's main path is Todo/Doing/Done; agent Doing displays as Implementation. The published seed versions are immutable.

Public discovery: `GET /healthz`, `/openapi.json`, `/api/actions`, `/skill.md`; browser assets `/`, `/app.js`, `/style.css`. No domain database access is available over the client surface. OpenAPI publishes input/result/error schemas for every registered operation; the exact request/result contract above is normative for this build.

## Projects and human tickets

`createProject` accepts `{name,key?,requestId}` and atomically returns a project, creator Maintainer membership and protected Unassigned goal/epic IDs. Keys are lowercase ASCII letters/digits/underscore/hyphen, 1–80 characters; names are 1–200 bytes. Stable creation `requestId` is mandatory (1–100 bytes), scoped to actor and operation; matching retries return the original result, conflicting reuse returns 409. `createTicket` accepts `{projectId,parentId?,kind?,title,description?,criteria?,agent?,priority?,requestId}`; omitted kind/intent/priority are task/agent/medium. Parent defaults are project-local; wrong types/projects/hidden immediate parents fail.

`listProjects` and `listTickets` return `{items,nextCursor}`. Default limit is 100, maximum 200; cursors encode the last ID and results are ordered ascending. Ticket filters: `projectId,kind,agent,status,query,includeHidden`. All pages enforce current project rights. `getTicket` takes `{id}`. `listTicketHistory` takes `{id,cursor?,limit?}` and returns immutable snapshots in ascending version order.

`updateTicket` takes `{id,expectedVersion}` and optional title/description/criteria/priority/labels/start/eta/due/progress fields. Omission preserves values; empty text/date clears where valid. Content changes require preexisting Todo; protected default titles cannot change. Dates use YYYY-MM-DD, ETA cannot precede start. Scalar agent progress is a report; human status uses explicit transitions in this phase. No-op edits create no revision.

`transitionTicket` accepts `{id,expectedVersion,target,confirm?}` for declared human routes. Done requires explicit `confirm:true` and records current criteria/actor/time/basis. Agent transitions refuse this generic route. Nonempty containers derive completion from non-deleted children; archived children still count. Percentage rounds to the nearest integer. Empty containers remain incomplete; epic open planning prevents Done. Every mutation reconciles its affected hierarchy within the same SQL transaction. Stale writes return 409 without partial changes. Current Contributor-or-higher rights are required to create/edit tickets; Viewers read only.

The generated OpenAPI contains complete concrete project/ticket/history and operation schemas. UI and CLI call the same tool routes. Alias names such as `createProject` and `listTicketHistory` map to the corresponding canonical specification actions.

## Persistence representation

## Work controls

`setDependencies` takes `{id,expectedVersion,dependencies:integer[]}` on unclaimed Todo tasks/epics, up to 200 direct IDs. New references must be unique same-project non-cancelled tasks/epics. Existing cancelled prerequisites remain blockers. Validate effective inherited prerequisites plus hierarchy-completion edges as one acyclic dependency graph; workflow cycles remain valid and separate. Human Doing/Done requires completed prerequisites.

`cancelTicket` takes `{id,expectedVersion,reason?}`. Tasks cancel individually; containers cancel themselves and unfinished descendants, including hidden work, with one correlation ID. Done/already-cancelled descendants remain intact. `reopenTicket` takes `{id,expectedVersion}` and affects only the selected record. Agent tasks and containers require Maintainer; human tasks require Contributor. Agent reopening is inactive Todo with retained history and a resume hold. Containers recompute current child completion.

`setTicketVisibility` takes `{id,expectedVersion,archived?,deleted?}` and requires Maintainer. Change only the selected record, preserve children and reject protected defaults or pending execution. Restore retains status and adds an agent resume hold. `transferTicket` takes `{id,expectedVersion,destinationProjectId,parentId?}` and requires current Maintainer authority in both projects, or installation administrator. Move the complete subtree including hidden descendants atomically, preserving IDs/history/pins; reject protected/busy records, incompatible assignees, invalid parents or resulting cross-project prerequisites. Omitted parents use destination defaults. `moveTicket` takes `{id,expectedVersion,parentId}` for same-project valid reparenting with Contributor rights.

`changePassword` takes `{currentPassword,newPassword}`, verifies the current password, requires 12–1,024 new-password bytes and revokes all account sign-in sessions atomically. It returns `{signedOut:true}` and requires a fresh sign-in. The combined visibility and container-cancellation routes compose their corresponding permanent action variants; API discovery names the primary action.

## Storage

## Workflow definitions

`listWorkflows` accepts `{projectId?,cursor?,limit?}` and returns authorized definitions ordered by exact key; its cursor is the last opaque workflow key. `getWorkflow` accepts `{workflowKey,ticketId?}`. Optional ticket context grants access only to that currently authorized ticket's exact retained pin, including after transfer. Published keys are `ID@VERSION`; mutable drafts use `ID@draft`. Definitions add `projectId,draftRevision,layout?` to the seed shape; layout maps phase IDs to bounded `{x,y}` coordinates.

`copyWorkflow` takes `{sourceKey,projectId,title,requestId}` and creates a new draft identity. `draftWorkflow` takes `{workflowKey}` and idempotently starts the next version of that same definition. `saveWorkflow` takes `{definition,expectedRevision}`, preserving identity/family/ownership and rejecting stale writes. `deleteWorkflow` takes `{workflowKey,expectedRevision}` for drafts only; published history remains retained. Maintainer authority controls project definitions; installation drafts require administrator authority. Save validates the whole graph, including disconnected branches, exact destinations, manual role gates and overlapping conditions. Declared cycles and self-retries are valid.

Rules match source/outcome and exact string conditions on `reason,revisionKind,questionOrigin,result`. Lower numeric priority wins (0–10,000); potentially overlapping matches at equal priority fail validation. Choose exactly one target or `wait`/`halt` action. `previewWorkflowOutcome` takes `{workflowKey,source,outcome,context?}` and returns `{ruleId,target?,action?,needsAttempt,requiredGates}` without state changes. An unmatched route returns 409 with a routing-specific code.

`activateWorkflowVersion` takes `{workflowKey,expectedRevision,projectId,expectedProjectVersion}`. It validates complete agent/human routing, publishes an immutable version if needed and updates only that project's selection for new tickets. Existing pins remain unchanged. Installation-wide future-project default selection is not shipped yet. `previewWorkflowMigration`/`migrateWorkflow` take `{sourceKey,targetKey,tickets:[{id,expectedVersion}],mapping:{sourcePhaseId:targetPhaseId}}`. Migration requires current Maintainer rights, stopped/unclaimed work, published same-family definitions and exact same-role mappings; the complete batch commits or fails together. No automatic queueing or acceptance occurs. Full agent continuation/migration guards depend on the later execution phase.

New tickets use a unique Todo phase in their selected pin, or explicit `phaseId` when required. Human transitions accept exact target IDs (or a uniquely resolved semantic role), with optional explicit `outcome,context` for configured rules. Display titles never grant authority.

## Installation aggregate

Schema 1 stores a versioned installation aggregate transactionally in the selected SQL backend. A serialized server owns mutations and snapshots; PostgreSQL additionally locks the row. SQLite uses WAL and an exclusive OS installation lock; PostgreSQL uses a connection-held advisory lock. Unsupported schema versions refuse operation. The aggregate is an initial local implementation, not a claim of large-workspace performance. Clients do not import the store. This representation may migrate behind stable domain contracts as capacity evidence requires.
