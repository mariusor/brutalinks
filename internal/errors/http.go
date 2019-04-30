package errors

import (
	"encoding/json"
	"fmt"
	"golang.org/x/xerrors"
	"net/http"
)

var IncludeBacktrace = true

type Http struct {
	Code     int    `json:"authCode,omitempty"`
	Message  string `json:"message"`
	Trace    *Stack `json:"trace,omitempty"`
	Location string `json:"location,omitempty"`
}

func HttpError(err error) Http {
	var msg string
	var loc string
	var trace *Stack

	switch e := err.(type) {
	case *json.UnmarshalTypeError:
		msg = fmt.Sprintf("%T: Value[%s] Type[%v]\n", e, e.Value, e.Type)
	case *json.InvalidUnmarshalError:
		msg = fmt.Sprintf("%T: Type[%v]\n", e, e.Type)
	case *Err:
		msg = fmt.Sprintf("%s", e.Error())
		if IncludeBacktrace {
			trace, _ = parseStack(e.t)
			f := e.f
			l := e.l
			if len(f) > 0 {
				loc = fmt.Sprintf("%s:%d", f, l)
			}
		}
	default:
		local := Err{}
		if ok := xerrors.As(err, &local); ok {
			if IncludeBacktrace {
				trace, _ = parseStack(local.t)
				f := local.f
				l := local.l
				if len(f) > 0 {
					loc = fmt.Sprintf("%s:%d", f, l)
				}
			}
		}
		msg = err.Error()
	}
	return Http{
		Message:  msg,
		Trace:    trace,
		Location: loc,
		Code:     httpErrorResponse(err),
	}
}

func httpErrorResponse(e error) int {
	if IsBadRequest(e) {
		return http.StatusBadRequest
	}
	if IsForbidden(e) {
		return http.StatusForbidden
	}
	if IsNotSupported(e) {
		return http.StatusHTTPVersionNotSupported
	}
	if IsMethodNotAllowed(e) {
		return http.StatusMethodNotAllowed
	}
	if IsNotFound(e) {
		return http.StatusNotFound
	}
	if IsNotImplemented(e) {
		return http.StatusNotImplemented
	}
	if IsUnauthorized(e) {
		return http.StatusUnauthorized
	}
	if IsTimeout(e) {
		return http.StatusGatewayTimeout
	}
	if IsNotValid(e) {
		return http.StatusNotAcceptable
	}
	if IsMethodNotAllowed(e) {
		return http.StatusMethodNotAllowed
	}
	return http.StatusInternalServerError
}
