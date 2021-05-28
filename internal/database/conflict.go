package database

import "github.com/genjidb/genji/document"

// OnInsertConflictAction is a function triggered when trying to insert a document that already exists.
// This function is triggered if the key is duplicated or if there is a unique constraint violation on one
// of the fields of the document.
type OnInsertConflictAction func(t *Table, key []byte, d document.Document) (document.Document, error)

// OnInsertConflictDoNothing ignores the duplicate error and returns the document with the given key.
func OnInsertConflictDoNothing(t *Table, key []byte, d document.Document) (document.Document, error) {
	return nil, nil
}
