package functions_test

import (
	"path/filepath"
	"testing"

	"github.com/chaisql/chai/internal/testutil"
)

func TestMathFunctions(t *testing.T) {
	testutil.ExprRunner(t, filepath.Join("testdata", "math_functions.sql"))
}
