package errors

import (
	juju "github.com/juju/errors"
)

type Error juju.Err

func (e Error) Error() string {
	j := juju.Err(e)
	return (&j).Error()
}

func (e Error) Unwrap() error {
	return nil
}

func Details(e error) error {
	return nil
}

func Annotate(e error, s string) error {
	return juju.Annotate(e, s)
}

func Annotatef(e error, s string, args ...interface{}) error {
	return juju.Annotatef(e, s, args...)
}

func New(s string) error {
	return juju.New(s)
}

func Errorf(s string, args ...interface{}) error {
	return juju.Errorf(s, args...)
}

func NotFoundf(s string, args ...interface{}) error {
	return juju.NotFoundf(s, args...)
}

func NewNotFound(e error, s string) error {
	return juju.NewNotFound(e, s)
}

func MethodNotAllowedf(s string, args ...interface{}) error {
	return juju.MethodNotAllowedf(s, args...)
}

func NewMethodNotAllowed(e error, s string) error {
	return juju.NewMethodNotAllowed(e, s)
}

func NotValidf(s string, args ...interface{}) error {
	return juju.NotValidf(s, args...)
}

func NewNotValid(e error, s string) error {
	return juju.NewNotValid(e, s)
}

func Forbiddenf(s string, args ...interface{}) error {
	return juju.Forbiddenf(s, args...)
}

func NewForbidden(e error, s string) error {
	return juju.NewForbidden(e, s)
}

func NotImplementedf(s string, args ...interface{}) error {
	return juju.NotImplementedf(s, args...)
}

func NewNotImplemented(e error, s string) error {
	return juju.NewNotImplemented(e, s)
}

func BadRequestf(s string, args ...interface{}) error {
	return juju.BadRequestf(s, args...)
}

func NewBadRequest(e error, s string) error {
	return juju.NewBadRequest(e, s)
}

func IsBadRequest(e error) bool {
	return juju.IsBadRequest(e)
}
func IsForbidden(e error) bool {
	return juju.IsForbidden(e)
}
func IsNotSupported(e error) bool {
	return juju.IsNotSupported(e)
}
func IsMethodNotAllowed(e error) bool {
	return juju.IsMethodNotAllowed(e)
}
func IsNotFound(e error) bool {
	return juju.IsNotFound(e)
}
func IsNotImplemented(e error) bool {
	return juju.IsNotImplemented(e)
}
func IsUnauthorized(e error) bool {
	return juju.IsUnauthorized(e)
}
func IsTimeout(e error) bool {
	return juju.IsTimeout(e)
}
func IsNotValid(e error) bool {
	return juju.IsNotValid(e)
}
