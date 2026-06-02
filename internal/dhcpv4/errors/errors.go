package errors

import "fmt"

type Code string

const (
	CodeOption82Invalid Code = "ErrOption82Invalid"
	CodeIPConflict      Code = "ErrIPConflict"
	CodeRateLimited     Code = "ErrRateLimited"
	CodeRelayRejected   Code = "ErrRelayRejected"
	CodeMalformedPacket Code = "ErrMalformedPacket"
)

type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Msg)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Msg, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func New(code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Msg: msg, Err: err}
}

var (
	ErrOption82Invalid = New(CodeOption82Invalid, "dhcpv4 option82 invalid")
	ErrIPConflict      = New(CodeIPConflict, "dhcpv4 ip conflict")
)
