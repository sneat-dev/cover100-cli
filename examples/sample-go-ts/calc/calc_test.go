package calc

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2, 3) = %d, want 5", got)
	}
}

func TestSub(t *testing.T) {
	if got := Sub(5, 3); got != 2 {
		t.Fatalf("Sub(5, 3) = %d, want 2", got)
	}
}

func TestDiv(t *testing.T) {
	got, err := Div(10, 2)
	if err != nil {
		t.Fatalf("Div(10, 2) returned error: %v", err)
	}
	if got != 5 {
		t.Fatalf("Div(10, 2) = %d, want 5", got)
	}
}
