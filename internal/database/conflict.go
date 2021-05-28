package database

import (
	"github.com/genjidb/genji/document"
)

// OnInsertConflictAction is a function triggered when trying to insert a document that already exists.
// This function is triggered if the key is duplicated or if there is a unique constraint violation on one
// of the fields of the document.
type OnInsertConflictAction func(t *Table, key []byte, d document.Document, err error) (document.Document, error)

// OnInsertConflictDoNothing ignores the duplicate error and returns nothing.
func OnInsertConflictDoNothing(t *Table, key []byte, d document.Document, err error) (document.Document, error) {
	return nil, nil
}

// OnInsertConflictDoReplace replaces the conflicting document with d.
func OnInsertConflictDoReplace(t *Table, key []byte, d document.Document, err error) (document.Document, error) {
	if key == nil {
		return d, err
	}

	err = t.Replace(key, d)
	if err != nil {
		return nil, err
	}

	return documentWithKey{
		Document: d,
		key:      key,
		pk:       t.Info.GetPrimaryKey(),
	}, nil
}
