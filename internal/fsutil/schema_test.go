package fsutil

import "testing"

const testSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["n"],
  "properties": {"n": {"type": "integer", "minimum": 1}}
}`

func TestSchemaValidate(t *testing.T) {
	s, err := CompileSchema("test.json", []byte(testSchema))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"valid", `{"n": 2}`, true},
		{"wrong type", `{"n": "2"}`, false},
		{"below minimum", `{"n": 0}`, false},
		{"missing required", `{}`, false},
		{"truncated", `{"n": 2`, false},
		{"empty", ``, false},
		{"trailing garbage", `{"n": 2} x`, false},
	}
	for _, c := range cases {
		err := s.Validate([]byte(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestCompileSchemaRejectsBadSchema(t *testing.T) {
	if _, err := CompileSchema("bad.json", []byte(`{"type": 5}`)); err == nil {
		t.Fatal("expected an error for an invalid schema")
	}
	if _, err := CompileSchema("bad.json", []byte(`{`)); err == nil {
		t.Fatal("expected an error for malformed schema JSON")
	}
}
