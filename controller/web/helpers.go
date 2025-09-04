package web

import (
	"encoding/json"
	"strings"

	"gorm.io/datatypes"
)

func jsonFromMap(m map[string]any) datatypes.JSON {
	if m == nil {
		return datatypes.JSON([]byte(`{}`))
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

func jsonPtrFromMap(m *map[string]any) *datatypes.JSON {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	j := datatypes.JSON(b)
	return &j
}

func stringsTrim(s string) string { return strings.TrimSpace(s) }
