package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseFormatsPreserveNumbersAndEscapeTerminalControls(t *testing.T) {
	body := []byte(`{"items":[{"id":9007199254740993,"name":"moon\u001b[2J"}],"nextCursor":"next"}`)
	var human, machine bytes.Buffer
	if err := printResponse(&human, body, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(human.String(), "\x1b") || !strings.Contains(human.String(), "9007199254740993") || !strings.Contains(human.String(), "next") {
		t.Fatal(human.String())
	}
	if err := printResponse(&machine, body, true); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(machine.Bytes()) || !strings.Contains(machine.String(), "\n  \"items\"") {
		t.Fatal(machine.String())
	}
}
