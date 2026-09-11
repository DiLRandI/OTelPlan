//go:build ignore

package accessors

func RequestID(value any) (string, bool) {
	request, ok := value.(*request)
	if !ok || request == nil {
		return "", false
	}
	return request.ID, true
}
