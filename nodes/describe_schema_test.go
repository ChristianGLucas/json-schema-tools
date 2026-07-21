package nodes_test

import (
	"encoding/json"
	"testing"

	gen "christiangeorgelucas/json-schema-tools/gen"
	"christiangeorgelucas/json-schema-tools/nodes"
	"context"
)

func describeSchema(t *testing.T, req *gen.CheckSchemaRequest) *gen.DescribeSchemaResponse {
	t.Helper()
	got, err := nodes.DescribeSchema(context.Background(), newTestContext(t), req)
	if err != nil {
		t.Fatalf("DescribeSchema returned a transport error (should never happen): %v", err)
	}
	if got == nil {
		t.Fatal("DescribeSchema returned nil response")
	}
	return got
}

// independentDescribe re-derives the same facts DescribeSchema reports, but
// via a wholly separate code path: plain encoding/json over the raw document,
// never touching the jsonschema library. This is the independent oracle.
type independentDescribe struct {
	draft      string
	types      []string
	title      string
	desc       string
	required   []string
	properties map[string][]string // name -> declared "type" values (as strings)
	deprecated bool
	readOnly   bool
	writeOnly  bool
	hasDefault bool
}

func mustIndependentDescribe(t *testing.T, schemaJSON string, defaultDraft string) independentDescribe {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &doc); err != nil {
		t.Fatalf("oracle: schema is not valid JSON object: %v", err)
	}
	out := independentDescribe{properties: map[string][]string{}}
	switch s, _ := doc["$schema"].(string); s {
	case "https://json-schema.org/draft/2020-12/schema":
		out.draft = "2020"
	case "https://json-schema.org/draft/2019-09/schema":
		out.draft = "2019"
	case "http://json-schema.org/draft-07/schema#", "http://json-schema.org/draft-07/schema":
		out.draft = "7"
	case "":
		out.draft = defaultDraft
	}
	switch t := doc["type"].(type) {
	case string:
		out.types = []string{t}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok {
				out.types = append(out.types, s)
			}
		}
	}
	out.title, _ = doc["title"].(string)
	out.desc, _ = doc["description"].(string)
	if req, ok := doc["required"].([]any); ok {
		for _, v := range req {
			if s, ok := v.(string); ok {
				out.required = append(out.required, s)
			}
		}
	}
	if props, ok := doc["properties"].(map[string]any); ok {
		for name, raw := range props {
			propDoc, _ := raw.(map[string]any)
			var types []string
			switch t := propDoc["type"].(type) {
			case string:
				types = []string{t}
			case []any:
				for _, v := range t {
					if s, ok := v.(string); ok {
						types = append(types, s)
					}
				}
			}
			out.properties[name] = types
		}
	}
	out.deprecated, _ = doc["deprecated"].(bool)
	out.readOnly, _ = doc["readOnly"].(bool)
	out.writeOnly, _ = doc["writeOnly"].(bool)
	_, out.hasDefault = doc["default"]
	return out
}

// TestDescribeSchema_IndependentOracle checks DescribeSchema's output against
// facts extracted independently, straight off the raw JSON via encoding/json
// (never through the jsonschema library), for a schema exercising every field.
func TestDescribeSchema_IndependentOracle(t *testing.T) {
	schema := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title": "Person",
		"description": "A human being.",
		"type": "object",
		"required": ["name"],
		"deprecated": true,
		"readOnly": true,
		"default": {"name": "anon"},
		"examples": [{"name": "a"}, {"name": "b"}],
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"},
			"tags": {"type": "array"}
		}
	}`
	oracle := mustIndependentDescribe(t, schema, "2020")
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: schema})

	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.Draft != oracle.draft {
		t.Errorf("draft = %q, oracle wants %q", got.Draft, oracle.draft)
	}
	if !equalStringSlices(got.Types, oracle.types) {
		t.Errorf("types = %v, oracle wants %v", got.Types, oracle.types)
	}
	if got.Title != oracle.title {
		t.Errorf("title = %q, oracle wants %q", got.Title, oracle.title)
	}
	if got.Description != oracle.desc {
		t.Errorf("description = %q, oracle wants %q", got.Description, oracle.desc)
	}
	if !equalStringSlices(got.Required, oracle.required) {
		t.Errorf("required = %v, oracle wants %v", got.Required, oracle.required)
	}
	if got.Deprecated != oracle.deprecated {
		t.Errorf("deprecated = %v, oracle wants %v", got.Deprecated, oracle.deprecated)
	}
	if got.ReadOnly != oracle.readOnly {
		t.Errorf("read_only = %v, oracle wants %v", got.ReadOnly, oracle.readOnly)
	}
	if got.WriteOnly != oracle.writeOnly {
		t.Errorf("write_only = %v, oracle wants %v (schema declares no writeOnly)", got.WriteOnly, oracle.writeOnly)
	}
	if got.HasDefault != oracle.hasDefault {
		t.Errorf("has_default = %v, oracle wants %v", got.HasDefault, oracle.hasDefault)
	}
	if got.ExamplesCount != 2 {
		t.Errorf("examples_count = %d, want 2", got.ExamplesCount)
	}
	if len(got.Properties) != len(oracle.properties) {
		t.Fatalf("got %d properties, oracle wants %d", len(got.Properties), len(oracle.properties))
	}
	for _, p := range got.Properties {
		want, ok := oracle.properties[p.Name]
		if !ok {
			t.Errorf("unexpected property %q in output", p.Name)
			continue
		}
		if !equalStringSlices(p.Types, want) {
			t.Errorf("property %q types = %v, oracle wants %v", p.Name, p.Types, want)
		}
	}
	// Properties must be sorted by name for deterministic output.
	for i := 1; i < len(got.Properties); i++ {
		if got.Properties[i-1].Name > got.Properties[i].Name {
			t.Errorf("properties not sorted: %q before %q", got.Properties[i-1].Name, got.Properties[i].Name)
		}
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// TestDescribeSchema_DraftDetection covers each supported draft explicitly.
func TestDescribeSchema_DraftDetection(t *testing.T) {
	cases := []struct {
		schema string
		draft  string
		want   string
	}{
		{`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string"}`, "", "2020"},
		{`{"$schema":"https://json-schema.org/draft/2019-09/schema","type":"string"}`, "", "2019"},
		{`{"$schema":"http://json-schema.org/draft-07/schema#","type":"string"}`, "", "7"},
		{`{"$schema":"http://json-schema.org/draft-06/schema#","type":"string"}`, "", "6"},
		{`{"$schema":"http://json-schema.org/draft-04/schema#","type":"string"}`, "", "4"},
		{`{"type":"string"}`, "7", "7"}, // no $schema -> falls back to requested draft
		{`{"type":"string"}`, "", "2020"}, // no $schema, no draft -> default 2020-12
	}
	for _, tc := range cases {
		got := describeSchema(t, &gen.CheckSchemaRequest{Schema: tc.schema, Draft: tc.draft})
		if got.Error != "" {
			t.Fatalf("schema %s draft %q: unexpected error %s", tc.schema, tc.draft, got.Error)
		}
		if got.Draft != tc.want {
			t.Errorf("schema %s draft %q: draft = %q, want %q", tc.schema, tc.draft, got.Draft, tc.want)
		}
	}
}

// TestDescribeSchema_BooleanSchema covers the "true"/"false" whole-document
// schema form (Draft 6+): no type/properties, just the boolean flag+value.
func TestDescribeSchema_BooleanSchema(t *testing.T) {
	for _, tc := range []struct {
		schema string
		want   bool
	}{
		{"true", true},
		{"false", false},
	} {
		got := describeSchema(t, &gen.CheckSchemaRequest{Schema: tc.schema})
		if got.Error != "" {
			t.Fatalf("boolean schema %s: unexpected error %s", tc.schema, got.Error)
		}
		if !got.IsBooleanSchema {
			t.Errorf("boolean schema %s: is_boolean_schema = false, want true", tc.schema)
		}
		if got.BooleanSchemaValue != tc.want {
			t.Errorf("boolean schema %s: boolean_schema_value = %v, want %v", tc.schema, got.BooleanSchemaValue, tc.want)
		}
		if len(got.Properties) != 0 || len(got.Types) != 0 {
			t.Errorf("boolean schema %s: expected no properties/types, got properties=%v types=%v", tc.schema, got.Properties, got.Types)
		}
	}
}

// TestDescribeSchema_RootRef: a root-level "$ref" is resolved through one
// level so the referenced shape is still reported, and has_ref is set.
func TestDescribeSchema_RootRef(t *testing.T) {
	schema := `{
		"$defs": {
			"Named": {
				"type": "object",
				"required": ["id"],
				"properties": {"id": {"type": "integer"}}
			}
		},
		"$ref": "#/$defs/Named"
	}`
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: schema})
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if !got.HasRef {
		t.Errorf("has_ref = false, want true")
	}
	if !equalStringSlices(got.Types, []string{"object"}) {
		t.Errorf("types (resolved through $ref) = %v, want [object]", got.Types)
	}
	if !equalStringSlices(got.Required, []string{"id"}) {
		t.Errorf("required (resolved through $ref) = %v, want [id]", got.Required)
	}
	if len(got.Properties) != 1 || got.Properties[0].Name != "id" {
		t.Errorf("properties (resolved through $ref) = %v, want one property named id", got.Properties)
	}
}

// TestDescribeSchema_DefaultJSON: the "default" annotation, when present, is
// re-serialized faithfully as JSON text.
func TestDescribeSchema_DefaultJSON(t *testing.T) {
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":"object","default":{"a":1,"b":[true,null]}}`})
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if !got.HasDefault {
		t.Fatalf("has_default = false, want true")
	}
	var roundTrip any
	if err := json.Unmarshal([]byte(got.DefaultJson), &roundTrip); err != nil {
		t.Fatalf("default_json is not valid JSON: %v (%q)", err, got.DefaultJson)
	}
	want := map[string]any{"a": float64(1), "b": []any{true, nil}}
	gotMap, ok := roundTrip.(map[string]any)
	if !ok || len(gotMap) != 2 {
		t.Fatalf("default_json round-tripped to %v, want %v", roundTrip, want)
	}
}

// TestDescribeSchema_NoAnnotations: a minimal schema has all annotation
// fields at their zero value, and no error.
func TestDescribeSchema_NoAnnotations(t *testing.T) {
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: `{}`})
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.Title != "" || got.Description != "" || got.Deprecated || got.ReadOnly || got.WriteOnly || got.HasDefault {
		t.Errorf("empty schema should have zero-value annotations, got %+v", got)
	}
	if len(got.Types) != 0 || len(got.Required) != 0 || len(got.Properties) != 0 {
		t.Errorf("empty schema should have no types/required/properties, got %+v", got)
	}
}

// TestDescribeSchema_MalformedJSON: unparseable input is reported via error,
// not a crash, and every other field is at its zero value.
func TestDescribeSchema_MalformedJSON(t *testing.T) {
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":`})
	if got.Error == "" {
		t.Errorf("malformed JSON: expected an error")
	}
	if got.Draft != "" || len(got.Types) != 0 || got.Title != "" {
		t.Errorf("malformed JSON: expected zero-value fields alongside error, got %+v", got)
	}
}

// TestDescribeSchema_ExternalRefBlocked: an external "$ref" cannot be
// compiled, so it is reported via error — and never fetched.
func TestDescribeSchema_ExternalRefBlocked(t *testing.T) {
	for _, ref := range []string{"file:///etc/passwd", "https://example.com/s.json"} {
		got := describeSchema(t, &gen.CheckSchemaRequest{Schema: `{"$ref":"` + ref + `"}`})
		if got.Error == "" {
			t.Errorf("external $ref %q: expected an error", ref)
		}
	}
}

// TestDescribeSchema_UnknownDraft: an unrecognized draft override is a
// processing error, consistent with the other nodes.
func TestDescribeSchema_UnknownDraft(t *testing.T) {
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":"string"}`, Draft: "banana"})
	if got.Error == "" {
		t.Errorf("unknown draft: expected an error")
	}
}

// TestDescribeSchema_SizeLimit: oversized schema input is rejected before
// compilation, mirroring the other nodes' size gate.
func TestDescribeSchema_SizeLimit(t *testing.T) {
	huge := `{"type":"string","title":"` + string(make([]byte, 1<<21)) + `"}`
	got := describeSchema(t, &gen.CheckSchemaRequest{Schema: huge})
	if got.Error == "" {
		t.Errorf("oversized schema: expected a size-limit error")
	}
}

// TestDescribeSchema_NonObjectSchemaDocument: a schema document that is
// valid JSON but not an object or boolean (array, string, number, null) is
// not a legal JSON Schema document. Regression test for a nil-pointer panic
// found by adversarial review in the shared compileSchema helper (it
// previously crashed the whole process on this shape instead of reporting a
// structured error, contradicting this package's own documented "never
// crashes on malformed input" contract).
func TestDescribeSchema_NonObjectSchemaDocument(t *testing.T) {
	for _, schema := range []string{`[1,2,3]`, `[]`, `"hello"`, `42`, `null`} {
		got := describeSchema(t, &gen.CheckSchemaRequest{Schema: schema})
		if got.Error == "" {
			t.Errorf("schema %s: expected an error", schema)
		}
		if len(got.Types) != 0 || got.Title != "" || got.IsBooleanSchema {
			t.Errorf("schema %s: expected zero-value fields alongside error, got %+v", schema, got)
		}
	}
}

// TestDescribeSchema_Deterministic: identical input yields byte-identical
// output across repeated calls (property ordering in particular).
func TestDescribeSchema_Deterministic(t *testing.T) {
	schema := `{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"integer"},"m":{"type":"boolean"}}}`
	first := describeSchema(t, &gen.CheckSchemaRequest{Schema: schema})
	for i := 0; i < 5; i++ {
		got := describeSchema(t, &gen.CheckSchemaRequest{Schema: schema})
		if len(got.Properties) != len(first.Properties) {
			t.Fatalf("run %d: property count changed: %d vs %d", i, len(got.Properties), len(first.Properties))
		}
		for j := range got.Properties {
			if got.Properties[j].Name != first.Properties[j].Name {
				t.Errorf("run %d: property order changed at %d: %q vs %q", i, j, got.Properties[j].Name, first.Properties[j].Name)
			}
		}
	}
	wantOrder := []string{"a", "m", "z"}
	for i, p := range first.Properties {
		if p.Name != wantOrder[i] {
			t.Errorf("properties[%d] = %q, want %q (sorted)", i, p.Name, wantOrder[i])
		}
	}
}
