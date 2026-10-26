package claudecfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errMalformed = errors.New("malformed JSON object")

// object is a top-level JSON object that keeps key order and every value as
// raw bytes, so replacing one key leaves all others untouched.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *object { return &object{vals: map[string]json.RawMessage{}} }

// parseObject decodes a top-level JSON object. Errors never include content.
func parseObject(data []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errMalformed
	}
	o := newObject()
	for dec.More() {
		tok, err := dec.Token()
		key, ok := tok.(string)
		if err != nil || !ok {
			return nil, errMalformed
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, errMalformed
		}
		o.set(key, raw)
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, errMalformed
	}
	return o, nil
}

// set replaces a key in place, or appends it when new.
func (o *object) set(key string, val json.RawMessage) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = append(json.RawMessage(nil), val...)
}

// encode renders the object one key per line; values are written verbatim.
func (o *object) encode() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{")
	for i, k := range o.keys {
		name, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString("\n  ")
		buf.Write(name)
		buf.WriteString(": ")
		buf.Write(o.vals[k])
	}
	if len(o.keys) > 0 {
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// readObject loads a file as an object. A missing file yields (nil, nil).
func readObject(path string) (*object, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	o, err := parseObject(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return o, nil
}

// replaceKey sets one key of the JSON object stored at path, atomically.
// A missing file is created (mode 0600); an existing file keeps its mode.
func replaceKey(path, key string, val json.RawMessage, rename func(string, string) error) error {
	o, err := readObject(path)
	if err != nil {
		return err
	}
	if o == nil {
		o = newObject()
	}
	o.set(key, val)
	data, err := o.encode()
	if err != nil {
		return err
	}
	return writeAtomic(path, data, rename)
}

// writeAtomic writes data to a temp file beside path and renames it over.
func writeAtomic(path string, data []byte, rename func(string, string) error) error {
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".cid-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	name := tmp.Name()
	fail := func(err error) error { return errors.Join(err, os.Remove(name)) }
	if err := tmp.Chmod(mode); err != nil {
		return fail(errors.Join(fmt.Errorf("set mode: %w", err), tmp.Close()))
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(errors.Join(fmt.Errorf("write temp file: %w", err), tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fail(fmt.Errorf("close temp file: %w", err))
	}
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(name, path); err != nil {
		return fail(fmt.Errorf("replace %s: %w", filepath.Base(path), err))
	}
	return nil
}
