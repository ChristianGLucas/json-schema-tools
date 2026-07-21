package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/json-schema-tools/gen"
	"christiangeorgelucas/json-schema-tools/nodes"
)

func checkSchema(t *testing.T, req *gen.CheckSchemaRequest) *gen.CheckSchemaResponse {
	t.Helper()
	got, err := nodes.CheckSchema(context.Background(), newTestContext(t), req)
	if err != nil {
		t.Fatalf("CheckSchema returned a transport error (should never happen): %v", err)
	}
	if got == nil {
		t.Fatal("CheckSchema returned nil response")
	}
	return got
}

// TestCheckSchema_Valid: well-formed schemas across drafts compile cleanly.
func TestCheckSchema_Valid(t *testing.T) {
	valid := []string{
		`{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`,
		`{"type":"array","items":{"type":"integer"},"minItems":1}`,
		`{"$defs":{"n":{"type":"integer"}},"$ref":"#/$defs/n"}`,
		`true`,  // boolean schema (accepts everything)
		`false`, // boolean schema (accepts nothing) — still a valid schema
		`{}`,
	}
	for _, s := range valid {
		got := checkSchema(t, &gen.CheckSchemaRequest{Schema: s})
		if !got.Valid {
			t.Errorf("schema %s: expected valid=true, got false with errors %v", s, got.Errors)
		}
		if len(got.Errors) != 0 {
			t.Errorf("schema %s: valid schema should have no errors, got %v", s, got.Errors)
		}
	}
}

// TestCheckSchema_Invalid: documents that are not usable schemas are reported
// invalid with structured reasons. Expectations are derived by hand from the
// meta-schema (independent oracle).
func TestCheckSchema_Invalid(t *testing.T) {
	cases := []struct {
		name   string
		schema string
	}{
		{"minLength-wrong-type", `{"minLength":"five"}`},   // must be a non-negative integer
		{"type-unknown-value", `{"type":"nope"}`},          // not a JSON type
		{"type-number", `{"type":123}`},                    // type must be string/array
		{"required-not-array", `{"required":"a"}`},         // required must be an array
		{"properties-not-object", `{"properties":[1,2]}`},  // properties must be an object
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSchema(t, &gen.CheckSchemaRequest{Schema: tc.schema})
			if got.Valid {
				t.Errorf("schema %s: expected invalid, got valid", tc.schema)
			}
			if got.Error != "" {
				t.Errorf("schema %s: an invalid schema is not a processing error, but error=%q", tc.schema, got.Error)
			}
			if len(got.Errors) == 0 {
				t.Errorf("schema %s: expected structured errors explaining why it is invalid", tc.schema)
			}
		})
	}
}

// TestCheckSchema_MalformedJSON: unparseable input is reported invalid.
func TestCheckSchema_MalformedJSON(t *testing.T) {
	got := checkSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":`})
	if got.Valid {
		t.Errorf("malformed JSON: expected invalid")
	}
	if len(got.Errors) == 0 {
		t.Errorf("malformed JSON: expected an explanatory error")
	}
}

// TestCheckSchema_ExternalRefBlocked: a schema whose "$ref" points outside the
// document cannot be compiled, so it is reported invalid — and never fetched.
func TestCheckSchema_ExternalRefBlocked(t *testing.T) {
	for _, ref := range []string{"file:///etc/passwd", "https://example.com/s.json"} {
		got := checkSchema(t, &gen.CheckSchemaRequest{Schema: `{"$ref":"` + ref + `"}`})
		if got.Valid {
			t.Errorf("external $ref %q: expected invalid", ref)
		}
		if len(got.Errors) == 0 {
			t.Errorf("external $ref %q: expected an explanatory error", ref)
		}
	}
}

// TestCheckSchema_DraftDefault: draft override is accepted for a schema without
// "$schema"; an unknown draft is a processing error.
func TestCheckSchema_DraftDefault(t *testing.T) {
	if got := checkSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":"string"}`, Draft: "7"}); !got.Valid {
		t.Errorf("draft 7 simple schema should be valid: %v", got.Errors)
	}
	if got := checkSchema(t, &gen.CheckSchemaRequest{Schema: `{"type":"string"}`, Draft: "banana"}); got.Error == "" {
		t.Errorf("unknown draft should produce a processing error")
	}
}

// TestCheckSchema_NonObjectSchemaDocument: a schema document that is valid
// JSON but not an object or boolean (array, string, number, null) is not a
// legal JSON Schema document. Regression test for a nil-pointer panic found
// by adversarial review in the shared compileSchema helper (it previously
// crashed the whole process on this shape instead of reporting invalid).
func TestCheckSchema_NonObjectSchemaDocument(t *testing.T) {
	for _, schema := range []string{`[1,2,3]`, `[]`, `"hello"`, `42`, `null`} {
		got := checkSchema(t, &gen.CheckSchemaRequest{Schema: schema})
		if got.Valid {
			t.Errorf("schema %s: expected invalid, got valid", schema)
		}
		if len(got.Errors) == 0 {
			t.Errorf("schema %s: expected structured errors explaining why it is not a usable schema", schema)
		}
	}
}
