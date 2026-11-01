package maps

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
)

// The handler's own check of the number of squares stands behind the request size limit of the field.
func TestCheckPaintSquaresRefusesMoreThanTheLimit(t *testing.T) {
	t.Parallel()
	if err := checkPaintSquares(maxPaintSquares); err != nil {
		t.Errorf("checkPaintSquares(%d) error = %v, want nil", maxPaintSquares, err)
	}
	for _, n := range []int{0, maxPaintSquares + 1} {
		err := checkPaintSquares(n)
		if connect.CodeOf(err) != connect.CodeInvalidArgument || err == nil || !strings.Contains(err.Error(), "squares must have 1 to 400 squares") {
			t.Errorf("checkPaintSquares(%d) error = %v, want invalid_argument: squares must have 1 to 400 squares", n, err)
		}
	}
}
