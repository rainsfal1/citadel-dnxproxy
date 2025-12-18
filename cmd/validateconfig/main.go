package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func main() {
	schemaPath := flag.String("schema", "configs/schema.json", "path to JSON schema")
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatalf("no config files supplied; usage: validateconfig -schema configs/schema.json configs/*.json")
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", mustOpen(*schemaPath)); err != nil {
		log.Fatalf("failed to load schema: %v", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		log.Fatalf("schema compilation failed: %v", err)
	}

	ok := true
	for _, path := range flag.Args() {
		data, err := readJSON(path)
		if err != nil {
			ok = false
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			continue
		}
		if err := schema.Validate(data); err != nil {
			ok = false
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		} else {
			fmt.Printf("%s: ok\n", path)
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func mustOpen(path string) *os.File {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open %s: %v", path, err)
	}
	return f
}

func readJSON(path string) (interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	var data interface{}
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return data, nil
}
