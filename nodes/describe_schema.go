package nodes

import (
	"context"

	"christiangeorgelucas/json-schema-tools/axiom"
	gen "christiangeorgelucas/json-schema-tools/gen"
)

// Compiles a JSON Schema and reports structural metadata about the schema itself — draft, root type, title/description/deprecated/readOnly/writeOnly annotations, required properties, and top-level properties with their own type constraints — without validating any instance. A root-level "$ref" is resolved through one level. External "$ref" resolution is disabled, same as the other nodes.
func DescribeSchema(ctx context.Context, ax axiom.Context, input *gen.CheckSchemaRequest) (*gen.DescribeSchemaResponse, error) {
	sch, err := compileSchema(input.Schema, input.Draft, false)
	if err != nil {
		return &gen.DescribeSchemaResponse{Error: err.Error()}, nil
	}

	return describeSchema(sch), nil
}
