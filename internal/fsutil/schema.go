// Package fsutil holds the safe-JSON-file helpers shared by cid's stores:
// schema validation, atomic writes with a .bak copy, and recovery from
// hand-edited or corrupted files.
package fsutil

import (
	"bytes"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Schema is a compiled JSON Schema.
type Schema struct{ s *jsonschema.Schema }

// CompileSchema compiles a JSON Schema document. name is only used in errors.
func CompileSchema(name string, doc []byte) (*Schema, error) {
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return nil, fmt.Errorf("parse schema %s: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, v); err != nil {
		return nil, fmt.Errorf("add schema %s: %w", name, err)
	}
	s, err := c.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", name, err)
	}
	return &Schema{s: s}, nil
}

// MustCompileSchema is CompileSchema for embedded schemas. It panics on a
// schema that does not compile, a programming error that tests catch.
func MustCompileSchema(name string, doc []byte) *Schema {
	s, err := CompileSchema(name, doc)
	if err != nil {
		panic(err)
	}
	return s
}

// Validate reports whether raw is well-formed JSON that satisfies the schema.
func (s *Schema) Validate(raw []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := s.s.Validate(inst); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	return nil
}
