package shell

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaisql/chai"
	"github.com/chaisql/chai/cmd/chai/dbutil"
	"github.com/stretchr/testify/require"
)

func TestRunTablesCmd(t *testing.T) {
	tests := []struct {
		name   string
		tables []string
		want   string
	}{
		{
			"No table",
			nil,
			"",
		},
		{
			"With tables",
			[]string{"foo", "bar"},
			"bar\nfoo\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := chai.Open(":memory:")
			require.NoError(t, err)
			defer db.Close()

			for _, tb := range test.tables {
				err := db.Exec("CREATE TABLE " + tb + "(a INT)")
				require.NoError(t, err)
			}

			var buf bytes.Buffer
			err = runTablesCmd(db, &buf)
			require.NoError(t, err)

			require.Equal(t, test.want, buf.String())
		})
	}
}

func TestIndexesCmd(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		want      string
		fails     bool
	}{
		{"All", "", "idx_bar_a_b\nidx_foo_a\nidx_foo_b\n", false},
		{"With table name", "foo", "idx_foo_a\nidx_foo_b\n", false},
		{"With nonexistent table name", "baz", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := chai.Open(":memory:")
			require.NoError(t, err)
			defer db.Close()

			err = db.Exec(`
				CREATE TABLE foo(a INT, b INT);
				CREATE INDEX idx_foo_a ON foo (a);
				CREATE INDEX idx_foo_b ON foo (b);
				CREATE TABLE bar(a INT, b INT);
				CREATE INDEX idx_bar_a_b ON bar (a, b);
			`)
			require.NoError(t, err)

			var buf bytes.Buffer
			err = runIndexesCmd(db, test.tableName, &buf)
			if test.fails {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, buf.String())
			}
		})
	}
}

func TestSaveCommand(t *testing.T) {
	dir, err := os.MkdirTemp("", "chai")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := chai.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.Exec(`
		CREATE TABLE test (a DOUBLE, b INT);
		CREATE INDEX idx_a_b ON test (a, b);
	`)
	require.NoError(t, err)
	err = db.Exec("INSERT INTO test (a, b) VALUES (?, ?)", 1, 2)
	require.NoError(t, err)
	err = db.Exec("INSERT INTO test (a, b) VALUES (?, ?)", 2, 2)
	require.NoError(t, err)
	err = db.Exec("INSERT INTO test (a, b) VALUES (?, ?)", 3, 2)
	require.NoError(t, err)

	// save the dummy database
	err = runSaveCmd(context.Background(), db, dir)
	require.NoError(t, err)

	db, err = chai.Open(dir)
	require.NoError(t, err)
	defer db.Close()

	// ensure that the data is present
	r, err := db.QueryRow("SELECT * FROM test")
	require.NoError(t, err)

	var res struct {
		A int
		B int
	}
	err = r.StructScan(&res)
	require.NoError(t, err)

	require.Equal(t, 1, res.A)
	require.Equal(t, 2, res.B)

	// ensure that the index has been created
	indexes, err := dbutil.ListIndexes(db, "")
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	require.Equal(t, "idx_a_b", indexes[0])
}

func BenchmarkImportCSV(b *testing.B) {
	db, err := chai.Open(b.TempDir())
	require.NoError(b, err)
	defer db.Close()

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write([]string{"a", "b", "c"})
	for i := 0; i < 10000; i++ {
		w.Write([]string{"1", "2", "3"})
	}
	w.Flush()

	fp := filepath.Join(b.TempDir(), "data.csv")
	err = os.WriteFile(fp, buf.Bytes(), 0644)
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err = runImportCmd(db, "csv", fp, "foo")
		require.NoError(b, err)

		b.StopTimer()
		err = db.Exec("DELETE FROM foo")
		require.NoError(b, err)
		b.StartTimer()
	}
}
