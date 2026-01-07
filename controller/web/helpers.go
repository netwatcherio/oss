// web/helpers.go
// Consolidated helper functions to reduce duplication across handler files.
package web

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
)

// -------------------- Context Helpers --------------------

// currentUserID extracts the authenticated user ID from the Iris context.
// Uses the same key as JWTMiddleware stores.
// Returns 0 if not authenticated.
func currentUserID(ctx iris.Context) uint {
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

// -------------------- Parameter Parsing --------------------

// uintParam extracts a uint path parameter by name.
func uintParam(ctx iris.Context, name string) uint {
	v, _ := strconv.Atoi(ctx.Params().Get(name))
	if v < 0 {
		return 0
	}
	return uint(v)
}

// uintParamName is an alias for uintParam for backward compatibility.
// Deprecated: Use uintParam instead.
func uintParamName(ctx iris.Context, name string) uint {
	return uintParam(ctx, name)
}

// intParam extracts an int query parameter with bounds checking.
func intParam(ctx iris.Context, name string, def, min, max int) int {
	if v, err := strconv.Atoi(ctx.URLParamDefault(name, "")); err == nil {
		if v < min {
			return min
		}
		if v > max {
			return max
		}
		return v
	}
	return def
}

// -------------------- Utility Functions --------------------

// ifZero returns def if v is 0, otherwise returns v.
func ifZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// stringsTrim trims whitespace from a string.
func stringsTrim(s string) string { return strings.TrimSpace(s) }

// -------------------- JSON Helpers --------------------

// jsonFromMap converts a map to GORM's datatypes.JSON.
// Returns empty JSON object if input is nil.
func jsonFromMap(m map[string]any) datatypes.JSON {
	if m == nil {
		return datatypes.JSON([]byte(`{}`))
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

// jsonPtrFromMap converts a pointer to a map to a pointer to datatypes.JSON.
// Returns nil if input is nil.
func jsonPtrFromMap(m *map[string]any) *datatypes.JSON {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	j := datatypes.JSON(b)
	return &j
}

// -------------------- Response Helpers --------------------

// ListResponse is a standardized wrapper for list endpoints.
// All list endpoints should use this format for consistency.
type ListResponse struct {
	Data   interface{} `json:"data"`
	Total  int         `json:"total,omitempty"`
	Limit  int         `json:"limit,omitempty"`
	Offset int         `json:"offset,omitempty"`
}

// NewListResponse creates a ListResponse with just data (no pagination).
func NewListResponse(data interface{}) ListResponse {
	return ListResponse{Data: data}
}

// NewPaginatedResponse creates a ListResponse with pagination info.
func NewPaginatedResponse(data interface{}, total, limit, offset int) ListResponse {
	return ListResponse{
		Data:   data,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

// -------------------- Error Response Helpers --------------------

// ErrorResponse represents a standardized error response.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// NewErrorResponse creates an error response from an error.
func NewErrorResponse(err error) ErrorResponse {
	return ErrorResponse{Error: err.Error()}
}

// NewErrorResponseWithCode creates an error response with an error code.
func NewErrorResponseWithCode(err error, code string) ErrorResponse {
	return ErrorResponse{
		Error: err.Error(),
		Code:  code,
	}
}
