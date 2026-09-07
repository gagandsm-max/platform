package coreclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// UnmarshalJSON deliberately keeps durable telemetry acknowledgements strict.
// Unlike heartbeat/status responses, acknowledgement semantics directly control
// durable spool deletion and therefore must fail closed on additive/unknown
// fields until those semantics are explicitly admitted by the Sensor contract.
func (a *UploadAck) UnmarshalJSON(data []byte) error {
	type uploadAckAlias UploadAck
	var decoded uploadAckAlias
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("durable acknowledgement contains trailing JSON data")
	}
	*a = UploadAck(decoded)
	return nil
}
