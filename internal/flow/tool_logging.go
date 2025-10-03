package flow

import (
	"encoding/json"
	"strings"
)

const toolArgumentsLogLimit = 1024

func formatToolArgumentsForLog(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	argStr := strings.TrimSpace(string(raw))
	if len(argStr) > toolArgumentsLogLimit {
		return argStr[:toolArgumentsLogLimit] + "...(truncated)"
	}
	return argStr
}
