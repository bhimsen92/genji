package rows_test

import (
	"encoding/json"
	"testing"

	"github.com/chaisql/chai/internal/environment"
	"github.com/chaisql/chai/internal/expr"
	"github.com/chaisql/chai/internal/row"
	"github.com/chaisql/chai/internal/sql/parser"
	"github.com/chaisql/chai/internal/stream/rows"
	"github.com/chaisql/chai/internal/testutil"
	"github.com/chaisql/chai/internal/types"
	"github.com/stretchr/testify/require"
)

func TestProject(t *testing.T) {
	tests := []struct {
		name  string
		exprs []expr.Expr
		in    row.Row
		out   string
		fails bool
	}{
		{
			"Constant",
			[]expr.Expr{parser.MustParseExpr("10")},
			testutil.MakeRow(t, `{"a":1,"b":true}`),
			`{"10":10}`,
			false,
		},
		{
			"Wildcard",
			[]expr.Expr{expr.Wildcard{}},
			testutil.MakeRow(t, `{"a":1,"b":true}`),
			`{"a":1,"b":true}`,
			false,
		},
		{
			"Multiple",
			[]expr.Expr{expr.Wildcard{}, expr.Wildcard{}, parser.MustParseExpr("10")},
			testutil.MakeRow(t, `{"a":1,"b":true}`),
			`{"a":1,"b":true,"a":1,"b":true,"10":10}`,
			false,
		},
		{
			"Named",
			[]expr.Expr{&expr.NamedExpr{Expr: parser.MustParseExpr("10"), ExprName: "foo"}},
			testutil.MakeRow(t, `{"a":1,"b":true}`),
			`{"foo":10}`,
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var inEnv environment.Environment
			inEnv.SetRow(test.in)

			err := rows.Project(test.exprs...).Iterate(&inEnv, func(out *environment.Environment) error {
				require.Equal(t, &inEnv, out.GetOuter())
				r, ok := out.GetRow()
				require.True(t, ok)
				tt, err := json.Marshal(r)
				require.NoError(t, err)
				require.JSONEq(t, test.out, string(tt))

				err = r.Iterate(func(field string, want types.Value) error {
					got, err := r.Get(field)
					require.NoError(t, err)
					require.Equal(t, want, got)
					return nil
				})
				require.NoError(t, err)
				return nil
			})
			if test.fails {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("String", func(t *testing.T) {
		require.Equal(t, "rows.Project(1, *, *, 1 + 1)", rows.Project(
			parser.MustParseExpr("1"),
			expr.Wildcard{},
			expr.Wildcard{},
			parser.MustParseExpr("1 +    1"),
		).String())
	})

	t.Run("No input", func(t *testing.T) {
		rows.Project(parser.MustParseExpr("1 + 1")).Iterate(new(environment.Environment), func(out *environment.Environment) error {
			r, ok := out.GetRow()
			require.True(t, ok)
			enc, err := row.MarshalJSON(r)
			require.NoError(t, err)
			require.JSONEq(t, `{"1 + 1": 2}`, string(enc))
			return nil
		})
	})
}
