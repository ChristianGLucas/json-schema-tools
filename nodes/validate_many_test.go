package nodes_test

import (
	"context"
	"testing"

	gen "christiangeorgelucas/json-schema-tools/gen"
	"christiangeorgelucas/json-schema-tools/nodes"
)

func validateMany(t *testing.T, req *gen.ValidateManyRequest) *gen.ValidateManyResponse {
	t.Helper()
	got, err := nodes.ValidateMany(context.Background(), newTestContext(t), req)
	if err != nil {
		t.Fatalf("ValidateMany returned a transport error (should never happen): %v", err)
	}
	if got == nil {
		t.Fatal("ValidateMany returned nil response")
	}
	return got
}

// TestValidateMany_Batch validates a mixed batch against one schema and asserts
// each per-instance result, index, and ordering. Expected valid/invalid values
// are computed by hand from the spec (independent oracle).
func TestValidateMany_Batch(t *testing.T) {
	schema := `{"type":"object","required":["id"],"properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`
	req := &gen.ValidateManyRequest{
		Schema: schema,
		Instances: []string{
			`{"id":1,"name":"a"}`, // valid
			`{"id":2}`,            // valid (name optional)
			`{"name":"x"}`,        // invalid: missing required id
			`{"id":"3"}`,          // invalid: id wrong type
			`{bad json`,           // malformed JSON -> per-instance error
		},
	}
	got := validateMany(t, req)
	if got.Error != "" {
		t.Fatalf("unexpected schema-level error: %s", got.Error)
	}
	if len(got.Results) != 5 {
		t.Fatalf("got %d results, want 5", len(got.Results))
	}
	wantValid := []bool{true, true, false, false, false}
	for i, r := range got.Results {
		if r.Index != int32(i) {
			t.Errorf("result %d has index %d, want %d", i, r.Index, i)
		}
		if r.Valid != wantValid[i] {
			t.Errorf("instance %d: valid=%v, want %v (errors=%v error=%q)", i, r.Valid, wantValid[i], r.Errors, r.Error)
		}
	}
	// Instances 2 and 3 are schema-invalid: structured errors, no processing error.
	for _, i := range []int{2, 3} {
		if len(got.Results[i].Errors) == 0 {
			t.Errorf("instance %d: expected structured validation errors", i)
		}
		if got.Results[i].Error != "" {
			t.Errorf("instance %d: unexpected processing error %q", i, got.Results[i].Error)
		}
	}
	// Instance 4 is malformed JSON: processing error, not a validation error.
	if got.Results[4].Error == "" {
		t.Errorf("instance 4 (malformed JSON): expected a per-instance processing error")
	}
	if len(got.Results[4].Errors) != 0 {
		t.Errorf("instance 4 (malformed JSON): should have no structured validation errors")
	}
}

// TestValidateMany_SchemaErrorShortCircuits: an unusable schema fails the whole
// call with a single schema-level error and no per-instance results.
func TestValidateMany_SchemaErrorShortCircuits(t *testing.T) {
	got := validateMany(t, &gen.ValidateManyRequest{Schema: `{"type":`, Instances: []string{`1`, `2`}})
	if got.Error == "" {
		t.Errorf("malformed schema: expected schema-level error")
	}
	if len(got.Results) != 0 {
		t.Errorf("malformed schema: expected no results, got %d", len(got.Results))
	}
	// External $ref in the schema is likewise rejected at the schema level.
	ext := validateMany(t, &gen.ValidateManyRequest{Schema: `{"$ref":"file:///etc/passwd"}`, Instances: []string{`1`}})
	if ext.Error == "" || len(ext.Results) != 0 {
		t.Errorf("external $ref: want schema-level error and no results, got error=%q results=%d", ext.Error, len(ext.Results))
	}
}

// TestValidateMany_LargeBatch: a large batch is not size/count-capped by the
// node (that's the platform's job) — it must process cleanly, per-instance,
// rather than crash or reject on count alone.
func TestValidateMany_LargeBatch(t *testing.T) {
	instances := make([]string, 1001)
	for i := range instances {
		instances[i] = `1`
	}
	got := validateMany(t, &gen.ValidateManyRequest{Schema: `{"type":"integer"}`, Instances: instances})
	if got.Error != "" {
		t.Errorf("1001 instances: want no processing error, got %q", got.Error)
	}
	if len(got.Results) != len(instances) {
		t.Errorf("1001 instances: want %d results, got %d", len(instances), len(got.Results))
	}
	for i, r := range got.Results {
		if !r.Valid {
			t.Errorf("instance %d: want valid, got errors=%v error=%q", i, r.Errors, r.Error)
			break
		}
	}
}

// TestValidateMany_Empty: an empty batch is valid and yields no results.
func TestValidateMany_Empty(t *testing.T) {
	got := validateMany(t, &gen.ValidateManyRequest{Schema: `{"type":"integer"}`, Instances: nil})
	if got.Error != "" || len(got.Results) != 0 {
		t.Errorf("empty batch: want no error and no results, got error=%q results=%d", got.Error, len(got.Results))
	}
}

// TestValidateMany_NonObjectSchemaDocument: a schema document that is valid
// JSON but not an object or boolean (array, string, number, null) is not a
// legal JSON Schema document — it fails the whole call with a schema-level
// error. Regression test for a nil-pointer panic found by adversarial review
// in the shared compileSchema helper (it previously crashed the whole
// process on this shape instead of short-circuiting with an error).
func TestValidateMany_NonObjectSchemaDocument(t *testing.T) {
	for _, schema := range []string{`[1,2,3]`, `[]`, `"hello"`, `42`, `null`} {
		got := validateMany(t, &gen.ValidateManyRequest{Schema: schema, Instances: []string{`1`}})
		if got.Error == "" {
			t.Errorf("schema %s: expected a schema-level error", schema)
		}
		if len(got.Results) != 0 {
			t.Errorf("schema %s: expected no results, got %d", schema, len(got.Results))
		}
	}
}
