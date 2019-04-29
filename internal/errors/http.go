package errors

import (
	"encoding/json"
	"fmt"
	juju "github.com/juju/errors"
	"net/http"
)

type Http struct {
	Code     int      `json:"authCode,omitempty"`
	Message  string   `json:"message"`
	Trace    []string `json:"trace,omitempty"`
	Location string   `json:"location,omitempty"`
}

func HttpError(err error) Http {
	var msg string
	var loc string
	var trace []string

	if IsBadRequest(err) {
		err = juju.Cause(err)
	}
	if IsForbidden(err) {
		err = juju.Cause(err)
	}
	if IsNotSupported(err) {
		err = juju.Cause(err)
	}
	if IsMethodNotAllowed(err) {
		err = juju.Cause(err)
	}
	if IsNotFound(err) {
		err = juju.Cause(err)
	}
	if IsNotImplemented(err) {
		err = juju.Cause(err)
	}
	if IsUnauthorized(err) {
		err = juju.Cause(err)
	}
	if IsTimeout(err) {
		err = juju.Cause(err)
	}
	if IsNotValid(err) {
		err = juju.Cause(err)
	}
	if IsMethodNotAllowed(err) {
		err = juju.Cause(err)
	}
	switch e := juju.Cause(err).(type) {
	case *json.UnmarshalTypeError:
		msg = fmt.Sprintf("%T: Value[%s] Type[%v]\n", e, e.Value, e.Type)
	case *json.InvalidUnmarshalError:
		msg = fmt.Sprintf("%T: Type[%v]\n", e, e.Type)
	case *juju.Err:
		msg = fmt.Sprintf("%s", e.Error())
		trace = e.StackTrace()
		f, l := e.Location()
		if len(f) > 0 {
			loc = fmt.Sprintf("%s:%d", f, l)
		}
	default:
		msg = e.Error()
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
