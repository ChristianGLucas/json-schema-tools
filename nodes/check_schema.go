package nodes

import (
	"context"

	"christiangeorgelucas/json-schema-tools/axiom"
	gen "christiangeorgelucas/json-schema-tools/gen"
)

// Checks that a document is itself a valid, compilable JSON Schema for the detected or selected draft, returning structured reasons when it is not. Useful for validating a schema before using it. External "$ref" resolution is disabled.
func CheckSchema(ctx context.Context, ax axiom.Context, input *gen.CheckSchemaRequest) (*gen.CheckSchemaResponse, error) {
	// An unknown draft is a bad request parameter (a processing error), not a
	// statement about the schema — surface it in "error", consistent with the
	// other nodes.
	if _, err := draftFor(input.Draft); err != nil {
		return &gen.CheckSchemaResponse{Error: err.Error()}, nil
	}

	if _, err := compileSchema(input.Schema, input.Draft, false); err != nil {
		return &gen.CheckSchemaResponse{Valid: false, Errors: compileErrorToSchemaErrors(err)}, nil
	}
	return &gen.CheckSchemaResponse{Valid: true}, nil
}
