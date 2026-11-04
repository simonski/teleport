package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"teleport/internal/clicommand"
	teleportskill "teleport/internal/skills/teleport"
)

type ConsoleCommandInput struct {
	Args []string `json:"args"`
}
type ConsoleCommandResult struct {
	Operation  string          `json:"operation,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Output     string          `json:"output,omitempty"`
	Diagnostic string          `json:"diagnostic,omitempty"`
	Machine    bool            `json:"machine,omitempty"`
}

func consoleCatalogue() []clicommand.Command {
	operations := make([]clicommand.Operation, 0, len(Operations))
	for _, item := range Operations {
		input, _ := OperationInputSchema(item.Name)
		operations = append(operations, clicommand.Operation{Name: item.Name, Summary: item.Summary, Input: input})
	}
	return clicommand.Catalogue(operations)
}

func resolveConsoleCommand(input ConsoleCommandInput, version string) (ConsoleCommandResult, error) {
	if len(input.Args) > 256 {
		return ConsoleCommandResult{}, errors.New("too many command arguments")
	}
	args := input.Args
	if len(args) > 0 && args[0] == "tk" {
		args = args[1:]
	} else if len(args) > 0 {
		return ConsoleCommandResult{}, errors.New("Commands must start with tk")
	}
	if len(args) > 0 {
		switch args[0] {
		case "version":
			if len(args) == 2 && (args[1] == "-json" || args[1] == "--json") {
				data, _ := json.MarshalIndent(map[string]string{"version": version}, "", "  ")
				return ConsoleCommandResult{Output: string(data)}, nil
			}
			if len(args) != 1 {
				return ConsoleCommandResult{}, errors.New("usage: tk version [-json]")
			}
			return ConsoleCommandResult{Output: version}, nil
		case "skill":
			if len(args) != 1 {
				return ConsoleCommandResult{}, errors.New("Use tk skill without flags in this console")
			}
			return ConsoleCommandResult{Output: teleportskill.Markdown}, nil
		case "tools":
			if len(args) > 2 || len(args) == 2 && args[1] != "-json" && args[1] != "--json" {
				return ConsoleCommandResult{}, errors.New("usage: tk tools [-json]")
			}
			data, _ := json.Marshal(map[string]any{"items": Operations})
			return ConsoleCommandResult{Data: data, Machine: len(args) == 2}, nil
		case "spec":
			if len(args) > 2 || len(args) == 2 && args[1] != "-json" && args[1] != "--json" {
				return ConsoleCommandResult{}, errors.New("usage: tk spec [-json]")
			}
			data, _ := json.Marshal(openAPI(version))
			return ConsoleCommandResult{Data: data, Machine: len(args) == 2}, nil
		case "init", "serve", "setup", "login", "import":
			return ConsoleCommandResult{}, errors.New("This local or interactive command requires the tk CLI; the browser console uses the current signed-in session")
		}
		if args[0] == "project" && len(args) > 1 && args[1] == "rm" {
			return ConsoleCommandResult{}, errors.New("Project removal requires the local tk CLI confirmation flow")
		}
	}
	for _, arg := range args {
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && (name == "server" || name == "config") {
			return ConsoleCommandResult{}, errors.New("The browser console uses its current server and authenticated session")
		}
	}
	catalogue := consoleCatalogue()
	var output, diagnostics bytes.Buffer
	expanded, done, err := clicommand.Expand(args, &output, &diagnostics, catalogue, false)
	if err != nil {
		return ConsoleCommandResult{}, err
	}
	if done {
		return ConsoleCommandResult{Output: output.String()}, nil
	}
	if len(expanded) == 0 {
		return ConsoleCommandResult{}, errors.New("Unknown command")
	}
	if expanded[0] == "logout" && len(expanded) == 1 {
		return ConsoleCommandResult{Operation: "logout", Data: json.RawMessage(`{}`)}, nil
	}
	var command *clicommand.Command
	for i := range catalogue {
		if catalogue[i].Operation == expanded[0] && expanded[0] != "login" {
			command = &catalogue[i]
			break
		}
	}
	if command == nil {
		return ConsoleCommandResult{}, errors.New("Unknown or unavailable API operation")
	}
	// Recheck raw operation spellings and typed expansions against the same
	// schema and browser file/secret restrictions before returning anything.
	expanded, done, err = clicommand.ExpandOperation(*command, expanded[1:], &output, &diagnostics, false)
	if err != nil {
		return ConsoleCommandResult{}, err
	}
	if done {
		return ConsoleCommandResult{Output: output.String()}, nil
	}
	if len(expanded) < 3 || expanded[1] != "-data" {
		return ConsoleCommandResult{}, fmt.Errorf("Cannot resolve command")
	}
	result := ConsoleCommandResult{Operation: expanded[0], Data: json.RawMessage(expanded[2]), Diagnostic: diagnostics.String()}
	for _, arg := range expanded[3:] {
		if arg == "-json" {
			result.Machine = true
		} else {
			return ConsoleCommandResult{}, errors.New("The browser console cannot change server or credentials")
		}
	}
	return result, nil
}

func init() {
	Operations = append(Operations, Operation{"resolveConsoleCommand", "ACT-1212", "Resolve a tk console command without executing it"})
	contracts["resolveConsoleCommand"] = contract{ConsoleCommandInput{}, ConsoleCommandResult{}}
}
