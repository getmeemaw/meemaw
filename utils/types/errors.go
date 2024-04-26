package types

import (
	"errors"
	"reflect"
	"testing"
)

type ErrNotFound struct{}

func (err *ErrNotFound) Error() string {
	return "not found"
}

type ErrUnauthorized struct{}

func (err *ErrUnauthorized) Error() string {
	return "unauthorized"
}

type ErrBadRequest struct{}

func (err *ErrBadRequest) Error() string {
	return "bad request"
}

type ErrTssProcessFailed struct{}

func (err *ErrTssProcessFailed) Error() string {
	return "tss process failed"
}

type ErrConflict struct{}

func (err *ErrConflict) Error() string {
	return "conflict"
}

type ErrTimeOut struct{}

func (err *ErrTimeOut) Error() string {
	return "timed out"
}

// ProcessShouldError compares the result of a test with what it should have been, and reacts accordingly (fail or succeed test)
func ProcessShouldError(testDescription string, err error, requiredErr error, resultObject any, t *testing.T) {
	if err != nil {
		if errors.Is(err, &ErrTimeOut{}) {
			t.Errorf("Failed "+testDescription+": was expecting error %s, got timeout\n", reflect.TypeOf(requiredErr).String())
		} else if (reflect.TypeOf(resultObject).Kind() == reflect.String && resultObject != "") || (reflect.TypeOf(resultObject).Kind() == reflect.Ptr && !reflect.ValueOf(resultObject).IsNil()) {
			t.Errorf("Failed "+testDescription+": got result %s despite error\n", resultObject)
		} else if errors.Is(err, requiredErr) {
			t.Logf("Successful "+testDescription+": was expecting %s, got %s\n", reflect.TypeOf(requiredErr).String(), reflect.TypeOf(err).String())
		} else {
			t.Errorf("Failed "+testDescription+": was expecting %s, got %s with value: %s\n", reflect.TypeOf(requiredErr).String(), reflect.TypeOf(err).String(), err)
		}
	} else {
		t.Errorf("Failed "+testDescription+": was expecting error %s, got nil instead. Object received: %s", reflect.TypeOf(requiredErr).String(), resultObject)
	}
}
