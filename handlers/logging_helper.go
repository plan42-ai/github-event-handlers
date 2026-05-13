package handlers

func logPayload(logPayloads bool, evt Event, attrs ...any) []any {
	if !logPayloads || evt == nil {
		return attrs
	}
	return append(attrs, "payload", evt)
}
