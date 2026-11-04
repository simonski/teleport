package clicommand

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type requestFlag struct {
	name, kind string
	fields     map[string]json.RawMessage
}

func (f *requestFlag) String() string   { return "" }
func (f *requestFlag) IsBoolFlag() bool { return f.kind == "boolean" }
func (f *requestFlag) Set(value string) error {
	if _, present := f.fields[f.name]; present {
		return errors.New("field supplied more than once")
	}
	var encoded []byte
	switch f.kind {
	case "string":
		encoded, _ = json.Marshal(value)
	case "integer":
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("expected a signed integer")
		}
		encoded, _ = json.Marshal(number)
	case "number":
		var number json.Number
		if err := json.Unmarshal([]byte(value), &number); err != nil {
			return errors.New("expected a JSON number")
		}
		encoded = []byte(value)
	case "boolean":
		boolean, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("expected true or false")
		}
		encoded, _ = json.Marshal(boolean)
	default:
		value = strings.TrimSpace(value)
		if !json.Valid([]byte(value)) || f.kind == "array" && !strings.HasPrefix(value, "[") || f.kind == "object" && !strings.HasPrefix(value, "{") {
			return errors.New("expected a JSON " + f.kind)
		}
		encoded = []byte(value)
	}
	f.fields[f.name] = encoded
	return nil
}

func fieldKind(input map[string]any) string {
	if kind, ok := input["type"].(string); ok {
		return kind
	}
	if alternatives, ok := input["anyOf"].([]any); ok {
		for _, alternative := range alternatives {
			if item, ok := alternative.(map[string]any); ok && item["type"] != "null" {
				return fieldKind(item)
			}
		}
	}
	return "value"
}

func kebabField(value string) string {
	var result strings.Builder
	for index, char := range value {
		if unicode.IsUpper(char) && index > 0 {
			result.WriteByte('-')
		}
		result.WriteRune(unicode.ToLower(char))
	}
	return result.String()
}

// Separate known flags from positional arguments without mistaking a flag's
// value for a positional ID. Unlike flag.Parse, this accepts flags after an ID.
func ParseInterspersed(set *flag.FlagSet, arguments []string) ([]string, error) {
	var options, positional []string
	seen := map[string]bool{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			positional = append(positional, arguments[index+1:]...)
			break
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			positional = append(positional, argument)
			continue
		}
		name, _, equal := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		if name == "h" || name == "help" {
			set.Usage()
			return nil, flag.ErrHelp
		}
		field := set.Lookup(name)
		if field == nil {
			return nil, fmt.Errorf("unknown flag %q; use -h for available flags", "-"+name)
		}
		if seen[name] {
			return nil, fmt.Errorf("flag %q was supplied more than once", "-"+name)
		}
		seen[name] = true
		options = append(options, argument)
		boolean, isBoolean := field.Value.(interface{ IsBoolFlag() bool })
		if !equal && !(isBoolean && boolean.IsBoolFlag()) {
			index++
			if index == len(arguments) {
				return nil, fmt.Errorf("flag -%s requires a value", name)
			}
			options = append(options, arguments[index])
		}
	}
	set.SetOutput(io.Discard)
	usage := set.Usage
	set.Usage = func() {}
	defer func() { set.Usage = usage }()
	if err := set.Parse(options); err != nil {
		return nil, errors.New("invalid flag value; use -h for the expected field types")
	}
	return positional, nil
}

func requestObject(body string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(body))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("-data/-f must contain a JSON object")
	}
	object := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid JSON request object")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid JSON object key")
		}
		if _, present := object[key]; present {
			return nil, fmt.Errorf("request field %q was supplied more than once", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid JSON request value")
		}
		object[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("invalid JSON request object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("provide exactly one JSON request object")
	}
	return object, nil
}

func NewRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("system randomness unavailable")
	}
	return fmt.Sprintf("cli-%x", value[:])
}

func ExpandOperation(command Command, arguments []string, out, diagnostics io.Writer, allowFiles bool) ([]string, bool, error) {
	set := flag.NewFlagSet(command.Command, flag.ContinueOnError)
	set.SetOutput(out)
	raw := set.String("data", "", "JSON request object; cannot duplicate field flags")
	file := set.String("f", "", "read a JSON request from FILE, or - for stdin")
	endpoint := set.String("server", "", "server URL (otherwise TELEPORT_SERVER/default)")
	config := set.String("config", "", "private client credential file")
	machine := set.Bool("json", false, "JSON result on stdout; diagnostics remain on stderr")
	fields := map[string]json.RawMessage{}
	properties, _ := command.Input["properties"].(map[string]any)
	required, _ := command.Input["required"].([]string)
	keys := make([]string, 0, len(properties))
	for name := range properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		input := properties[name].(map[string]any)
		// Credentials use JSON file/stdin input, not generated argument flags.
		lower := strings.ToLower(name)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			continue
		}
		value := &requestFlag{name, fieldKind(input), fields}
		usage := name + " (" + value.kind + ")"
		for _, key := range required {
			if key == name {
				usage += "; required"
			}
		}
		flagName := kebabField(name)
		if set.Lookup(flagName) != nil {
			flagName = "field-" + flagName
		}
		set.Var(value, flagName, usage)
		for alias, target := range map[string]string{"project": "projectId", "parent": "parentId", "p": "parentId", "version": "expectedVersion", "t": "title", "d": "description", "desc": "description"} {
			if target == name && set.Lookup(alias) == nil {
				set.Var(value, alias, "alias for -"+kebabField(name))
			}
		}
	}
	set.Usage = func() {
		set.SetOutput(out)
		defer set.SetOutput(io.Discard)
		fmt.Fprintf(out, "Usage: tk %s [ID] [flags]\n%s\nAPI operation: %s\n", command.Command, command.Summary, command.Operation)
		set.PrintDefaults()
		fmt.Fprintln(out, "Object/array flags take JSON. Secret fields use -f - JSON stdin. Required request IDs are generated if omitted; pass -request-id to retry the same request.")
	}
	positional, err := ParseInterspersed(set, arguments)
	if errors.Is(err, flag.ErrHelp) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	hasData, hasFile := false, false
	set.Visit(func(value *flag.Flag) {
		hasData = hasData || value.Name == "data"
		hasFile = hasFile || value.Name == "f"
	})
	if hasData && hasFile {
		return nil, false, errors.New("choose -data or -f, not both")
	}
	if hasFile {
		if !allowFiles {
			return nil, false, errors.New("-f is not available in the browser console; use the local tk CLI for file or stdin input")
		}
		if *file == "" {
			return nil, false, errors.New("-f requires a file path or - for stdin")
		}
		input := io.Reader(os.Stdin)
		if *file != "-" {
			opened, err := os.Open(*file)
			if err != nil {
				return nil, false, err
			}
			defer opened.Close()
			input = opened
		}
		body, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			return nil, false, errors.New("JSON request must fit within 1 MiB")
		}
		*raw = string(body)
	}
	if hasData || hasFile {
		if len(*raw) > 1<<20 {
			return nil, false, errors.New("JSON request must fit within 1 MiB")
		}
		object, err := requestObject(*raw)
		if err != nil {
			return nil, false, err
		}
		for key, value := range object {
			if _, exists := fields[key]; exists {
				return nil, false, fmt.Errorf("%q appears in both JSON and field flags", key)
			}
			fields[key] = value
		}
	}
	if len(positional) > 0 {
		if len(positional) != 1 || properties["id"] == nil {
			return nil, false, errors.New("use named flags for this command; at most one positional ID is accepted")
		}
		input := properties["id"].(map[string]any)
		if err := (&requestFlag{"id", fieldKind(input), fields}).Set(positional[0]); err != nil {
			return nil, false, errors.New("positional ID conflicts with -id/-data, or has the wrong type")
		}
	}
	for key := range fields {
		if properties[key] == nil {
			return nil, false, fmt.Errorf("unknown request field %q; use -h", key)
		}
		lower := strings.ToLower(key)
		if *file == "" && (strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret")) {
			return nil, false, errors.New("secret request fields require -f FILE or -f - JSON stdin")
		}
	}
	for _, key := range required {
		if _, present := fields[key]; !present {
			if key == "requestId" {
				continue
			}
			return nil, false, fmt.Errorf("-%s is required; use tk %s -h", kebabField(key), command.Command)
		}
	}
	wantsRequestID := false
	for _, key := range required {
		wantsRequestID = wantsRequestID || key == "requestId"
	}
	wantsRequestID = wantsRequestID || command.Operation == "saveIdea" && (len(fields["id"]) == 0 || string(fields["id"]) == `""`)
	if wantsRequestID && len(fields["requestId"]) == 0 {
		id := NewRequestID()
		fields["requestId"], _ = json.Marshal(id)
		fmt.Fprintln(diagnostics, "Request ID:", id, "(reuse -request-id for an uncertain retry)")
	}
	body, err := json.Marshal(fields)
	if err != nil || len(body) > 1<<20 {
		return nil, false, errors.New("complete JSON request must fit within 1 MiB")
	}
	result := []string{command.Operation, "-data", string(body)}
	if *machine {
		result = append(result, "-json")
	}
	if *endpoint != "" {
		result = append(result, "-server", *endpoint)
	}
	if *config != "" {
		result = append(result, "-config", *config)
	}
	return result, false, nil
}
