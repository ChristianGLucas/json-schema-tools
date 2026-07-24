package nodes

import (
	"context"

	"christiangeorgelucas/json-schema-tools/axiom"
	gen "christiangeorgelucas/json-schema-tools/gen"
)

// Validates many JSON instances against a single JSON Schema in one call (the schema is compiled once) and returns a per-instance result in input order. External "$ref" resolution is disabled.
func ValidateMany(ctx context.Context, ax axiom.Context, input *gen.ValidateManyRequest) (*gen.ValidateManyResponse, error) {
	sch, err := compileSchema(input.Schema, input.Draft, input.AssertFormats)
	if err != nil {
		return &gen.ValidateManyResponse{Error: err.Error()}, nil
	}

	results := make([]*gen.InstanceResult, 0, len(input.Instances))
	for i, instanceText := range input.Instances {
		res := &gen.InstanceResult{Index: int32(i)}
		valid, errs, procErr := validateInstance(sch, instanceText)
		if procErr != nil {
			res.Error = procErr.Error()
		} else {
			res.Valid = valid
			res.Errors = errs
		}
		results = append(results, res)
	}
	return &gen.ValidateManyResponse{Results: results}, nil
}
