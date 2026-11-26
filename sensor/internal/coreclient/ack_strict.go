package coreclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// UnmarshalJSON deliberately keeps durable telemetry acknowledgements strict.
// Unlike heartbeat/status responses, acknowledgement semantics directly control
// durable spool deletion and therefore must fail closed on additive/unknown
// fields until those semantics are explicitly admitted by the Sensor contract.
func (a *UploadAck) UnmarshalJSON(data []byte) error {
	type uploadAckAlias UploadAck
	if err := validateAckObjectMembers(data); err != nil {
		return err
	}
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

// validateAckObjectMembers rejects ambiguous top-level members before encoding/json
// can apply its last-value-wins behavior. Token returns decoded member names, so
// escaped-equivalent names are treated as duplicates as well.
func validateAckObjectMembers(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return errors.New("durable acknowledgement must be a JSON object")
	}
	known := [...]string{"status", "accepted_through_sequence", "sensor_identity"}
	seen := make(map[string]struct{}, len(known))
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return errors.New("durable acknowledgement member name is not a string")
		}
		for _, canonical := range known {
			if name == canonical {
				if _, exists := seen[name]; exists {
					return errors.New("durable acknowledgement contains a duplicate member")
				}
				seen[name] = struct{}{}
				break
			}
			if strings.EqualFold(name, canonical) {
				return errors.New("durable acknowledgement member name is not canonical")
			}
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("durable acknowledgement contains trailing JSON data")
	}
	return nil
}
