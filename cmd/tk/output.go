package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// printResponse keeps machine output valid JSON and escapes control characters in terminals.
func printResponse(out io.Writer, body []byte, machine bool) error {
	if machine {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, body, "", "  "); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, pretty.String())
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return err
	}
	var line func(any) string
	line = func(v any) string {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				b, _ := json.Marshal(x[k])
				parts = append(parts, k+"="+string(b))
			}
			return strings.Join(parts, "  ")
		default:
			b, _ := json.Marshal(x)
			return string(b)
		}
	}
	if obj, ok := value.(map[string]any); ok {
		if items, ok := obj["items"].([]any); ok {
			for _, item := range items {
				if _, err := fmt.Fprintln(out, line(item)); err != nil {
					return err
				}
			}
			if len(items) == 0 {
				fmt.Fprintln(out, "No items.")
			}
			for k, v := range obj {
				if k != "items" && v != nil && v != "" {
					fmt.Fprintf(out, "%s: %s\n", k, line(v))
				}
			}
			return nil
		}
	}
	if items, ok := value.([]any); ok {
		for _, item := range items {
			if _, err := fmt.Fprintln(out, line(item)); err != nil {
				return err
			}
		}
		if len(items) == 0 {
			fmt.Fprintln(out, "No items.")
		}
		return nil
	}
	_, err := fmt.Fprintln(out, line(value))
	return err
}
