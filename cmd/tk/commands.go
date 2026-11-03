package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"teleport/internal/server"
)

type cliCommand struct {
	Command   string         `json:"command"`
	Operation string         `json:"operation,omitempty"`
	Summary   string         `json:"summary"`
	Input     map[string]any `json:"input,omitempty"`
}

// Canonical noun/action spellings. Registry additions without a spelling remain
// callable and discoverable as "api NAME" until a deliberate alias is chosen.
const commandRoutes = `
getSession account show
logout account logout
changePassword account password
listSessions agent ls
getLocalHarnesses agent harnesses
openConsoleSession agent start
startTicketSession agent start-ticket
attachSession agent attach
sendSessionInput agent input
resizeSession agent resize
stopSession agent stop
shutdownLocalRuntime agent shutdown
listSessionInterventions agent interventions
getSessionTurn agent turn
getSessionRequests agent requests
respondToAgent agent respond
createProject project new
listProjects project ls
listProjectSummaries project summaries
saveProject project edit
configureLocalRepository project repository
getHome project home
setLaneCapacity project capacity
getBoardExecution board execution
createTicket ticket new
listTickets ticket ls
getTicket ticket show
updateTicket ticket edit
transitionTicket ticket move
listTicketHistory ticket history
searchTickets ticket search
moveTicket ticket parent
transferTicket ticket transfer
cancelTicket ticket cancel
reopenTicket ticket reopen
setTicketVisibility ticket visibility
setDependencies ticket dependencies
pauseTicket ticket pause
resumeTicket ticket resume
getTicketAgreements ticket agreements
getTicketIdeas ticket ideas
requestRefinement ticket refine
returnRefinementToTodo ticket unqueue
acceptRefinement ticket accept-scope
rejectRefinement ticket reject-scope
answerQuestion ticket answer
markReady ticket ready
approveTicket ticket approve
rejectTicket ticket reject
setExecutionBudget ticket budget
prepareTaskWorktree ticket worktree
recordTaskDelivery ticket delivery
reconcileGitOperation ticket reconcile-git
getPrompt prompt show
getEffectivePermission permission show
getGoalView goal ls
getGoalSummary goal summary
configureGoalReview goal configure-review
reviewGoal goal review
listGoalReviews goal reviews
getEpicView epic ls
saveIdea idea save
listIdeas idea ls
getIdea idea show
getIdeaHistory idea history
getIdeaLinks idea links
getIdeaProposal idea proposal
submitIdeaProposal idea propose
acceptIdeaProposal idea accept
previewIdeaPlacement idea preview-placement
setIdeaPlacement idea placement
setIdeaVisibility idea visibility
listWorkflows workflow ls
getWorkflow workflow show
copyWorkflow workflow copy
draftWorkflow workflow draft
saveWorkflow workflow save
publishWorkflow workflow publish
deleteWorkflow workflow delete-draft
activateWorkflowVersion workflow activate
previewWorkflowOutcome workflow preview
previewWorkflowMigration workflow preview-migration
migrateWorkflow workflow migrate
getCalendar calendar show
getBacklogOrder backlog order
saveBacklogOrder backlog save-order
getGraph graph show
saveGraphLayout graph layout
getAppearanceStyles appearance styles
setAppearance appearance set
getViewPreferences preferences show
saveViewPreferences preferences save
saveCurrentLocation preferences location
setExecutionMode system execution
setHandbrake system handbrake
runDemoStage demo stage
getScriptBindings script bindings
startScriptRun script run
listScriptRuns script ls
getScriptRun script show
stopScriptRun script stop
confirmScriptCall script confirm
scriptCall script call
getWelcomeSetup welcome show
checkProjectAvailability welcome availability
saveWelcomeSetup welcome save
completeWelcomeSetup welcome complete
`

func commandCatalogue() []cliCommand {
	routes := map[string]string{}
	for _, line := range strings.Split(commandRoutes, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 3 {
			routes[parts[0]] = strings.Join(parts[1:], " ")
		}
	}
	commands := []cliCommand{
		{Command: "api OPERATION", Summary: "Call a server operation with explicit JSON; native/private commands are rejected"},
		{Command: "help [NOUN [ACTION]]", Summary: "List every shipped public command, or command-specific help (offline)"},
		{Command: "commands [-json]", Summary: "Print the complete offline command catalogue"},
		{Command: "init", Summary: "Initialize configured PostgreSQL; never starts services"},
		{Command: "serve", Summary: "Run the web/API server and configured components"},
		{Command: "login -password-stdin", Operation: "login", Summary: "Sign in without putting a password in process arguments"},
		{Command: "import FILE | -i", Summary: "Capture UTF-8 text as an Idea; classification follows separately"},
		{Command: "skill", Summary: "Print the bundled harness SKILL.md (offline)"},
		{Command: "version", Summary: "Print the local build identity (offline)"},
		{Command: "tools", Summary: "Fetch the running server's operation discovery JSON"},
		{Command: "spec", Summary: "Fetch the running server's OpenAPI contract"},
	}
	for _, operation := range server.Operations {
		if operation.Name == "login" {
			continue // The native login command owns secret input and credential storage.
		}
		path := routes[operation.Name]
		if path == "" {
			path = "api " + operation.Name
		}
		input, _ := server.OperationInputSchema(operation.Name)
		commands = append(commands, cliCommand{path, operation.Name, operation.Summary, input})
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Command < commands[j].Command })
	return commands
}

func printCatalogue(out io.Writer, topic string, machine bool) error {
	items := []cliCommand{}
	for _, command := range commandCatalogue() {
		if topic == "" || command.Command == topic || strings.HasPrefix(command.Command, topic+" ") || command.Operation == topic {
			items = append(items, command)
		}
	}
	if len(items) == 0 {
		return fmt.Errorf("unknown command %q; run tk help", topic)
	}
	if machine {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			Commands []cliCommand `json:"commands"`
		}{items})
	}
	fmt.Fprintln(out, "teleport (tk) — shipped public commands")
	for _, command := range items {
		fmt.Fprintf(out, "  tk %-30s %s\n", command.Command, command.Summary)
	}
	fmt.Fprintln(out, "\nUse tk NOUN ACTION -h for typed flags. list/create/get/update/grep also work as ls/new/show/edit/search aliases where available.")
	fmt.Fprintln(out, "API-style names remain supported: tk OPERATION -data JSON, or tk api OPERATION -data JSON.")
	fmt.Fprintln(out, "Use -server URL and -config PATH for API calls. tk tools/spec describe the running server, which may differ from this build.")
	fmt.Fprintln(out, "Init starts no services or agents. Only init accesses storage as a client. Environment: TELEPORT_SERVER, TELEPORT_CONFIG, TELEPORT_DB, TELEPORT_DB_CONFIG.")
	return nil
}

func expandCommand(args []string, out io.Writer) ([]string, bool, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "commands" || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		var topic []string
		machine := false
		if len(args) > 1 {
			for _, argument := range args[1:] {
				if argument == "-json" {
					machine = true
				} else if argument == "-h" || argument == "-help" || argument == "--help" {
					continue
				} else if strings.HasPrefix(argument, "-") {
					return nil, true, errors.New("help accepts a noun/action topic and optional -json")
				} else {
					topic = append(topic, argument)
				}
			}
		}
		// Detailed operation help includes its generated field flags.
		if len(topic) == 2 && !machine {
			return expandCommand(append(topic, "-h"), out)
		}
		return nil, true, printCatalogue(out, strings.Join(topic, " "), machine)
	}
	for _, native := range []string{"init", "serve", "login", "logout", "skill", "version", "tools", "spec", "import"} {
		if args[0] == native || strings.HasPrefix(args[0], "_") {
			return args, false, nil
		}
	}
	if args[0] == "api" {
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return nil, true, printCatalogue(out, "", false)
		}
		if args[1] == "" {
			return nil, false, errors.New("provide an API operation name")
		}
		for _, reserved := range []string{"init", "serve", "skill", "version", "tools", "spec", "import", "help", "commands", "api"} {
			if args[1] == reserved {
				return nil, false, errors.New("tk api accepts API operation names, not native commands")
			}
		}
		for index, char := range args[1] {
			if !(char >= 'a' && char <= 'z' || index > 0 && char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
				return nil, false, errors.New("API operation names must start with a lowercase letter and contain only letters and digits")
			}
		}
		// Explicit escape hatch for an operation added by a newer server build.
		return args[1:], false, nil
	}
	for _, command := range commandCatalogue() {
		if command.Operation != "" && args[0] == command.Operation {
			return args, false, nil // Preserve the existing raw operation interface exactly.
		}
	}
	actions := map[string]string{"list": "ls", "create": "new", "get": "show", "update": "edit", "grep": "search"}
	for _, command := range commandCatalogue() {
		parts := strings.Fields(command.Command)
		if command.Operation == "" || len(parts) != 2 || args[0] != parts[0] {
			continue
		}
		if len(args) == 1 || args[1] == "-h" || args[1] == "--help" {
			return nil, true, printCatalogue(out, args[0], false)
		}
		action := args[1]
		if alternative := actions[action]; alternative != "" {
			action = alternative
		}
		if action == parts[1] {
			return expandOperation(command, args[2:], out)
		}
	}
	return nil, false, errors.New("unknown command; run tk help to list available nouns and actions")
}
