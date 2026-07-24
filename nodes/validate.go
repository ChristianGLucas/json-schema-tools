package nodes

import (
	"context"

	"christiangeorgelucas/json-schema-tools/axiom"
	gen "christiangeorgelucas/json-schema-tools/gen"
)

// Validates a JSON instance against a JSON Schema (Draft 4/6/7/2019-09/2020-12) and returns whether it conforms plus structured errors. External "$ref" resolution is disabled, so validation is offline and deterministic.
func Validate(ctx context.Context, ax axiom.Context, input *gen.ValidateRequest) (*gen.ValidateResponse, error) {
	sch, err := compileSchema(input.Schema, input.Draft, input.AssertFormats)
	if err != nil {
		return &gen.ValidateResponse{Error: err.Error()}, nil
	}

	valid, errs, procErr := validateInstance(sch, input.Instance)
	if procErr != nil {
		return &gen.ValidateResponse{Error: procErr.Error()}, nil
	}
	return &gen.ValidateResponse{Valid: valid, Errors: errs}, nil
}
