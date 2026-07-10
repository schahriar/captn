package common

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

type ObservationSchemaType interface {
	Marshal() ([]byte, error)
}

type ObservationSchema struct {
	ID     string `json:"id" jsonschema_description:"The ID for this observation. Copy verbatim from the input."`
	Answer string `json:"answer" jsonschema:"minLength=40,description=Answer given user prompt against the current snippet in concise reasoning format"`
}

func NewObservationSchema(id string, ans string) ObservationSchema {
	return ObservationSchema{
		ID:     id,
		Answer: ans,
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

type BatchObservationSchema struct {
	Observations           []ObservationSchema `json:"observations" jsonschema:"description:Observations for each provided source. Each entry's id MUST match the corresponding input ID. YOUR JOB IS TO PROVIDE A BATCH RESPONSE / NEVER MIX OBSERVATIONS UNNECESSARILY."`
	ConnectionObservations []ObservationSchema `json:"connectionObservations" jsonschema:"description:If asked to explain relationship between two nodes, provide the answer taking into account that the first connection is the parent and second connection is imported / related code used by the parent. Each entry's id MUST match the corresponding input connection ID and connection IDs are always edge followed by link IDs."`
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
