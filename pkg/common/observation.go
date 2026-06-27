package common

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

type ObservationSchemaType interface {
	Marshal() ([]byte, error)
}

type ObservationSchema struct {
	ID       string `json:"id" jsonschema_description:"The ID for this observation. Copy verbatim from the input."`
	Behavior string `json:"behavior" jsonschema:"minLength=40,description=Behavior of the code described in concise reasoning format. Prefer bullet-points"`
}

func NewObservationSchema(id string, behavior string) ObservationSchema {
	return ObservationSchema{
		ID:       id,
		Behavior: behavior,
	}
}

func (o ObservationSchema) GetSchema() *jsonschema.Schema {
	return jsonschema.Reflect(o)
}

func (o ObservationSchema) Marshal() ([]byte, error) {
	b, err := json.Marshal(o)

	if err != nil {
		return []byte{}, err
	}

	return b, nil
}

func UnmarshalObservationSchema(bytes []byte) (ObservationSchema, error) {
	scma := NewObservationSchema("", "")

	if err := json.Unmarshal(bytes, &scma); err != nil {
		return scma, err
	}

	return scma, nil
}

type BatchObservationSchema struct {
	Observations           []ObservationSchema `json:"observations" jsonschema:"description:Observations for each provided source. Each entry's id MUST match the corresponding input ID. YOUR JOB IS TO PROVIDE A BATCH RESPONSE / NEVER MIX OBSERVATIONS UNNECESSARILY."`
	ConnectionObservations []ObservationSchema `json:"connectionObservations" jsonschema:"description:If asked to explain relationship between two nodes, provide the behavior taking into account that the first connection is the parent and second connection is imported / related code used by the parent. Each entry's id MUST match the corresponding input connection ID and connection IDs are always edge followed by link IDs."`
}

func (o *BatchObservationSchema) GetSchema() *jsonschema.Schema {
	return jsonschema.Reflect(o)
}

func (o *BatchObservationSchema) Marshal() ([]byte, error) {
	scma := o.GetSchema()
	b, err := scma.MarshalJSON()

	if err != nil {
		return []byte{}, err
	}

	return b, nil
}

var _ ObservationSchemaType = (*ObservationSchema)(nil)
