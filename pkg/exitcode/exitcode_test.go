package exitcode

import (
	"errors"
	"testing"
)

func TestErrorCarriesCodeMessageAndCause(t *testing.T) {
	cause := errors.New("disk full")
	err := Wrap(NotFound, "cannot scan /nope", cause)

	if err.Error() != "cannot scan /nope" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.ExitCode() != NotFound {
		t.Errorf("ExitCode() = %d, want %d", err.ExitCode(), NotFound)
	}
	if !errors.Is(err, cause) {
		t.Error("Wrap must preserve the cause for errors.Is")
	}
}

func TestConstructorsMapToTheDocumentedCodes(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want int
	}{
		{"invalid args", InvalidArgsError("a"), InvalidArgs},
		{"invalid args formatted", InvalidArgsErrorf("a %d", 1), InvalidArgs},
		{"not found", NotFoundError("n"), NotFound},
		{"not found formatted", NotFoundErrorf("n %d", 1), NotFound},
		{"not found cause", NotFoundErrorCause("n", errors.New("x")), NotFound},
		{"unexpected", UnexpectedError("u"), Unexpected},
		{"unexpected formatted", UnexpectedErrorf("u %d", 1), Unexpected},
		{"unexpected cause", UnexpectedErrorCause("u", errors.New("x")), Unexpected},
		{"new", New(130, "interrupted"), 130},
		{"new formatted", Newf(130, "interrupted %s", "now"), 130},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.ExitCode(); got != tc.want {
				t.Errorf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCodesAreDistinctAndSuccessIsZero(t *testing.T) {
	if Success != 0 {
		t.Errorf("Success = %d, want 0", Success)
	}
	seen := map[int]string{}
	for name, code := range map[string]int{
		"InvalidArgs": InvalidArgs,
		"NotFound":    NotFound,
		"Unexpected":  Unexpected,
	} {
		if other, dup := seen[code]; dup {
			t.Errorf("%s and %s share exit code %d", name, other, code)
		}
		seen[code] = name
	}
}

func TestErrorSatisfiesTheExitCoderConvention(t *testing.T) {
	var err error = UnexpectedError("boom")
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) {
		t.Fatal("*Error must satisfy the exit-code interface the top-level runner checks")
	}
	if coded.ExitCode() != Unexpected {
		t.Errorf("ExitCode() = %d, want %d", coded.ExitCode(), Unexpected)
	}
}
