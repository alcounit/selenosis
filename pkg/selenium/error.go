package selenium

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type SeleniumError struct {
	Value struct {
		Name    string `json:"error"`
		Message string `json:"message"`
	} `json:"value"`
}

func WriteError(rw http.ResponseWriter, status int, err *SeleniumError) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(err)
}

func ErrSessionNotCreated(err error) *SeleniumError {
	return Error("session not created", err)
}

func ErrInvalidSessionId(err error) *SeleniumError {
	return Error("invalid session id", err)
}

func ErrInvalidArgument(err error) *SeleniumError {
	return Error("invalid argument", err)
}

func ErrBadRequest(err error) *SeleniumError {
	return Error("bad request", err)
}

func ErrUnknown(err error) *SeleniumError {
	return Error("unknown error", err)
}

func Error(name string, err error) *SeleniumError {
	se := &SeleniumError{}
	se.Value.Name = name
	se.Value.Message = fmt.Errorf("%s: %v", name, err).Error()
	return se
}
