package main

import (
	"flag"
	"io"
	"os"

	"teleport/internal/clicommand"
	"teleport/internal/server"
)

type cliCommand = clicommand.Command

func commandCatalogue() []cliCommand {
	operations := make([]clicommand.Operation, 0, len(server.Operations))
	for _, operation := range server.Operations {
		input, _ := server.OperationInputSchema(operation.Name)
		operations = append(operations, clicommand.Operation{Name: operation.Name, Summary: operation.Summary, Input: input})
	}
	return clicommand.Catalogue(operations)
}

func expandCommand(args []string, out io.Writer) ([]string, bool, error) {
	return clicommand.Expand(args, out, os.Stderr, commandCatalogue(), true)
}

func expandOperation(command cliCommand, args []string, out io.Writer) ([]string, bool, error) {
	return clicommand.ExpandOperation(command, args, out, os.Stderr, true)
}

func parseInterspersed(set *flag.FlagSet, args []string) ([]string, error) {
	return clicommand.ParseInterspersed(set, args)
}

func newRequestID() string { return clicommand.NewRequestID() }
