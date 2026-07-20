package nodes

import (
	"context"
	"fmt"

	"christiangeorgelucas/json-schema-tools/axiom"
	gen "christiangeorgelucas/json-schema-tools/gen"
)

// Validates many JSON instances against a single JSON Schema in one call (the schema is compiled once) and returns a per-instance result in input order. External "$ref" resolution is disabled.
func ValidateMany(ctx context.Context, ax axiom.Context, input *gen.ValidateManyRequest) (*gen.ValidateManyResponse, error) {
	if len(input.Schema) > maxSchemaBytes {
		return &gen.ValidateManyResponse{Error: fmt.Sprintf("schema exceeds size limit of %d bytes", maxSchemaBytes)}, nil
	}
	if len(input.Instances) > maxInstancesCount {
		return &gen.ValidateManyResponse{Error: fmt.Sprintf("too many instances: %d exceeds limit of %d", len(input.Instances), maxInstancesCount)}, nil
	}
	total := 0
	for _, s := range input.Instances {
		total += len(s)
	}
	if total > maxInstancesBytes {
		return &gen.ValidateManyResponse{Error: fmt.Sprintf("instances total %d bytes, exceeds limit of %d", total, maxInstancesBytes)}, nil
	}

	sch, err := compileSchema(input.Schema, input.Draft, input.AssertFormats)
	if err != nil {
		return &gen.ValidateManyResponse{Error: err.Error()}, nil
	}

	results := make([]*gen.InstanceResult, 0, len(input.Instances))
	for i, instanceText := range input.Instances {
		res := &gen.InstanceResult{Index: int32(i)}
		if len(instanceText) > maxInstanceBytes {
			res.Error = fmt.Sprintf("instance exceeds size limit of %d bytes", maxInstanceBytes)
			results = append(results, res)
			continue
		}
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
