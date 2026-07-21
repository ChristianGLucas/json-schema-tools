package nodes_test

import (
	"context"
	"strings"
	"testing"

	gen "christiangeorgelucas/json-schema-tools/gen"
	"christiangeorgelucas/json-schema-tools/axiom"
	"christiangeorgelucas/json-schema-tools/nodes"
)

// testContext is a testing.T-backed axiom.Context for unit tests. Populate
// secretsMap with any secrets your node needs during the test.
type testContext struct {
	t          *testing.T
	secretsMap map[string]string
}

func newTestContext(t *testing.T) *testContext {
	return &testContext{t: t, secretsMap: map[string]string{}}
}

// testLogger forwards log output to testing.T so it is captured per-test.
type testLogger struct{ t *testing.T }

func (l *testLogger) Debug(msg string, args ...any) { l.t.Logf("DEBUG  %s %v", msg, args) }
func (l *testLogger) Info(msg string, args ...any)  { l.t.Logf("INFO   %s %v", msg, args) }
func (l *testLogger) Warn(msg string, args ...any)  { l.t.Logf("WARN   %s %v", msg, args) }
func (l *testLogger) Error(msg string, args ...any) { l.t.Logf("ERROR  %s %v", msg, args) }

// testSecrets is a simple in-memory axiom.Secrets backed by testContext.secretsMap.
type testSecrets struct{ m map[string]string }

func (s testSecrets) Get(name string) (string, bool) { v, ok := s.m[name]; return v, ok }

// testFlowReflection is an empty running-flow view — no graph in a unit test.
// Override its methods (via a custom axiom.FlowReflection) in a specific test
// if your node reads ax.Reflection().Flow() (ADR-050/055).
type testFlowReflection struct{}

func (testFlowReflection) Nodes() []axiom.ReflectionNode     { return nil }
func (testFlowReflection) Edges() []axiom.ReflectionEdge     { return nil }
func (testFlowReflection) LoopEdges() []axiom.ReflectionEdge { return nil }
func (testFlowReflection) Position() axiom.FlowPosition      { return axiom.FlowPosition{} }
func (testFlowReflection) GraphID() string                   { return "" }

type testReflection struct{}

func (testReflection) Flow() axiom.FlowReflection { return testFlowReflection{} }

// testFlowMutation is a no-op mutation sink. If your node is mutation-capable,
// replace it with a recorder you assert on to verify it called AddNode/AddEdge
// with the expected package + condition (ADR-051/054).
type testFlowMutation struct{}

func (testFlowMutation) AddNode(_, _ string, _ *axiom.CanvasPosition) uint32 { return 0 }
func (testFlowMutation) AddEdge(_, _ uint32, _ *axiom.EdgeCondition)         {}

type testMutation struct{}

func (testMutation) Flow() axiom.FlowMutation { return testFlowMutation{} }

func (c *testContext) Log() axiom.Logger            { return &testLogger{c.t} }
func (c *testContext) Secrets() axiom.Secrets       { return testSecrets{c.secretsMap} }
func (c *testContext) ExecutionID() string          { return "test-execution-id" }
func (c *testContext) FlowID() string               { return "test-flow-id" }
func (c *testContext) TenantID() string             { return "test-tenant-id" }
func (c *testContext) Reflection() axiom.Reflection { return testReflection{} }
func (c *testContext) Mutation() axiom.Mutation     { return testMutation{} }

// TESTS — delete this block when done ─────────────────────────────────────────
// Tests are required to push this package. The push pipeline runs your
// tests as a quality gate — a package will not be pushed if tests fail or
// do not meet the minimum requirements.
//
// Requirements checked before pushing:
//   - At least one test per node
//   - All tests must pass
//   - Output fields must be meaningfully asserted — not just error-checked
//
// The generated test below is a starting point. Replace the TODO comment with
// real assertions that verify your node returns correct data for known inputs.
// Think: given a specific input, what should the output fields contain?
//
// Run your tests locally at any time:
//   axiom test

func validate(t *testing.T, req *gen.ValidateRequest) *gen.ValidateResponse {
	t.Helper()
	got, err := nodes.Validate(context.Background(), newTestContext(t), req)
	if err != nil {
		t.Fatalf("Validate returned a transport error (should never happen): %v", err)
	}
	if got == nil {
		t.Fatal("Validate returned nil response")
	}
	return got
}

// TestValidate_IndependentOracle checks the node against results computed BY HAND
// from the JSON Schema specification — an oracle independent of the wrapped
// library. Each want value is what the spec says, not what santhosh-tekuri says.
func TestValidate_IndependentOracle(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		instance  string
		wantValid bool
	}{
		// type + numeric bounds
		{"integer-min-pass", `{"type":"integer","minimum":5}`, `7`, true},
		{"integer-min-fail", `{"type":"integer","minimum":5}`, `3`, false},
		{"integer-rejects-float", `{"type":"integer"}`, `7.5`, false},
		{"number-multipleOf-pass", `{"type":"number","multipleOf":2}`, `4`, true},
		{"number-multipleOf-fail", `{"type":"number","multipleOf":2}`, `5`, false},
		{"exclusiveMaximum-fail", `{"type":"number","exclusiveMaximum":10}`, `10`, false},
		{"exclusiveMaximum-pass", `{"type":"number","exclusiveMaximum":10}`, `9.999`, true},
		// strings
		{"maxLength-fail", `{"type":"string","maxLength":3}`, `"abcd"`, false},
		{"maxLength-pass", `{"type":"string","maxLength":3}`, `"abc"`, true},
		{"pattern-pass", `{"type":"string","pattern":"^[0-9]+$"}`, `"12345"`, true},
		{"pattern-fail", `{"type":"string","pattern":"^[0-9]+$"}`, `"12a45"`, false},
		{"enum-pass", `{"enum":["a","b"]}`, `"a"`, true},
		{"enum-fail", `{"enum":["a","b"]}`, `"c"`, false},
		{"const-pass", `{"const":42}`, `42`, true},
		{"const-fail", `{"const":42}`, `43`, false},
		// objects
		{"required-pass", `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`, `{"name":"x"}`, true},
		{"required-fail-missing", `{"type":"object","required":["name"]}`, `{}`, false},
		{"required-fail-wrongtype", `{"type":"object","properties":{"name":{"type":"string"}}}`, `{"name":5}`, false},
		{"additionalProperties-false-fail", `{"type":"object","properties":{"a":{}},"additionalProperties":false}`, `{"a":1,"b":2}`, false},
		// arrays
		{"array-items-pass", `{"type":"array","items":{"type":"number"},"minItems":2}`, `[1,2,3]`, true},
		{"array-minItems-fail", `{"type":"array","minItems":2}`, `[1]`, false},
		{"array-item-type-fail", `{"type":"array","items":{"type":"number"}}`, `[1,"x"]`, false},
		{"uniqueItems-fail", `{"type":"array","uniqueItems":true}`, `[1,1]`, false},
		// combinators
		{"anyOf-pass", `{"anyOf":[{"type":"string"},{"type":"number"}]}`, `5`, true},
		{"anyOf-fail", `{"anyOf":[{"type":"string"},{"type":"number"}]}`, `true`, false},
		{"not-pass", `{"not":{"type":"string"}}`, `5`, true},
		{"not-fail", `{"not":{"type":"string"}}`, `"s"`, false},
		// null / boolean handling
		{"null-pass", `{"type":"null"}`, `null`, true},
		{"null-fail", `{"type":"null"}`, `0`, false},
		{"empty-schema-accepts-anything", `{}`, `{"anything":[1,2,3]}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validate(t, &gen.ValidateRequest{Schema: tc.schema, Instance: tc.instance})
			if got.Error != "" {
				t.Fatalf("unexpected processing error: %s", got.Error)
			}
			if got.Valid != tc.wantValid {
				t.Errorf("valid = %v, want %v (errors: %v)", got.Valid, tc.wantValid, got.Errors)
			}
			if !tc.wantValid && len(got.Errors) == 0 {
				t.Errorf("invalid instance produced no structured errors")
			}
			if tc.wantValid && len(got.Errors) != 0 {
				t.Errorf("valid instance unexpectedly produced errors: %v", got.Errors)
			}
		})
	}
}

// TestValidate_GoldenErrorPaths pins the exact structured error location for a
// known-failing nested instance — a deterministic golden output.
func TestValidate_GoldenErrorPaths(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{
			"user":{
				"type":"object",
				"properties":{"age":{"type":"integer","minimum":0}},
				"required":["age"]
			}
		},
		"required":["user"]
	}`
	got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `{"user":{"age":-3}}`})
	if got.Valid {
		t.Fatal("expected invalid, got valid")
	}
	found := false
	for _, e := range got.Errors {
		if e.InstancePath == "/user/age" {
			found = true
			if !strings.Contains(e.KeywordPath, "minimum") {
				t.Errorf("keyword_path = %q, want it to reference 'minimum'", e.KeywordPath)
			}
			if e.Message == "" {
				t.Errorf("error at /user/age has empty message")
			}
		}
	}
	if !found {
		t.Errorf("no error reported at instance_path /user/age; got %+v", got.Errors)
	}
}

// TestValidate_SecurityExternalRefsBlocked proves a caller cannot use "$ref" to
// read local files (file://) or reach the network (http/https) — the SSRF/file-
// read guard. A blocked ref must surface as a processing error, never a fetch.
func TestValidate_SecurityExternalRefsBlocked(t *testing.T) {
	for _, ref := range []string{"file:///etc/passwd", "https://example.com/schema.json", "http://169.254.169.254/latest/meta-data/"} {
		schema := `{"$ref":"` + ref + `"}`
		got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `{"x":1}`})
		if got.Error == "" {
			t.Errorf("external $ref %q: expected a processing error, got none (valid=%v)", ref, got.Valid)
		}
		if got.Valid {
			t.Errorf("external $ref %q: must not report valid=true", ref)
		}
		if !strings.Contains(got.Error, "not allowed") {
			t.Errorf("external $ref %q: error = %q, want it to mention the ref is not allowed", ref, got.Error)
		}
	}
}

// TestValidate_InternalRefResolves confirms internal "$ref" (the legitimate,
// in-document case) still works after external refs are blocked.
func TestValidate_InternalRefResolves(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{"a":{"$ref":"#/$defs/positive"},"b":{"$ref":"#/$defs/positive"}},
		"$defs":{"positive":{"type":"integer","minimum":1}}
	}`
	if got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `{"a":5,"b":2}`}); !got.Valid || got.Error != "" {
		t.Errorf("internal $ref valid case: valid=%v error=%q", got.Valid, got.Error)
	}
	if got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `{"a":5,"b":0}`}); got.Valid {
		t.Errorf("internal $ref invalid case: expected invalid for b=0")
	}
}

// TestValidate_Determinism runs the same input repeatedly and requires identical
// output — determinism is a claimed property.
func TestValidate_Determinism(t *testing.T) {
	req := &gen.ValidateRequest{
		Schema:   `{"type":"object","required":["a","b"],"properties":{"a":{"type":"integer"},"b":{"type":"string","minLength":2}}}`,
		Instance: `{"a":"nope","b":"x"}`,
	}
	first := validate(t, req)
	if first.Valid {
		t.Fatal("fixture should be invalid")
	}
	if len(first.Errors) < 2 {
		t.Fatalf("fixture should surface multiple errors to exercise ordering, got %d", len(first.Errors))
	}
	firstKey := errorKey(first.Errors)
	for i := 0; i < 50; i++ {
		got := validate(t, req)
		// Full ordered comparison: the exact sequence of (path, keyword, message)
		// must be byte-identical every run, not merely the same count.
		if k := errorKey(got.Errors); k != firstKey {
			t.Fatalf("nondeterministic output on run %d:\n first: %s\n got:   %s", i, firstKey, k)
		}
	}
}

// errorKey renders the ordered error list into a single comparable string.
func errorKey(errs []*gen.SchemaError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.InstancePath + "|" + e.KeywordPath + "|" + e.Message
	}
	return strings.Join(parts, "\n")
}

// TestValidate_FormatAssertion backs the assert_formats claim: "format" is an
// annotation (does not fail) by default, and asserted only when requested.
func TestValidate_FormatAssertion(t *testing.T) {
	schema := `{"type":"string","format":"email"}`
	instance := `"definitely-not-an-email"`
	if got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: instance}); !got.Valid {
		t.Errorf("assert_formats=false: expected valid (format is annotation-only), got invalid: %v", got.Errors)
	}
	if got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: instance, AssertFormats: true}); got.Valid {
		t.Errorf("assert_formats=true: expected invalid for a malformed email")
	}
	// A well-formed email passes even when formats are asserted.
	if got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `"a@b.com"`, AssertFormats: true}); !got.Valid {
		t.Errorf("assert_formats=true: valid email should pass: %v", got.Errors)
	}
}

// TestValidate_DraftSelection checks explicit draft selection and that an
// in-document "$schema" is honored.
func TestValidate_DraftSelection(t *testing.T) {
	// Draft 2020-12 prefixItems tuple validation.
	tuple := `{"type":"array","prefixItems":[{"type":"integer"},{"type":"string"}]}`
	if got := validate(t, &gen.ValidateRequest{Schema: tuple, Instance: `[1,"x"]`, Draft: "2020"}); !got.Valid {
		t.Errorf("draft 2020 prefixItems valid case failed: %v %q", got.Errors, got.Error)
	}
	if got := validate(t, &gen.ValidateRequest{Schema: tuple, Instance: `["x",1]`, Draft: "2020"}); got.Valid {
		t.Errorf("draft 2020 prefixItems should reject wrong element types")
	}
	// Draft 7 explicit selection on a simple schema.
	if got := validate(t, &gen.ValidateRequest{Schema: `{"type":"integer"}`, Instance: `5`, Draft: "7"}); !got.Valid {
		t.Errorf("draft 7 simple case failed: %v %q", got.Errors, got.Error)
	}
	// Unknown draft is a processing error.
	if got := validate(t, &gen.ValidateRequest{Schema: `{}`, Instance: `1`, Draft: "99"}); got.Error == "" {
		t.Errorf("unknown draft should produce a processing error")
	}
}

// TestValidate_ErrorPaths covers malformed input and size limits — the
// structured error contract, never a crash.
func TestValidate_ErrorPaths(t *testing.T) {
	// Malformed schema JSON.
	if got := validate(t, &gen.ValidateRequest{Schema: `{"type":`, Instance: `1`}); got.Error == "" || got.Valid {
		t.Errorf("malformed schema: want error+invalid, got error=%q valid=%v", got.Error, got.Valid)
	}
	// Malformed instance JSON.
	if got := validate(t, &gen.ValidateRequest{Schema: `{"type":"integer"}`, Instance: `{bad`}); got.Error == "" || got.Valid {
		t.Errorf("malformed instance: want error+invalid, got error=%q valid=%v", got.Error, got.Valid)
	}
	// Schema that violates its meta-schema is a processing error, not a panic.
	if got := validate(t, &gen.ValidateRequest{Schema: `{"minLength":"five"}`, Instance: `"x"`}); got.Error == "" {
		t.Errorf("schema violating meta-schema should produce a processing error")
	}
	// Oversize schema.
	big := `{"type":"string","description":"` + strings.Repeat("x", 1<<20) + `"}`
	if got := validate(t, &gen.ValidateRequest{Schema: big, Instance: `"x"`}); got.Error == "" || !strings.Contains(got.Error, "size limit") {
		t.Errorf("oversize schema: want size-limit error, got %q", got.Error)
	}
	// Oversize instance.
	bigInst := `"` + strings.Repeat("y", (4<<20)+10) + `"`
	if got := validate(t, &gen.ValidateRequest{Schema: `{"type":"string"}`, Instance: bigInst}); got.Error == "" || !strings.Contains(got.Error, "size limit") {
		t.Errorf("oversize instance: want size-limit error, got %q", got.Error)
	}
}

// TestValidate_NonObjectSchemaDocument: a schema document that is valid JSON
// but not an object or boolean (array, string, number, null) is not a legal
// JSON Schema document at all. Regression test for a nil-pointer panic found
// by adversarial review in the shared compileSchema helper (it previously
// crashed the whole process on this shape instead of returning the
// structured error this package's axiom.yaml description promises).
func TestValidate_NonObjectSchemaDocument(t *testing.T) {
	for _, schema := range []string{`[1,2,3]`, `[]`, `"hello"`, `42`, `null`} {
		got := validate(t, &gen.ValidateRequest{Schema: schema, Instance: `1`})
		if got.Error == "" {
			t.Errorf("schema %s: expected a structured error, got none (valid=%v)", schema, got.Valid)
		}
		if got.Valid {
			t.Errorf("schema %s: expected valid=false alongside the error", schema)
		}
	}
}

var _ axiom.Context = (*testContext)(nil) // compile-time interface check
