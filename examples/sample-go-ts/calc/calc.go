// Package calc is the tested half of the example: some of it is exercised by
// calc_test.go and some deliberately is not, so the treemap has something
// interesting to colour.
package calc

import (
	"errors"
	"fmt"
)

// ErrDivideByZero is returned by Div.
var ErrDivideByZero = errors.New("division by zero")

// Add returns a + b. Covered by the example tests.
func Add(a, b int) int {
	return a + b
}

// Sub returns a - b. Covered by the example tests.
func Sub(a, b int) int {
	return a - b
}

// Mul returns a * b. Deliberately left uncovered.
func Mul(a, b int) int {
	return a * b
}

// Div returns a / b. The success path is covered; the error path is not.
func Div(a, b int) (int, error) {
	if b == 0 {
		return 0, ErrDivideByZero
	}
	return a / b, nil
}

// Describe renders a result. Deliberately left uncovered.
func Describe(a, b int) string {
	sum, err := Div(a, b)
	if err != nil {
		return fmt.Sprintf("%d/%d: %v", a, b, err)
	}
	return fmt.Sprintf("%d/%d = %d", a, b, sum)
}
