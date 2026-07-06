package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func decodeStrictObject(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read object start: %w", err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return errors.New("arguments must be a JSON object")
	}

	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("read field name: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("argument field name must be a string")
		}
		if _, exists := fields[key]; exists {
			return fmt.Errorf("duplicate argument field %q", key)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode argument field %q: %w", key, err)
		}
		fields[key] = value
	}

	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read object end: %w", err)
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return errors.New("arguments object is not closed")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("arguments contain trailing JSON")
		}
		return fmt.Errorf("read trailing JSON: %w", err)
	}

	normalized, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("normalize arguments: %w", err)
	}
	strictDecoder := json.NewDecoder(bytes.NewReader(normalized))
	strictDecoder.DisallowUnknownFields()
	if err := strictDecoder.Decode(destination); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	return nil
}
