package common

import (
	"github.com/invopop/jsonschema"
)

type ObservationSchemaType interface {
	Serialize() (string, error)
}

type ObservationSchema struct {
	Behavior  string   `json:"behavior" jsonschema:"minLength=40,description=Behavior of the code described in concise reasoning format. Prefer bullet-points"`
	EdgeCases []string `json:"edge_cases" jsonschema:"description=Important edge cases, if any with a concise description"`
}

func (o ObservationSchema) GetSchema() *jsonschema.Schema {
	return jsonschema.Reflect(o)
}

func (o ObservationSchema) Serialize() (string, error) {
	scma := o.GetSchema()
	b, err := scma.MarshalJSON()

	if err != nil {
		return "", err
	}

	return string(b), nil
}

type BatchObservationSchema struct {
	Observations           map[string]ObservationSchema `json:"observations" jsonschema:"description:Observations keyed by the provided file ID. YOUR JOB IS TO PROVIDE A BATCH RESPONSE / NEVER MIX OBSERVATIONS UNNECESSARILY."`
	ConnectionObservations map[string]ObservationSchema `json:"connectionObservations" jsonschema:"description:If asked to explain relationship between two nodes, provide the behavior taking into account that the first connection is the parent and second connection is imported / related code used by the parent."`
}

func (o *BatchObservationSchema) GetSchema() *jsonschema.Schema {
	return jsonschema.Reflect(o)
}

func (o *BatchObservationSchema) Serialize() (string, error) {
	scma := o.GetSchema()
	b, err := scma.MarshalJSON()

	if err != nil {
		return "", err
	}

	return string(b), nil
}

var _ ObservationSchemaType = &ObservationSchema{}
