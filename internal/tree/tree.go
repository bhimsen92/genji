package tree

import (
	"fmt"

	"github.com/chaisql/chai/internal/encoding"
	"github.com/chaisql/chai/internal/engine"
	"github.com/cockroachdb/errors"
)

type Namespace uint64

// SortOrder is a 64-bit unsigned integer that represents
// the sort order (ASC or DESC) of each value in a key.
// By default, all values are sorted in ascending order.
// Each bit represents the sort order of the corresponding value
// in the key.
// SortOrder is used in a tree to encode keys.
// It can only support up to 64 values.
type SortOrder uint64

func (o SortOrder) IsDesc(i int) bool {
	if i > 63 {
		panic(fmt.Sprintf("cannot get sort order of value %d, only 64 values are supported", i))
	}

	mask := uint64(1) << (63 - i)
	return uint64(o)&mask>>(63-i) != 0
}

func (o SortOrder) SetDesc(i int) SortOrder {
	if i > 63 {
		panic(fmt.Sprintf("cannot set sort order of value %d, only 64 values are supported", i))
	}

	mask := uint64(1) << (63 - i)
	return SortOrder(uint64(o) | mask)
}

func (o SortOrder) SetAsc(i int) SortOrder {
	if i > 63 {
		panic(fmt.Sprintf("cannot set sort order of value %d, only 64 values are supported", i))
	}
	mask := uint64(1) << (63 - i)
	return SortOrder(uint64(o) &^ mask)
}

// A Tree is an abstraction over a k-v store that allows
// manipulating data using high level keys and values of the
// Chai type system.
// Trees are used as the basis for tables and indexes.
// The key of a tree is a composite combination of several
// values, while the value can be any value of Chai's type system.
// The tree ensures all keys are sort-ordered according to the rules
// of the types package operators.
// A Tree doesn't support duplicate keys.
type Tree struct {
	Session   engine.Session
	Namespace Namespace
	Order     SortOrder
}

func New(session engine.Session, ns Namespace, order SortOrder) *Tree {
	return &Tree{
		Namespace: ns,
		Session:   session,
		Order:     order,
	}
}

func NewTransient(session engine.Session, ns Namespace, order SortOrder) (*Tree, func() error, error) {
	t := Tree{
		Namespace: ns,
		Session:   session,
		Order:     order,
	}

	// ensure the namespace is not in use
	err := t.IterateOnRange(nil, false, func(k *Key, b []byte) error {
		return errors.Errorf("namespace %d is already in use", ns)
	})
	if err != nil {
		return nil, nil, err
	}

	return &t, t.Truncate, nil
}

var defaultValue = []byte{0}

// Insert adds a key-obj combination to the tree.
// If the key already exists, it returns engine.ErrKeyAlreadyExists.
func (t *Tree) Insert(key *Key, value []byte) error {
	if len(value) == 0 {
		value = defaultValue
	}
	k, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return err
	}

	return t.Session.Insert(k, value)
}

// Put adds or replaces a key-obj combination to the tree.
// If the key already exists, its value will be replaced by
// the given value.
func (t *Tree) Put(key *Key, value []byte) error {
	if len(value) == 0 {
		value = defaultValue
	}
	k, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return err
	}

	return t.Session.Put(k, value)
}

// Get a key from the tree. If the key doesn't exist,
// it returns engine.ErrKeyNotFound.
func (t *Tree) Get(key *Key) ([]byte, error) {
	k, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return nil, err
	}

	v, err := t.Session.Get(k)
	if err != nil {
		return nil, err
	}

	if len(v) == 0 || v[0] == 0 {
		return nil, nil
	}

	return v, nil
}

// Exists returns true if the key exists in the tree.
func (t *Tree) Exists(key *Key) (bool, error) {
	k, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return false, err
	}

	return t.Session.Exists(k)
}

// Delete a key from the tree. If the key doesn't exist,
// it returns engine.ErrKeyNotFound.
func (t *Tree) Delete(key *Key) error {
	k, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return err
	}

	return t.Session.Delete(k)
}

// Truncate the tree.
func (t *Tree) Truncate() error {
	return t.Session.DeleteRange(encoding.EncodeInt(nil, int64(t.Namespace)), encoding.EncodeInt(nil, int64(t.Namespace)+1))
}

// IterateOnRange iterates on all keys that are in the given range.
func (t *Tree) IterateOnRange(rng *Range, reverse bool, fn func(*Key, []byte) error) error {
	var start, end []byte
	var err error

	if rng == nil {
		rng = &Range{}
	}

	var min, max *Key
	desc := t.isDescRange(rng)
	if !desc {
		min, max = rng.Min, rng.Max
	} else {
		min, max = rng.Max, rng.Min
	}

	if !rng.Exclusive {
		start, end, err = t.buildInclusiveBoundaries(min, max, desc)
	} else {
		start, end, err = t.buildExclusiveBoundaries(min, max, desc)
	}
	if err != nil {
		return err
	}

	opts := engine.IterOptions{
		LowerBound: start,
		UpperBound: end,
	}
	it, err := t.Session.Iterator(&opts)
	if err != nil {
		return err
	}
	defer it.Close()

	if !reverse {
		it.First()
	} else {
		it.Last()
	}

	var k Key
	for it.Valid() {
		k.Encoded = it.Key()
		k.values = nil

		v, err := it.Value()
		if err != nil {
			return err
		}
		if len(v) == 0 || v[0] == 0 {
			v = nil
		}

		err = fn(&k, v)
		if err != nil {
			return err
		}

		if !reverse {
			it.Next()
		} else {
			it.Prev()
		}
	}

	return it.Error()
}

func (t *Tree) isDescRange(rng *Range) bool {
	if rng.Min != nil {
		return t.Order.IsDesc(len(rng.Min.values) - 1)
	}
	if rng.Max != nil {
		return t.Order.IsDesc(len(rng.Max.values) - 1)
	}

	return false
}

func (t *Tree) buildInclusiveBoundaries(min, max *Key, desc bool) (start []byte, end []byte, err error) {
	if min == nil {
		start, err = t.buildMinKeyForType(max, desc)
	} else {
		start, err = t.buildStartKeyInclusive(min, desc)
	}
	if err != nil {
		return
	}
	if max == nil {
		end, err = t.buildMaxKeyForType(min, desc)
	} else {
		end, err = t.buildEndKeyInclusive(max, desc)
	}
	return
}

func (t *Tree) buildExclusiveBoundaries(min, max *Key, desc bool) (start []byte, end []byte, err error) {
	if min == nil {
		start, err = t.buildMinKeyForType(max, desc)
	} else {
		start, err = t.buildStartKeyExclusive(min, desc)
	}
	if err != nil {
		return
	}
	if max == nil {
		end, err = t.buildMaxKeyForType(min, desc)
	} else {
		end, err = t.buildEndKeyExclusive(max, desc)
	}
	return
}

func (t *Tree) buildFirstKey() ([]byte, error) {
	k := NewKey()
	return k.Encode(t.Namespace, t.Order)
}

func (t *Tree) buildMinKeyForType(max *Key, desc bool) ([]byte, error) {
	if max == nil {
		k, err := t.buildFirstKey()
		if err != nil {
			return nil, err
		}
		return k, nil
	}

	if len(max.values) == 1 {
		buf := encoding.EncodeInt(nil, int64(t.Namespace))
		if desc {
			return append(buf, max.values[0].Type().MinEnctypeDesc()), nil
		}

		return append(buf, max.values[0].Type().MinEnctype()), nil
	}

	buf, err := NewKey(max.values[:len(max.values)-1]...).Encode(t.Namespace, t.Order)
	if err != nil {
		return nil, err
	}
	i := len(max.values) - 1
	if desc {
		return append(buf, max.values[i].Type().MinEnctypeDesc()), nil
	}

	return append(buf, max.values[i].Type().MinEnctype()), nil
}

func (t *Tree) buildMaxKeyForType(min *Key, desc bool) ([]byte, error) {
	if min == nil {
		return t.buildLastKey(), nil
	}

	if len(min.values) == 1 {
		buf := encoding.EncodeInt(nil, int64(t.Namespace))
		if desc {
			return append(buf, min.values[0].Type().MaxEnctypeDesc()), nil
		}
		return append(buf, min.values[0].Type().MaxEnctype()), nil
	}

	buf, err := NewKey(min.values[:len(min.values)-1]...).Encode(t.Namespace, t.Order)
	if err != nil {
		return nil, err
	}
	i := len(min.values) - 1
	if desc {
		return append(buf, min.values[i].Type().MaxEnctypeDesc()), nil
	}

	return append(buf, min.values[i].Type().MaxEnctype()), nil
}

func (t *Tree) buildLastKey() []byte {
	buf := encoding.EncodeInt(nil, int64(t.Namespace))
	return append(buf, 0xFF)
}

func (t *Tree) buildStartKeyInclusive(key *Key, desc bool) ([]byte, error) {
	return key.Encode(t.Namespace, t.Order)
}

func (t *Tree) buildStartKeyExclusive(key *Key, desc bool) ([]byte, error) {
	b, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return nil, err
	}

	return append(b, 0xFF), nil
}

func (t *Tree) buildEndKeyInclusive(key *Key, desc bool) ([]byte, error) {
	b, err := key.Encode(t.Namespace, t.Order)
	if err != nil {
		return nil, err
	}

	return append(b, 0xFF), nil
}

func (t *Tree) buildEndKeyExclusive(key *Key, desc bool) ([]byte, error) {
	return key.Encode(t.Namespace, t.Order)
}

// A Range of keys to iterate on.
// By default, Min and Max are inclusive.
// If Exclusive is true, Min and Max are excluded
// from the results.
type Range struct {
	Min, Max  *Key
	Exclusive bool
}
