package settings

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestLookup(t *testing.T) {
	d, err := Lookup("language")
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Enum || d.Default != "auto" || d.Env != "CID_LANG" {
		t.Errorf("unexpected language def: %+v", d)
	}
	if _, err := Lookup("nope"); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("Lookup(nope) err = %v, want ErrUnknownKey", err)
	}
}

func TestNormalize(t *testing.T) {
	d, _ := Lookup("language")
	for _, in := range []string{"es", "ES", " es ", "Auto"} {
		if _, err := d.Normalize(in); err != nil {
			t.Errorf("Normalize(%q) = %v, want ok", in, err)
		}
	}
	got, _ := d.Normalize(" ES ")
	if got != "es" {
		t.Errorf("Normalize(\" ES \") = %q, want es", got)
	}
	for _, in := range []string{"", "fr", "spanish"} {
		var iv *InvalidValueError
		if _, err := d.Normalize(in); !errors.As(err, &iv) {
			t.Errorf("Normalize(%q) err = %v, want InvalidValueError", in, err)
		} else if !slices.Equal(iv.Allowed, []string{"auto", "en", "es"}) {
			t.Errorf("allowed = %v", iv.Allowed)
		}
	}
}

func TestNormalizeBoolAndString(t *testing.T) {
	b := Def{Key: "b", Kind: Bool, Default: "false"}
	if v, err := b.Normalize("TRUE"); err != nil || v != "true" {
		t.Errorf("bool normalize = %q, %v", v, err)
	}
	if _, err := b.Normalize("yes"); err == nil {
		t.Error("bool accepted yes")
	}
	s := Def{Key: "s", Kind: String}
	if v, err := s.Normalize(" x "); err != nil || v != " x " {
		t.Errorf("string normalize = %q, %v", v, err)
	}
}

// The embedded schema is the contract for config.json; it must describe
// exactly the registry.
func TestSchemaMatchesRegistry(t *testing.T) {
	var doc struct {
		Properties map[string]struct {
			Type    string   `json:"type"`
			Enum    []string `json:"enum"`
			Default any      `json:"default"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemaDoc, &doc); err != nil {
		t.Fatal(err)
	}
	defs := Defs()
	if len(doc.Properties) != len(defs) {
		t.Fatalf("schema has %d properties, registry %d", len(doc.Properties), len(defs))
	}
	for _, d := range defs {
		p, ok := doc.Properties[d.Key]
		if !ok {
			t.Errorf("key %q missing from schema", d.Key)
			continue
		}
		wantType := map[Kind]string{String: "string", Enum: "string", Bool: "boolean"}[d.Kind]
		if p.Type != wantType {
			t.Errorf("%s: schema type %q, want %q", d.Key, p.Type, wantType)
		}
		if d.Kind == Enum && !slices.Equal(p.Enum, d.Allowed) {
			t.Errorf("%s: schema enum %v, registry %v", d.Key, p.Enum, d.Allowed)
		}
		if d.Kind != Bool && p.Default != d.Default {
			t.Errorf("%s: schema default %v, registry %q", d.Key, p.Default, d.Default)
		}
	}
}

func TestValidateEntry(t *testing.T) {
	if err := validateEntry("language", json.RawMessage(`"es"`)); err != nil {
		t.Errorf("valid entry: %v", err)
	}
	for _, raw := range []string{`"fr"`, `5`, `null`, `["es"]`} {
		if err := validateEntry("language", json.RawMessage(raw)); err == nil {
			t.Errorf("entry %s accepted", raw)
		}
	}
}
