package nodes

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
	// A JSON Schema document is either an object (the normal case) or, from
	// Draft 6 onward, the bare boolean literal "true"/"false". Any other
	// top-level JSON shape (array, string, number, null) is not a valid
	// schema document at all — and, critically, the underlying compiler
	// assumes this invariant and panics (nil-pointer dereference deep in its
	// meta-schema resolution) rather than erroring if it is violated. Reject
	// it here, before it ever reaches the compiler, so malformed input always
	// yields a structured error instead of crashing the whole process.
	switch doc.(type) {
	case bool, map[string]any:
		// ok
	default:
		return nil, fmt.Errorf("schema must be a JSON object or boolean (true/false), got %s", jsonKindName(doc))
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

// jsonKindName names the JSON kind of a value decoded by jsonschema.UnmarshalJSON
// (which uses json.Number for numbers), for a clear error message.
func jsonKindName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case string:
		return "string"
	case json.Number:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
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
	// The library's Basic output derives from Go map iteration, whose order is
	// randomized, so sort into a stable order to honor the deterministic-output
	// contract: identical input always yields byte-identical output.
	sort.SliceStable(errs, func(i, j int) bool {
		if errs[i].InstancePath != errs[j].InstancePath {
			return errs[i].InstancePath < errs[j].InstancePath
		}
		if errs[i].KeywordPath != errs[j].KeywordPath {
			return errs[i].KeywordPath < errs[j].KeywordPath
		}
		return errs[i].Message < errs[j].Message
	})
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

// describeSchema reads structural metadata off an already-compiled schema —
// draft, root type, documentation annotations, and top-level properties — for
// DescribeSchema. It never validates an instance; it only reports what the
// compiler already parsed out of the schema document itself.
//
// A root-level "$ref" is resolved through exactly one level (never
// recursively — the compiled schema graph can contain real Go-pointer cycles
// for recursive schemas, e.g. via $ref/$dynamicRef, so an unbounded walk
// would risk an infinite loop) so a schema of the common shape
// {"$ref": "#/$defs/Foo"} still reports Foo's shape rather than nothing.
func describeSchema(sch *jsonschema.Schema) *gen.DescribeSchemaResponse {
	resp := &gen.DescribeSchemaResponse{Draft: strconv.Itoa(sch.DraftVersion)}

	if sch.Bool != nil {
		// A boolean schema ("true"/"false"): no type/properties/annotations.
		resp.IsBooleanSchema = true
		resp.BooleanSchemaValue = *sch.Bool
		return resp
	}

	resp.HasRef = sch.Ref != nil
	eff := sch // the schema to read type/required/properties from
	if sch.Ref != nil {
		eff = sch.Ref
	}

	if eff.Types != nil {
		resp.Types = eff.Types.ToStrings()
	}
	resp.Required = append([]string{}, eff.Required...)
	if len(eff.Properties) > 0 {
		names := make([]string, 0, len(eff.Properties))
		for name := range eff.Properties {
			names = append(names, name)
		}
		sort.Strings(names) // deterministic output: map iteration order is randomized
		resp.Properties = make([]*gen.SchemaProperty, 0, len(names))
		for _, name := range names {
			propSch := eff.Properties[name]
			propEff := propSch
			if propSch.Ref != nil {
				propEff = propSch.Ref
			}
			var types []string
			if propEff.Types != nil {
				types = propEff.Types.ToStrings()
			}
			resp.Properties = append(resp.Properties, &gen.SchemaProperty{Name: name, Types: types})
		}
	}

	// Documentation annotations: prefer the root schema's own value; for a
	// bare {"$ref": ...} root that declares none of its own, fall back to the
	// referenced schema's.
	title, desc := sch.Title, sch.Description
	deprecated, readOnly, writeOnly := sch.Deprecated, sch.ReadOnly, sch.WriteOnly
	def, examples := sch.Default, sch.Examples
	if sch.Ref != nil {
		if title == "" {
			title = eff.Title
		}
		if desc == "" {
			desc = eff.Description
		}
		deprecated = deprecated || eff.Deprecated
		readOnly = readOnly || eff.ReadOnly
		writeOnly = writeOnly || eff.WriteOnly
		if def == nil {
			def = eff.Default
		}
		if len(examples) == 0 {
			examples = eff.Examples
		}
	}
	resp.Title = title
	resp.Description = desc
	resp.Deprecated = deprecated
	resp.ReadOnly = readOnly
	resp.WriteOnly = writeOnly
	if def != nil {
		resp.HasDefault = true
		if b, err := json.Marshal(*def); err == nil {
			resp.DefaultJson = string(b)
		}
	}
	resp.ExamplesCount = int32(len(examples))
	return resp
}
