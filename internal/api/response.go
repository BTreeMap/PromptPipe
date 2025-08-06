// Package api provides HTTP response utilities for PromptPipe.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

// Pre-marshaled fallback responses to avoid runtime JSON encoding failures
var (
	fallbackErrorResponse []byte
)

// init validates that our fallback responses can be marshaled
func init() {
	var err error
	fallbackErrorResponse, err = json.Marshal(models.Error("Internal server error"))
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal fallback error response at startup: %v", err))
	}
}

// writeJSONResponse writes a JSON response to the http.ResponseWriter with the given status code.
func writeJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) {
	// Marshal the response to JSON first to catch encoding errors before writing headers
	jsonData, err := json.Marshal(response)
	if err != nil {
		slog.Error("Server.writeJSONResponse: failed to marshal JSON response", "error", err)
		// Use pre-marshaled fallback response - if this fails, we have bigger problems
		jsonData = fallbackErrorResponse
		statusCode = http.StatusInternalServerError
	}

	// Write headers and response only after successful JSON marshaling
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, writeErr := w.Write(jsonData); writeErr != nil {
		slog.Error("Server.writeJSONResponse: failed to write JSON response", "error", writeErr)
	}
}
