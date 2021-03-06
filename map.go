package objects

import (
	"context"
	"fmt"
	"reflect"
)

type Map struct {
	v reflect.Value
}

var (
	_ Reader     = (*Map)(nil)
	_ SafeReader = (*Map)(nil)
	_ ListerTo   = (*Map)(nil)
)

func (m *Map) Type() Type {
	return TypeMap
}

func (m *Map) Get(ctx context.Context, key string) (any, bool) {
	v, err := m.SafeGet(ctx, key)
	return v, err == nil
}

func (m *Map) SafeGet(ctx context.Context, key string) (any, error) {
	var (
		t = m.v.Type().Key()
		k = reflect.ValueOf(key)
	)

	if k.CanConvert(t) {
		k = k.Convert(t)
	}

	switch v := m.v.MapIndex(k); {
	case v.IsZero():
		return nil, &Error{
			Op:  "Get",
			Key: []string{key},
			Err: ErrNotFound,
		}
	case !v.CanInterface():
		return nil, &Error{
			Op:  "Get",
			Key: []string{key},
			Got: v,
			Err: fmt.Errorf("cannot access value: %s", v.Type()),
		}
	default:
		return tryMake(v.Interface()), nil
	}
}

var typstr = reflect.TypeOf(string(""))

func (m *Map) List(ctx context.Context) []string {
	var keys []string
	m.ListTo(ctx, &keys)
	return keys
}

func (m *Map) ListTo(ctx context.Context, keys *[]string) {
	for _, k := range m.v.MapKeys() {
		var key string
		if k.CanConvert(typstr) {
			key = k.Convert(typstr).Interface().(string)
		} else {
			key = fmt.Sprint(k.Interface())
		}

		*keys = append(*keys, key)
	}
}
