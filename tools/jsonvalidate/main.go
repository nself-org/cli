// Command jsonvalidate checks one JSON document against the generated schema.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type schemaIndex struct {
	Schemas []struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	} `json:"schemas"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "jsonvalidate:", err)
		os.Exit(1)
	}
}

// run validates a single document and, when requested, its data property
// against the command's registered data schema.
func run(args []string) error {
	flags := flag.NewFlagSet("jsonvalidate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	schema := flags.String("schema", "", "schema file under schemas/")
	command := flags.String("data-from-registry", "", "registry command path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *schema == "" || flags.NArg() != 1 {
		return errors.New("usage: jsonvalidate -schema <file> [-data-from-registry <command path>] <json file>")
	}
	idxBytes, err := os.ReadFile(filepath.Join("schemas", "index.json"))
	if err != nil {
		return err
	}
	var idx schemaIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	ids := map[string]string{}
	for _, entry := range idx.Schemas {
		b, err := os.ReadFile(filepath.Join("schemas", entry.Path))
		if err != nil {
			return err
		}
		var doc any
		if err := json.Unmarshal(b, &doc); err != nil {
			return err
		}
		if err := compiler.AddResource(entry.ID, doc); err != nil {
			return err
		}
		ids[entry.Path] = entry.ID
	}
	path := strings.TrimPrefix(*schema, "schemas/")
	id, ok := ids[path]
	if !ok {
		return fmt.Errorf("schema %q is not in schemas/index.json", path)
	}
	doc, err := readOne(flags.Arg(0))
	if err != nil {
		return err
	}
	if err := validate(compiler, id, doc); err != nil {
		return fmt.Errorf("envelope: %w", err)
	}
	if *command == "" {
		return nil
	}
	b, err := os.ReadFile(".github/command-registry.json")
	if err != nil {
		return err
	}
	var registry struct {
		Commands []struct {
			Path       string  `json:"path"`
			DataSchema *string `json:"data_schema"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(b, &registry); err != nil {
		return err
	}
	var matched *string
	for _, entry := range registry.Commands {
		if !strings.HasPrefix("nself "+*command+" ", entry.Path+" ") {
			continue
		}
		if matched != nil && len(*matched) >= len(entry.Path) {
			continue
		}
		path := entry.Path
		matched = &path
	}
	for _, entry := range registry.Commands {
		if matched == nil || entry.Path != *matched {
			continue
		}
		if entry.DataSchema == nil {
			return fmt.Errorf("%s has no data schema", *command)
		}
		dataID := ids[strings.TrimPrefix(*entry.DataSchema, "schemas/")]
		if dataID == "" {
			return fmt.Errorf("data schema %s is not indexed", *entry.DataSchema)
		}
		value, ok := doc.(map[string]any)["data"]
		if !ok {
			return nil
		} // error envelopes have no data property
		return validate(compiler, dataID, value)
	}
	return fmt.Errorf("registry has no command %q", *command)
}

func readOne(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	dec.UseNumber()
	var doc, extra any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("input must contain exactly one JSON value")
	}
	return doc, nil
}

func validate(c *jsonschema.Compiler, id string, value any) error {
	s, err := c.Compile(id)
	if err != nil {
		return err
	}
	return s.Validate(value)
}
