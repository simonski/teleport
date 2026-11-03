make check is killing my laptop performance
    stop running it for now a
    then optimise what it is/what it is doing

-------
the looping graph; the DAG
    the workflow is a dag
        each state in it is an explanation of what'shappening


using the tk skill, create a story per area

-------------------
MVP
-------------------
Bootstrap until the application is building itself.
Phases
    1.  MVP
            Create todo
            Implement code.
        REQUIRES
            workflow
            pty
            tk serve
                tk agent


        OUT OF SCOPE
            waitlist
            email
            user inspection
            claude, copilot, ollama



-------------------
animation
-------------------
    v1: introuce a launch vehicle which transports the rocket to the site, then continue the animation as-is
        when the rocket lands, have another vehicle turn up and plug-in, fill up the tank, then detach and move back a bit, then take off
    v2: occasionally instead of refilling, have a piece of the rocket fall off; then a different launch vehicle takes it away, a new rocket - perhaps a larger one or slightly diferent shape arrives and it repeats.  make half a dozen different rocket shapes including a big starship.  perhaps some have fins which shoudl animate when it is attached to the launchpad before closing for takeoff.

-------------------
import and classify
-------------------
    the abiltity to import random freeform thoughts and ideate them towards goals and identifty existing epics and stories

human todos
    todo
    doing
    done

agentic todos
    todo -> refine -> breakdown -> refine -> loop until -> ready

    implement -> receive refined and ready work  -> review -> approve -> move to integrate
        crank otu until complete and then review           -> reject. -> go back to refine and/or provide input

-------------------
workflow steps
-------------------

create new workflow steps

todo:
    human freeform ideation
    unclassified (what goal, what epic, what scale?)
    unassigned
    complexity undeclared
    unprioristised
    inconsistent

    can assign and classiffy here
    can have an agent spellcheck, classify and assign
        pre-refinement
    can have an agent refine and interview

intent:
    agentic step attempts to udnerstand the 'what' of the todo
        spellcheck
        directional
        classifier: work out which areas of the codebase this involves

refinement:
    agentic step takes in context (code, todos) and massages this ticket until
    it is deemed 'consistent' against the overalll goal and epics, work so far and other work remaining
    any work that is then deemed some distance from consistent is called out during this phase.
    it may then be broken into an epic if it is 'too complex'
        in which case the 'origninal intent' is preserved and the epic is then minted and broken down

implement:
    agentic step
    writes the code
    verifies against requirement
    runs, tests

integrator:
    agentic step
    releases the feature into the main codebase

deployer
    agentic step
    deploys the code to an environment

the workflow designer is used to DAG chain these workflow steps

a step is triggered with an input and a todo

-------------------
tickets
-------------------
    A ticket is basically a task that can be broken down to subtasks
    a ticket name is just a human thing; project, goal, epic, [task, bug, feature, chore] - really it is just a task that may or may not have children but always has a parent - with the exceptio of the root task per project which has none.

    a ticket will then have a start and end date, last updated and history
        in this way the ticket an edge in a ticket graph

    a ticket ultimatey has a prompt - th aggregate of itself, any parental context or advice.
    this prompt can be viewed at any point and can be sent directly to a harness.
    this is basically hte point-  the entire endevour is meant to be a low-friction way of interacting with the latest and best harness and model from the providers in a structured manner.  But hte prompt itself shoudl basically be copy/paste-able into a harness by hand and (Assuming the laptop setup is the same re codebase and permisssions) that the developer experience is basically the same inside or out, per task

    the key then is the friction and speed of all processes 'outside' of the harness.  the harness itself is a direct invocation, e.g. 'codex' whci is PTY'ed via a javascript session (or CLI session) which we call 'teleporting'.  You can 'teleport' to an agent or task at any point - which will then be the way of "jumping in" to a task and interacting with the harness where necessary.   The WWW interface also then embellishes the interface, so e..g when the user is ask fored prompts information like 1, 2, 3, yes, no, the harness wrapper will intelligently inspect the STDOUT and and assemble two wasys of interacting - the original STDIN/OUT as well as some relevant buttons on the WWW gui.


-------------------
attention
-------------------

Attention is the limited resource.   Whenever an agent requires something from the human, we want to generally score this in a negative sense; the continualy improvemnt of the workflow and tools themselves goal is to reduce the need for attention - that is, the attention of the human should be steered toward the most important diecisions adn feedback.

This means for example lowering the friction in harnesss invocation and allow prompting; teleport should 'learn' from the history the tickets what the 'better' shape of permissios is for harnesses they are wrapping and it should augment the permissions between taks - this means under the covers perhaps the harness process is killed, reconfigured and then resumed? this then depends on the harness version which is something that the binary will continually need to stay up to date wiht.

-------------------
ideation
-------------------
unstructured ideas should be able to be instantly captured and then easily upated later
double-shift is a context-aware search capability
ideas for example are generally sorted in most recent first
in an idea it should be possible to be presented multiple ideas that feel similar (classified same) which should then allow them to be merged.

-------------------
tk headless
-------------------
    ability to dry-run a workflow via the API

-------------------
the big split
-------------------
    promote 'hard' capabilities to their own library or binary and move the codebase out to a tool+skill
    this is to enforce API contract boundaries and known interfaces of code bleed/context mgmt

-------------------
dedicated agent session
-------------------
- *all* detected harness sessions either inside or outside the wrapper agent process should be reportable e.g via like an agent monitor (top one would be this shell), followed by other running agents and agent wrappers; all teh wrappers can be telepoted to
- enable a dedicated harness session that is clickable similar to the javascript shell
- the session should
    - have a drop down to select harness, model, thinking
    should preload any allow/deny rules
    preload skills for tk
    should be 'stoppable'



-------------------
tk agent
-------------------

Run a binary pointing to the tk server api
    binary is a wrapper around a harness (depending on what is availble on the computer)
    agent wraps an invoked codex/claude etc. session and so will be iterating a lot based on the PTY from the harness provider
    initially the binary itself will be spawned and running on same computer as all the other processes
    eventually it will be a remote process (on a different computer)
    an agent has a id/password and an owner - the project it is attached to
    agent uses the same tk api spec as a human -it communicates using the same primitive
    agent uses a tk skill to know how to interact with tk, or just calls `tk help`
    agent has a protocol to talk back to the api - this does not change often
    agent has a provider-specific interface to a PTY for the provider harness codex, claude etc. - this will need to change as often as a new harness version comes out.  this means building tk agent will be something that needs to be
        - fast
        - reliable
        - cater to multiple harnesses
    agent receives the todo and project configration via the api
        - so only state an agent has is identity and url to server

    out of scope
        - credentials/specific harness setup - the process can be started on the same computer as the server so -I will preauthenticate teh harnesses for now
        - provider api calls - we use the harnesses like a normal human uses them wiht all the features - but mostly via prompting directly in the stdin prompting loop

    tk agentd
        TELEPORT_USENAMNE
        TELEPORT_PASSWORD
        TELEPORT_URL
        -dir (working dir)


-------------------
tk agent manegment
-------------------
A project will have N agents allocated.  The workflow pemits a maximum of M agents per agentic swimlane.   The workflow may allow more agents thatn the project has, in which case there would be a warning that the project is not allocated sufficientlt - but this is a WIP and we will iterate.

The website user can invoke an agent 'in' the project which will receive the setup to have all context like one of the agents used in the workflow.  This is a teleport session that the user will find indistinguishable from a regular harness call; except it is wrapped as a javascript interface.

The www uer can teleport to any running agent.

Whenever an agent requires input, this is sent back to the site and rendered appropriately
    - as a call in the actions log on the user profile (turns red)
    - on the story itsefl indicating it requires attention

-------------------
waitlist
-------------------

implement a waitlist capability on the login page that can be enabled by the admin themselves; it is to capture an email address that is ten stored in the db with a timestamp of creation, the status (waiting), and the last updated by admin; this foms a queue for registering; encrypt the email so I cannot see it - BUT that if the admin wanted to, they could search for an email which would indicate if it exists or not - in this way duplicates can be managed.

-------------------
email process
-------------------
    implement an email function
    configurable by admin to setup email (using forwardemail)
    configurable page size and send frequency
    SMTP or STDOUT
    records emails sent and encrypts email and content
    CLI to contain new command email usable users with role email

    # send an invite email
    tk email invite          -email first.last@domain.com

    # send a forgot password link invokes the backend process and puts email onto queue
    tk email forgot-password -email first.last@domain.com

    # send a forgot password link invokes the forgot password process and puts email onto queue
    tk email forgot-password -email first.last@domain.com

    # send a change password link invokes the change password process and puts email onto queue
    tk email change-password -email first.last@domain.com

    # send top N emails now
    tk send -size N

    # send default top N emails now
    tk send

email gui
    create an email tab visible to the admin
        queued/sent, pagination on the table
        ability to resend and sent

-------------------
user inspection
-------------------
    ability to inspect users and their history and any emails or changes as an admin

-------------------
email queue
-------------------

implement an email function in the binary e.g. tk emaild which would be hte email deamon.  this can run indepdendently using SMTP or STDOUT and talks 'back' to the api deaemon/server which gives it its configuration

the admin can set via config on www or cli to STDOUT or SMTP and supply encrypted credentials

serve: www, api, email, scheduler commands

    the server side components bundled in a single binary `tk`:
    split the proceses so that they can be run toger or separaterly, e.g.
        tk serve.       - everything
            this is hte siplest way of just runing everything
            it is assumed that via config (www or cli) the processes below can be controlled
        tk www          - only the www
        tk api          - only the api
        tk email        - only the www
        tk scheduler    - only the scheduler


    tk agent        - runs a named agent which is the wrpper around a harness
        there will be multiple agents
        they are
            - wrappers around a harness on the computer running
            - clients of the teleport server

-------------------
organisations (e..g org.teleport.engineering)
    should route to different infrastructure and ensure the hosts carries through so the org is understood
    shoudl run on a different set of infra 100%

-------------------
membership
-------------------

Give user N invites; via a code they can send to the users email.
    user can register with that email only
    later on they can change email address
-




-----
UX
-----
the card view from teh graph is useful for epics/stories
I think perhaps if more stories are created inside of another, then the parent becomes a container that can be grown/shrunk to
show all the sotires and their dependencies style

in this way clicking on an epic (or parent) an clicking new woudl bring up the dialgo with the preset parent as the previous selection
if the parent was an epic then this woudl eb a task


--------
ideas about workflow steps
--------

context menu
    spellcheck
        runs a small model grammar/spellchecker
    classifier
        reviews the project goals, specification and attempts to classify what sectiosn this story adheres to
    refiner
        reviews the project goals, specification and attempts to classify what sectiosn this story adheres to
    duplicate checker
        attempts to find any similar items in a backlog and decides should these merge or shuld othe aspects get pulled in
    consistency
        attempts to find flaws in the todo vs. any others, any contradictions


-------

merge views for: ideas, goals, ecpis, stories to a single "backlog"
    list of stories: use the visual style of the cards in the 2d graph view
        slighly indent stories as tehy go through their parentage
        allow them to be drag/drop to reorder them
        if a task is not yet locked; it can be dragged into/out of another task

remove the projects view as it is available from the dropdown

the header of all the views shoudl eb configurable to appear on the left, right or the top via settings


----

a tk import <file> command to take an entire text piece and attemtp to classify into goals
a web capability
    <available ont he backlog via an "import/upload" which either selects a file or accepts a long-form text input">
    same for the web where it would then be proposed back to the user the arrangement of goals being proposed, at which point
    the response shoudl use the card-style layout of all the goals and then hovering over them soudl show their details and any    questions/clarifications; the user is free to accept as-is at any time and hte questions woudl then remain on each goaal as necessary for future refinement.

    the user coudl accept and continue, accept, refine, cancel
        the accept
            creates the goasl and stops
        refine
            takes input which is probably "merge goals 1, 2, 3, split 5 into three" - which goes back to the agent classifier
        cancel
            just stop and terminate this agentic session

    this is powered by an agent which
        classifies the text and proposes actual goals
        optionally looks at existing project documentation to see how consistent those goals [or can be done later during refinement]

all agents need to have an "inspectable/editable" set of motiviations and goals; this means that generallt the agent harness code is not 'hard coded' for a given workflow, rather it is given its configuration/goal/prompts as they are stored in the server themselves, version controlled.  this allows for introspection adn evolution of goals, personas, motivation over time.


goal lifecycle
    it's possible that a goal is nto actuallty achievable insofar as marked as "done" - that is directional for hte capabilitiesand outcomes that are desired of any given software solution

    whereas a task can be done - so a task is discrete, bounded, constrained.   A goal is possibly. amuch longer-form thing.
