package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type jsonResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (app *Config) readJson(w http.ResponseWriter, r *http.Request, data any) error {

	maxBytes := 1048576 // 1M

	// Limits the size of the request body to 1 MB to prevent large payloads
	// from consuming too much memory.
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)

	// This line attempts to decode the first JSON value from the request body
	// into the data variable. If this decoding fails (e.g., if the JSON is
	// malformed), it returns the error immediately.
	err := dec.Decode(data)
	if err != nil {
		return err
	}

	// Purpose: Ensure single JSON object.
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must has only single JSON value")
	}

	return nil
}

func (app *Config) writeJson(w http.ResponseWriter, status int, data any, headers ...http.Header) error {
	out, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Check headers
	if len(headers) > 0 {
		fmt.Println("headers:", headers)
		for k, v := range headers[0] {
			w.Header()[k] = v
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_, err = w.Write(out)
	if err != nil {
		return err
	}

	return nil
}

// Format error message and write to response JSON contents.
func (app *Config) errorJson(w http.ResponseWriter, err error, status ...int) error {
	// Predefined status code
	statusCode := http.StatusBadRequest

	if len(status) > 0 {
		statusCode = status[0]
	}

	var payload jsonResponse
	payload.Error = true
	payload.Message = err.Error()

	return app.writeJson(w, statusCode, payload)
}
