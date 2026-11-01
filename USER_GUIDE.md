# Usage Guide

## Local setup

Building requires Go 1.26.1+, a C compiler with CGO enabled, Python 3 and Make. Node.js is used for development browser checks. The installed executable includes the JavaScript engine.

Run `make build`, then `bin/tk init` and `bin/tk serve`. SQLite defaults to `~/.teleport/database.db`; `--db PATH` or `TELEPORT_DB` selects an isolated installation. The local server defaults to `localhost:9999`; `serve --addr HOST:PORT` overrides it. Repeated init preserves identity and credentials, refuses a running installation, and starts nothing.

Sign in through the browser with initial `admin` / `password`. CLI login reads the password from standard input: `bin/tk login --password-stdin`. `TELEPORT_SERVER` selects the server; `TELEPORT_CONFIG` selects the private mode-0600 client credential file. All CLI operations except init and server hosting use HTTP APIs. `bin/tk logout` revokes that sign-in session.

The landing rocket travels behind and in front of the wordmark, loops around the moon and returns. Reduced-motion preferences show a static rocket. View content starts beneath navigation without a duplicate workspace/project breadcrumb.

First browser sign-in opens Welcome to teleport: name/select a project, choose Codex or configure later, then select attention preferences. Names are checked while typing; the suggested prefix uses four letters, or three plus an available number on collision. Progress stays private until Finish setup atomically creates the project and completes setup. Escape, outside click or Later dismisses; Finish setup in the account menu resumes it. Repository/worktree paths and access grants are configured separately, after setup. Finishing starts no harness or paid work. Missing Codex installation is shown without claiming readiness. Attention preferences affect notifications; ticket badges remain visible.

## Support boundary

The compact header has permanent JavaScript and harness-session buttons; opening either viewer starts no execution. The harness button turns red when an accessible running session needs attention. The bottom-right footer shows the actual server build on login and in every view. The profile menu opens personal Settings and separate installation System popups over the current view. Choose Project settings for the selected project's repository, handbrake, native readiness and shortcut to Workflows. Escape/outside dismisses popups without changing the underlying view. Evolution remains a subsequent slice.

Popups blur the background and contain application keys and Tab/Shift+Tab focus. Typing and local controls keep their normal behavior; underlying shortcuts stay inactive. Escape closes a nested dropdown first, then its dialog. Completed outside clicks also dismiss dialogs, restoring the opener.

Focused interface checks use `python3 -B tools/smoke.py --browser --shell-only`; add PostgreSQL parity with `python3 -B tools/postgres_check.py --browser --shell-only`. Both use disposable installations.

Drag tabs to reorder them, or focus a tab and press Alt+Shift+Arrow. Settings → Navigation selects top, left or right placement; side navigation can collapse to icons. Changes save privately to the account and survive reload. Reset navigation restores the default order and top placement. Updates from another browser wait while an editor or terminal owns input. Q/Shift+Q follows the saved order. The project selector shows the current selection and a separate New project section; cancelling its dialog creates nothing.

Projects creates projects and their Unassigned hierarchy. Card view and List view show all accessible projects, independently of the project selector. List view includes Description, Tickets, Epics, Goals, Tokens, Last active and Agents. Counts exclude hidden/protected tickets; Epics and Goals are subsets of Tickets. Tokens stays Unavailable until metering exists. Agents counts only this account's accessible, runtime-verified running sessions; unverifiable activity stays Unavailable. Last active uses retained ticket changes and authorized session start/exit events; full lifecycle reporting remains pending. Maintainers can use Edit to change a project's name/description with version checks. Refresh reloads totals; narrow screens scroll within the table.

New and N preselect Goal in Goals, Epic in Epics and Task elsewhere; Todos and Board's Human lens preselect human intent. The type and intent remain editable. Click a ticket to edit it or inspect history. Original content changes require Todo; Done asks for explicit completion acceptance. Todos has exactly Todo/Doing/Done; Board defaults to agent intent and offers Human/Both lenses. In Board/Todos, arrows move ticket focus within/across nonempty lanes and E opens the focused ticket. These keys preserve editor and terminal input. Home shows actual latest/upcoming work. Q/Shift+Q switches views, and Escape dismisses the current dialog without applying changes. Browser changes poll committed state while preserving open editors and ticket focus.

Right-click a Board/Todos ticket or Graph ticket node/list entry, or focus it and press Shift+F10, to open its CSS menu. Open ticket uses the current editor; Questions appears for pending questions. Teleport appears only for the exact running attached session and reconnects without sending input or starting another harness. Arrows/Home/End navigate the menu, Enter selects, and Escape restores source focus. Outside interaction or Tab dismisses. Complete Prompt inspection remains pending; terminal input is not retained as a launch-prompt snapshot.

For example, `bin/tk createProject --data '{"name":"Flight","key":"flight","requestId":"create-flight-1"}'`, then `bin/tk createTicket --data '{"projectId":1,"title":"Pack case","agent":false,"requestId":"pack-1"}'`. Supply the current `expectedVersion` for edits/transitions. `bin/tk listTickets --data '{"projectId":1,"limit":100}'` reads a bounded page. `tk spec` publishes all shipped schemas.

This is an incremental local implementation. Agents and registrations start disabled. Only operations listed by `tk tools` are shipped; unsupported actions fail explicitly. Published human and agent workflow definitions are seeded. Local PTY sessions, the Codex interpreter and bounded script automation are available through explicit launch; live provider acceptance and automatic native workflow execution remain pending. Remote hosts, multi-user activation, external sign-in, command shorthands and release publication remain future work. Private project-draft autosave and broad drag/layout controls remain pending. Never use demo evidence as proof of real code delivery.

Ticket details offer prerequisites, cancellation and permitted reopen/transfer/archive/delete controls. Visibility changes affect only that record; Project settings → Hidden tickets provides restoration. Container cancellation affects unfinished descendants, while container reopening affects only the selection. Transfers require ownership (Maintainer) in both projects and refuse cross-project prerequisite links. Settings → Change password verifies the current password and signs out every session; repeat init never resets it.

Workflows hosts the graph and list editor in its own view; Project settings → Workflow editor is a shortcut. Choose an immutable v-N publication or editable latest draft. New from this workflow copies into a project-owned draft; edit phase titles, descriptions, roles, instructions, agentic settings and outcome rules in the lists. Agentic Workers accepts 0–20: blank inherits the shared project pool, zero pauses new admission, and a positive quota cannot increase project capacity. Unknown/stopping ownership retains its reservation. Manual phases have no worker setting. Drag a phase grip or use its up/down buttons to reorder lanes; Alt+ArrowUp/Down on the grip also works. Escape cancels an unfinished drag. Save latest persists the order without changing routes or existing ticket pins. Canvas node dragging saves graph layout only; model/harness advice remains pending. Save latest persists changes; Discard changes reloads the saved draft. Save/discard before changing view/project. Delete latest explicitly deletes only the draft; cancelling its popup preserves unsaved input.

Save latest before previewing a saved route or choosing Publish latest. Publication freezes the next numbered version without selecting it. Retry an unconfirmed publication response using the same action; it returns the original version. Select for new work separately changes future tickets; existing Board sections retain their exact pin. Edit next version starts a new latest draft. Settings can make Workflows the sign-in default. The CLI also exposes stopped same-role migration and retains its older combined activation operation.

## JavaScript console

Press `/` outside text entry to open the right-side console. Choose Script or Expression, enter source and press Run. Script mode supports variables, functions, loops, `await` and an explicit `return`; Expression mode returns the expression value. Tab completes a matching operation name inside `tk("…`. Available bindings appear in the console and `tk getScriptBindings`.

```javascript
const projects = await tk("listProjects", {});
console.log(projects.items.map(project => project.name));
return projects.items.length;
```

Runs receive stable `J1` IDs and private source/results. Escape, outside click and Detach close the viewer while the job continues. Reopen and select its history entry to reconnect. Logout also detaches; sign in again to inspect or confirm. Source edits prepare a new Run and do not rewrite an existing job.

Scope acceptance, code decisions and human Done pause for an exact-action confirmation with current ticket/version context. Script code cannot confirm itself. Declining affects only that script action. Stop terminates the interpreter while preserving committed API effects and receipts. Failed or uncertain dispatch is never automatically repeated as a canonical mutation.

Execution uses a bounded embedded engine without browser, shell, filesystem, database or arbitrary network bindings. The initial safe API subset is explicit; unsupported commands fail. Limits include 120 seconds, 64 MiB JavaScript heap, 128 calls and four live jobs. Source/results remain initiating-account private even from other administrators; administrators may inspect operational metadata and Stop. Expired authority, account suspension and unresolved process ownership prevent further dispatch.

## Local harnesses and demo workflow

Local terminals run through an owned daemon on macOS/Linux. The verified fixture can be enabled explicitly with `tk serve --enable-terminal-fixture --runtime-dir /absolute/private/path`; it calls no model. Grant a project repository, press backtick and choose the fixture. A free console gets a separate owned Git worktree without a ticket claim. Robot icons show stable `H1`, `H2` IDs in the header and on attached Board tickets; the session switcher lists other retained instances.

The terminal displays actual merged PTY output and accepts keyboard input. Escape, outside click or Detach closes only the viewer; the process and retained work survive. Reconnect through its robot icon, including after an API-server restart. Send Esc deliberately sends a native Escape. Ctrl/Cmd+C copies selection; otherwise an application dialog asks before sending an interrupt. Stop requires confirmation and reports stopping until the owned process group actually exits. Direct ticket-terminal input records actor/time/operation/delivery metadata without raw keystrokes. A missing daemon or unproven child exit stays unknown and blocks replacement. `tk shutdownLocalRuntime --data '{"confirm":true}'` stops an idle runtime only.

With `codex-cli 0.166.0` installed, backtick also offers Codex in trusted local mode. Native permissions and existing native configuration govern OS commands. Teleport launches one owned native engine/native-terminal pair; terminal input may start paid model work. No session starts during build, sign-in or Welcome setup. A prepared Doing task offers Start local harness in its detail dialog; a free console stays separate from ticket claims.

Structured native questions and simple permission decisions appear in ticket details and turn the matching robot heads red. Complete every question before sending; declared secret answers are redacted from retained Q&A. Extended native policy choices remain in the terminal. Attention notification preferences from Welcome control toasts; badges remain visible. Closing the viewer never answers a question. Stop waits for native cleanup and owned exit; unresolved cleanup stays unknown and blocks replacement.

Zero-provider protocol fixtures establish transport/lifecycle behavior rather than live provider or full automated-workflow compatibility. Terminal replay keeps up to 2 MiB with explicit gaps; question history retains up to 128 recent requests with a visible gap when older settled entries expire. No complete transcript or exact screen recovery is claimed after lost output/daemon ownership.

`tk getSessionTurn --data '{"id":"H1"}'` reads the latest structured native turn and any failure reason for the initiating account with current execution rights. It records exact thread/turn IDs and bounded final assistant text, with explicit unknown/truncated states. It is separate from terminal replay and does not approve or advance a ticket. Missing/failed output is never inferred from a terminal echo. Diagnostics retain finite error codes and observed engine exit information without raw prompts or credentials. Live provider acceptance and automatic workflow consumption remain pending.

System → Execution → Demo enables automatic deterministic outcomes with no paid calls or source writes. Create an agent Todo ticket, open it and request refinement. Accept the displayed proposal explicitly; the demo proceeds through Implementation and read-only Review to Approval. Its evidence is labelled simulated and cannot complete real agent work. Human ticket completion remains independently available in Todos. Revision buttons retain scope or return to Todo for scope changes.

Demo questions retain their origin and consumed attempt. Answers automatically continue eligible demo work; pause/resume, handbrakes, prerequisites and finite attempt limits remain enforced. Project handbrake controls appear in Settings for Maintainers. The profile dropdown lists active attention, and Board attention cards precede ordinary cards. Demo stage events and question/proposal history remain inspectable. Native sessions use native OS permissions; automated native workflow outcome/Review handoff remains unimplemented.

## Graph, appearance and Cost

Graph shows each project/ticket once, with hierarchy/dependency edges and dashed required context. Drag the scene to orbit, right-drag to pan and wheel to zoom. Select nodes through the scene or entity list; Enter opens work, Alt-arrows arrange, and Alt-Page Up/Down changes depth. Shift-drag or edit world coordinates, then explicitly Save arrangement. Project arrangements are shared and require Contributor rights; All projects has a separate personal arrangement. A concurrent layout conflict retains local edits. Camera/selection remain viewer-local; arranging never reparents or revises a ticket. The entity list and coordinate controls remain available without WebGL. Oversized graphs require a narrower project/type/search selection. Assignment gestures and richer graph context menus remain pending.

Open Settings through the account icon. Paper/Instrument styles, Light/Dark mode and sign-in defaults save automatically; failures retain edits with an inline Retry action. Appearance changes show no success toast. Preferences synchronize without redirecting another open client or replacing foreground edits. Sign-out clears actor-bound presentation state. Cost currently explains the absence of observed provider usage; it reports no invented token or money total.

## Calendar and Finder

Edit a ticket's Start, ETA and Due dates, then open Calendar. Spans include both endpoints; containers show derived bounds without changing their saved dates. Default includes the previous week, current week and six following weeks. Week/month/quarter/year/full and due-date presets use calendar boundaries. Deadlines remain visible even when an earlier planned span is outside the window. Context ancestors retain hierarchy, and unscheduled work is counted separately. Custom colours and richer epic context menus remain pending.

Tap Shift twice outside text/foreground controls. Search a numeric ID (optional #) or all words, optionally scoped to a project. Exact ID/title and title prefixes rank ahead of metadata matches. Arrow keys choose a result; Enter opens it through normal ticket permissions. Preview is read-only, and More matches reads the next bounded page. Finder does not create or start work. Project create/edit commands and contextual creation shortcuts remain pending. Sign-out/account changes clear filter, record and layout caches.

Dialogs use application styling. Escape or an outside click dismisses without submitting, including dirty ticket editors. Workflow confirmations close while preserving the underlying draft. Dropdowns support arrows, Home/End, type-ahead and Escape; checkboxes retain labelled keyboard/form behavior with CSS appearance. Date fields accept YYYY-MM-DD without opening an OS date picker.

## Goals and outcome reviews

Goals and Epics use the same hierarchy IDs and child-based delivery rollups as Board/Graph. Open a goal to configure up to 20 success measures while it is Todo; cadence can be monthly or quarterly. Record human review freezes the current goal/objectives, direct-child versions and delivery snapshot, alongside observed values, evidence notes, sources, as-of dates and an explicit human assessment. Missing evidence remains visible; Done never implies the objective was achieved.

Reviews are immutable and remain unchanged when later work reopens or objective criteria change. The next review adds one/three calendar months with month-end clamping. Reviews do not change delivery status or start follow-up work. Periodic reviews are on demand; background reminders are not yet implemented.

## Ideas

Ideas keeps planning thoughts separate from tickets. Create a private inbox Idea under All projects, or a project Idea in the selected scope. Private inbox content is visible only to its creator and installation administrators. Project placement shares retained content/history under that project's permissions; Change placement previews the access change and requires explicit confirmation.

Draft work proposal saves an immutable human proposal without creating tickets. Review acceptance shows exact proposed work; acceptance creates normal Todo tickets or links existing records atomically and retains the original Idea. Linked work does not inherit readiness or code approval. Existing work keeps its status/content. History and authorized links preserve the accepted versions. Archive/delete/restore affects only the Idea, with source-owner/Maintainer controls. Agent curation remains unavailable until verified live execution; CLI/API proposals support up to 20 targets, while the browser creates or links one per proposal.

## Local Git delivery

Select a project, then open Project settings from the account menu and grant its exact local repository and separate worktree storage directory as administrator. Use a committed `main` checkout. Turn demo execution off in System before real delivery. After accepting an agent task's refinement, open Ready work and choose Prepare owned worktree. Commit external changes on the displayed task branch, then choose Capture committed delivery. The controller reads the actual commit/diff and runs Git whitespace checks; separately supplied check reports do not become controller evidence.

Human Approval integrates the exact eligible candidate with a fast-forward and records Done only after integration. The default workflow requires real agent Review, which remains unavailable until the live adapter is verified. An explicitly configured workflow without Review can exercise external delivery and human Approval; demo Review never satisfies the default gate. Stale main/conflicts preserve the branch and refuse integration. Automatic Rebase, merged-worktree cleanup and full crash recovery remain pending. Unknown command exit blocks replacement; known-exit operations offer read-only reconciliation. Reconfigured readiness must be explicitly re-agreed against its displayed current basis.

## Development

Select a project, open Project settings and choose Inspect native execution readiness to see the exact missing adapter proofs. CLI `tk getEffectivePermission --data '{"projectId":1,"adapter":"codex-structured","command":["git","status"]}'` performs the same read-only inspection. It does not authorize or execute the inspected command. `python3 -B tools/codex_probe.py --output /tmp/teleport-codex-capabilities.json` exports installed protocol schemas for development without starting an agent session or using provider credits.

SQLite is the default. For PostgreSQL, provide an existing dedicated database URL through `TELEPORT_DB`, then run the same `tk init` and `tk serve` commands. Only init creates the application table; server/client startup never resets it. Keep database credentials in the environment rather than shell history, and configure transport/authentication for the intended host. The server holds exclusive installation ownership; loss of its retained PostgreSQL connection requires a controlled restart. Backend conversion, backup/restore commands, automatic schema upgrades and large-workspace performance remain pending.

`python3 -B tools/postgres_check.py --browser` creates a private disposable PostgreSQL cluster, runs the same race/domain suite with one fresh database per fixture, exercises the real binary/browser flow, and stops/removes the cluster. PostgreSQL 17+ server utilities must be on PATH or supplied with `--postgres-bin`; Postgres.app is also discovered. The runner opens no TCP listener and modifies no existing service. PostgreSQL 17.7 is the current parity target.

`make check` verifies documentation, Go formatting/vet, race tests, browser JavaScript syntax, delayed-response regressions and bundled Three.js hashes. Node.js is required for development checks; the built application needs no Node runtime or CDN. `make build` increments only the ignored local build counter; checks do not. `TELEPORT_BUILD_VERSION` provides an explicit reproducible build version. `make install` installs the built binary. No command auto-starts a service or browser.
