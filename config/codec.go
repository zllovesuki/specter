package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"go.miragespace.co/specter/util"
)

const snapshotFormat = 1

type snapshot struct {
	Format  uint64          `json:"format"`
	Records snapshotRecords `json:"records"`
}

type snapshotRecords []Record

func (records *snapshotRecords) UnmarshalJSON(data []byte) error {
	if *records != nil {
		return errors.New("configuration snapshot repeats records")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('[') {
		return errors.New("configuration records must be an array")
	}
	*records = make([]Record, 0)
	for decoder.More() {
		if len(*records) == MaxDocuments {
			return ErrTooLarge
		}
		var record Record
		if err := decoder.Decode(&record); err != nil {
			return err
		}
		*records = append(*records, record)
	}
	_, err = decoder.Token()
	return err
}

// EncodeSnapshot validates and bounds the complete wire envelope. Documents are
// opaque; no consumer-specific decoding occurs here.
func EncodeSnapshot(records []Record) ([]byte, error) {
	if err := checkRecords(records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []Record{}
	}
	return json.Marshal(snapshot{Format: snapshotFormat, Records: records})
}

// DecodeSnapshot bounds bytes before parsing and document count before
// allocation, then validates every record, including losing duplicate versions.
func DecodeSnapshot(data []byte) ([]Record, error) {
	if len(data) > MaxSnapshotBytes {
		return nil, ErrTooLarge
	}
	var state snapshot
	if err := util.DecodeJSON(bytes.NewReader(data), &state); err != nil {
		return nil, fmt.Errorf("decode configuration snapshot: %w", err)
	}
	if state.Format != snapshotFormat || state.Records == nil {
		return nil, errors.New("unsupported configuration snapshot format")
	}
	if err := checkRecords(state.Records); err != nil {
		return nil, err
	}
	return state.Records, nil
}

func checkRecords(records []Record) error {
	if len(records) > MaxDocuments {
		return ErrTooLarge
	}
	size := len(`{"format":1,"records":[]}`)
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		size += len(encoded)
		if i != 0 {
			size++
		}
		if size > MaxSnapshotBytes {
			return fmt.Errorf("%w: snapshot exceeds %d bytes", ErrTooLarge, MaxSnapshotBytes)
		}
	}
	return nil
}
