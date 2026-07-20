package nodes

import (
	"encoding/json"
	"fmt"
	"strings"

	gen "christiangeorgelucas/json-schema-tools/gen"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Size limits bound the caller-controlled cost of a request. They are checked
// against the RAW input string length, before any JSON parsing or compilation.
const (
	maxSchemaBytes    = 1 << 20  // 1 MiB per schema document
	maxInstanceBytes  = 4 << 20  // 4 MiB per instance
	maxInstancesCount = 1000     // instances per ValidateMany call
	maxInstancesBytes = 16 << 20 // 16 MiB total across a ValidateMany batch
)

// schemaBaseURL is an in-memory base URI for the compiled schema. Using a
// custom scheme keeps compilation entirely in memory: no filesystem or network
// base is ever consulted.
const schemaBaseURL = "mem://schema"

// denyLoader refuses every external reference. The compiler consults it only
// for a "$ref" whose target is neither an embedded meta-schema nor internal to
// the in-memory schema resource. Returning an error here prevents SSRF via
// http(s):// refs and local file reads via file:// refs — only references
// internal to the supplied document are ever resolved.
type denyLoader struct{}

func (denyLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external reference %q is not allowed; only references internal to the schema are resolved", url)
}

// draftFor maps a caller-supplied draft name to a library draft. An empty name
// selects Draft 2020-12. It is used only for schemas that omit "$schema"; a
// schema that declares "$schema" always uses the draft it names.
func draftFor(name string) (*jsonschema.Draft, error) {
	switch strings.TrimSpace(name) {
	case "", "2020", "2020-12":
		return jsonschema.Draft2020, nil
	case "2019", "2019-09":
		return jsonschema.Draft2019, nil
	case "7":
		return jsonschema.Draft7, nil
	case "6":
		return jsonschema.Draft6, nil
	case "4":
		return jsonschema.Draft4, nil
	default:
		return nil, fmt.Errorf("unknown draft %q; use one of 4, 6, 7, 2019, 2020", name)
	}
}

// compileSchema parses and compiles the schema text into a validator. External
// references are rejected (denyLoader) and, when assertFormats is true, "format"
// keywords are asserted rather than treated as annotations. A returned error is
// suitable for the caller-facing "error" field (malformed JSON, rejected ref,
// or a schema that does not conform to its meta-schema).
func compileSchema(schemaText, draft string, assertFormats bool) (*jsonschema.Schema, error) {
	d, err := draftFor(draft)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaText))
	if err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(denyLoader{}) // block file:// and http(s):// $ref resolution
	c.DefaultDraft(d)
	if assertFormats {
		c.AssertFormat()
	}
	if err := c.AddResource(schemaBaseURL, doc); err != nil {
		return nil, fmt.Errorf("invalid schema: %v", err)
	}
	sch, err := c.Compile(schemaBaseURL)
	if err != nil {
		return nil, err
	}
	return sch, nil
}

// validateInstance validates one instance JSON string against a compiled schema.
// It returns (valid, errors, procErr). procErr is non-nil only when the instance
// itself is not valid JSON (a processing failure, not a validation failure).
func validateInstance(sch *jsonschema.Schema, instanceText string) (bool, []*gen.SchemaError, error) {
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(instanceText))
	if err != nil {
		return false, nil, fmt.Errorf("instance is not valid JSON: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		var ve *jsonschema.ValidationError
		if asValidationError(err, &ve) {
			return false, toSchemaErrors(ve), nil
		}
		// Non-validation error (should not occur for a compiled schema); surface
		// it as a single structured error rather than crashing.
		return false, []*gen.SchemaError{{Message: err.Error()}}, nil
	}
	return true, nil, nil
}

// asValidationError reports whether err is a *jsonschema.ValidationError and, if
// so, stores it in *target.
func asValidationError(err error, target **jsonschema.ValidationError) bool {
	ve, ok := err.(*jsonschema.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

// outUnit mirrors the library's Basic output structure. The library's
// OutputError marshals to a JSON string (its message), so decoding into a
// plain string field yields a stable, human-readable message without relying
// on any unexported printer state.
type outUnit struct {
	Valid            bool      `json:"valid"`
	KeywordLocation  string    `json:"keywordLocation"`
	InstanceLocation string    `json:"instanceLocation"`
	Error            string    `json:"error"`
	Errors           []outUnit `json:"errors"`
}

// toSchemaErrors flattens a ValidationError into the leaf failures that carry a
// message, each with its instance and schema-keyword JSON Pointers.
func toSchemaErrors(ve *jsonschema.ValidationError) []*gen.SchemaError {
	raw, err := json.Marshal(ve.BasicOutput())
	if err != nil {
		return []*gen.SchemaError{{Message: ve.Error()}}
	}
	var root outUnit
	if err := json.Unmarshal(raw, &root); err != nil {
		return []*gen.SchemaError{{Message: ve.Error()}}
	}
	var errs []*gen.SchemaError
	var walk func(u outUnit)
	walk = func(u outUnit) {
		if strings.TrimSpace(u.Error) != "" {
			errs = append(errs, &gen.SchemaError{
				InstancePath: u.InstanceLocation,
				KeywordPath:  u.KeywordLocation,
				Message:      u.Error,
			})
		}
		for _, c := range u.Errors {
			walk(c)
		}
	}
	walk(root)
	if len(errs) == 0 {
		errs = append(errs, &gen.SchemaError{
			InstancePath: root.InstanceLocation,
			KeywordPath:  root.KeywordLocation,
			Message:      ve.Error(),
		})
	}
	return errs
}

// compileErrorToSchemaErrors turns a compile failure into structured errors for
// CheckSchema. A meta-schema violation carries a nested ValidationError that we
// expand; any other compile error becomes a single message.
func compileErrorToSchemaErrors(err error) []*gen.SchemaError {
	if sve, ok := err.(*jsonschema.SchemaValidationError); ok {
		if ve, ok := sve.Err.(*jsonschema.ValidationError); ok {
			return toSchemaErrors(ve)
		}
	}
	return []*gen.SchemaError{{Message: err.Error()}}
}
