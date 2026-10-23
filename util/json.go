package util

import (
	"encoding/json"
	"errors"
	"io"
)

// DecodeJSON reads one JSON value, rejecting unknown fields and trailing data.
func DecodeJSON(input io.Reader, value any) error {
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("must contain one JSON value")
	}
	return nil
}
