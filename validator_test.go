package validator_test

import (
	"testing"

	"github.com/Kevinguj0/validator"
)

type Address struct {
	Street string `validate:"required"`
	City   string `validate:"required"`
}

type UserProfile struct {
	Address // Embedded struct
	ID      string `validate:"required"`
	Name    string `validate:"required"`
}

func TestStructPartial_EmbeddedStruct(t *testing.T) {
	v := validator.New()

	// User only updates Name. Address fields are zero-value (empty) and omitted from validation mask.
	user := UserProfile{
		Name: "Alice",
	}

	// Validate ONLY "Name"
	err := v.StructPartial(user, "Name")
	if err != nil {
		t.Fatalf("expected no validation error for omitted embedded fields, got: %v", err)
	}
}

func TestStructPartial_EmbeddedStructTargeted(t *testing.T) {
	v := validator.New()

	user := UserProfile{
		Name: "Alice",
	}

	// Validate ONLY "Street"
	err := v.StructPartial(user, "Street")
	if err == nil {
		t.Fatalf("expected validation error for required Street field, got nil")
	}

	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors type, got %T", err)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error for Street, got %d errors: %v", len(errs), errs)
	}
}

func TestStructPartial_EmbeddedStructPath(t *testing.T) {
	v := validator.New()

	user := UserProfile{
		Name: "Alice",
	}

	// Validate ONLY "Address.Street"
	err := v.StructPartial(user, "Address.Street")
	if err == nil {
		t.Fatalf("expected validation error for Address.Street, got nil")
	}

	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors type, got %T", err)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error for Address.Street, got %d errors: %v", len(errs), errs)
	}
}

func TestStruct_Full(t *testing.T) {
	v := validator.New()

	user := UserProfile{
		Name: "Alice",
	}

	err := v.Struct(user)
	if err == nil {
		t.Fatalf("expected validation errors for full struct validation, got nil")
	}
}
