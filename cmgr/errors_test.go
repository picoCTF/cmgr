package cmgr

import (
	"errors"
	"fmt"
	"testing"
)

func TestUnknownIdentifierError(t *testing.T) {
	err := &UnknownIdentifierError{Type: "challenge", Name: "test-challenge"}
	expected := "unknown challenge identifier: test-challenge"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestUnknownChallengeIdError(t *testing.T) {
	err := unknownChallengeIdError(ChallengeId("my/challenge"))
	expected := "unknown challenge identifier: my/challenge"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	var uidErr *UnknownIdentifierError
	if !errors.As(err, &uidErr) {
		t.Error("expected error to be UnknownIdentifierError")
	}
	if uidErr.Type != "challenge" {
		t.Errorf("expected Type 'challenge', got %q", uidErr.Type)
	}
}

func TestUnknownBuildIdError(t *testing.T) {
	err := unknownBuildIdError(BuildId(42))
	expected := "unknown build identifier: 42"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	var uidErr *UnknownIdentifierError
	if !errors.As(err, &uidErr) {
		t.Error("expected error to be UnknownIdentifierError")
	}
	if uidErr.Type != "build" {
		t.Errorf("expected Type 'build', got %q", uidErr.Type)
	}
}

func TestUnknownInstanceIdError(t *testing.T) {
	err := unknownInstanceIdError(InstanceId(99))
	expected := "unknown instance identifier: 99"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	var uidErr *UnknownIdentifierError
	if !errors.As(err, &uidErr) {
		t.Error("expected error to be UnknownIdentifierError")
	}
	if uidErr.Type != "instance" {
		t.Errorf("expected Type 'instance', got %q", uidErr.Type)
	}
}

func TestUnknownSchemaIdError(t *testing.T) {
	err := unknownSchemaIdError("my-schema")
	expected := "unknown schema identifier: my-schema"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	var uidErr *UnknownIdentifierError
	if !errors.As(err, &uidErr) {
		t.Error("expected error to be UnknownIdentifierError")
	}
	if uidErr.Type != "schema" {
		t.Errorf("expected Type 'schema', got %q", uidErr.Type)
	}
}

func TestIsEmptyQueryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"matching error", fmt.Errorf("sql: no rows in result set"), true},
		{"non-matching error", fmt.Errorf("some other error"), false},
		{"similar but different", fmt.Errorf("sql: no rows in result"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmptyQueryError(tt.err)
			if result != tt.expected {
				t.Errorf("isEmptyQueryError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
