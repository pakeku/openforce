package main

import "fmt"

type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func apiErr(status int, msg string) error {
	return &apiError{status: status, msg: msg}
}

func apiErrf(status int, format string, args ...any) error {
	return apiErr(status, fmt.Sprintf(format, args...))
}
