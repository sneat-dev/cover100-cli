package model

import "testing"

// TestPointerHelpersReturnPointersToTheirValues pins the contract the JSON
// encoders rely on: each helper must hand back a non-nil pointer to the value
// it was given. A helper that returned nil, or a pointer to a zero value,
// would silently drop optional fields from the report.
func TestPointerHelpersReturnPointersToTheirValues(t *testing.T) {
	s := StrPtr("typescript")
	if s == nil {
		t.Fatal("StrPtr returned nil")
	}
	if *s != "typescript" {
		t.Errorf("*StrPtr(\"typescript\") = %q, want %q", *s, "typescript")
	}

	i := IntPtr(42)
	if i == nil {
		t.Fatal("IntPtr returned nil")
	}
	if *i != 42 {
		t.Errorf("*IntPtr(42) = %d, want 42", *i)
	}

	yes := BoolPtr(true)
	if yes == nil {
		t.Fatal("BoolPtr(true) returned nil")
	}
	if !*yes {
		t.Error("*BoolPtr(true) = false, want true")
	}

	no := BoolPtr(false)
	if no == nil {
		t.Fatal("BoolPtr(false) returned nil")
	}
	if *no {
		t.Error("*BoolPtr(false) = true, want false")
	}
}
