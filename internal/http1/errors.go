package http1

import "fmt"

type protocolError struct {
	code int
	msg  string
}

func (e *protocolError) Error() string {
	return fmt.Sprintf("http %d: %s", e.code, e.msg)
}

func protoErr(code int, msg string) *protocolError {
	return &protocolError{code: code, msg: msg}
}
