package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func readProjectList[T any](file string) ([]T, error) {
	if file == "" {
		return nil, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var entries []T
	if err := dec.Decode(&entries); err != nil || entries == nil || dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("invalid GitHub project list in %s", file)
	}
	return entries, nil
}
