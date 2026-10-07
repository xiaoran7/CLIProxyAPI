package common

import "fmt"

// UnsupportedPartError is a request-scoped rejection. The part type was present
// and this translation has no equivalent, so the request must not be sent with
// that part removed.
type UnsupportedPartError struct {
	Type string
}

func (e *UnsupportedPartError) Error() string {
	if e == nil || e.Type == "" {
		return "unsupported content part"
	}
	return fmt.Sprintf("unsupported content part: %s", e.Type)
}

func (e *UnsupportedPartError) StatusCode() int { return 400 }

func (e *UnsupportedPartError) IsRequestScoped() bool { return true }

// ErrIfNothingLeft is the shared policy for a file part a translation cannot
// send: text beside it still goes out, but a request left with no content is
// refused rather than forwarded empty.
func ErrIfNothingLeft(droppedType string, remaining int) error {
	if droppedType == "" || remaining > 0 {
		return nil
	}
	return &UnsupportedPartError{Type: droppedType}
}
