package types

import "context"

type PrefixedReader struct {
	Key Key
	R   Reader
}

type PrefixedWriter struct {
	Key Key
	W   Writer
}

type Prefixed struct {
	PrefixedReader
	PrefixedWriter
}

var (
	_ Reader        = PrefixedReader{}
	_ Writer        = PrefixedWriter{}
	_ SafeReader    = PrefixedReader{}
	_ SafeWriter    = PrefixedWriter{}
	_ Interface     = Prefixed{}
	_ SafeInterface = Prefixed{}
)

func PrefixReader(r Reader, keys ...string) PrefixedReader {
	return PrefixedReader{
		Key: keys,
		R:   r,
	}
}

func PrefixWriter(w Writer, keys ...string) PrefixedWriter {
	return PrefixedWriter{
		Key: keys,
		W:   w,
	}
}

func Prefix(iface Interface, keys ...string) Prefixed {
	return Prefixed{
		PrefixedReader{
			Key: keys,
			R:   iface,
		},
		PrefixedWriter{
			Key: keys,
			W:   iface,
		},
	}
}

func (pr PrefixedReader) Get(ctx context.Context, key string) (value any, ok bool) {
	v, err := pr.SafeGet(ctx, key)
	return v, err == nil
}

func (pr PrefixedReader) List(ctx context.Context) []string {
	r, err := pr.base(ctx, "List")
	if err != nil {
		return nil
	}

	return r.List(ctx)
}

func (pr PrefixedReader) Type() Type {
	return pr.R.Type()
}

func (pr PrefixedReader) SafeGet(ctx context.Context, key string) (any, error) {
	r, err := pr.base(ctx, "Get")
	if err != nil {
		return nil, err
	}

	var (
		v any
	)

	if sr, ok := r.(SafeReader); ok {
		if v, err = sr.SafeGet(ctx, key); err != nil {
			return nil, &Error{
				Op:  "Get",
				Key: append(pr.Key, key),
				Got: sr,
				Err: err,
			}
		}
	} else if v, ok = r.Get(ctx, key); !ok {
		return nil, &Error{
			Op:  "Get",
			Key: append(pr.Key, key),
			Got: r,
			Err: ErrNotFound,
		}
	}

	return v, nil
}

func (pr PrefixedReader) base(ctx context.Context, op string) (Reader, error) {
	var (
		r   = pr.R
		v   any
		err error
		ok  bool
	)

	for i, key := range pr.Key {
		if sr, ok := r.(SafeReader); ok {
			if v, err = sr.SafeGet(ctx, key); err != nil {
				return nil, &Error{
					Op:  op,
					Key: pr.Key[:i+1],
					Got: sr,
					Err: err,
				}
			}
		} else if v, ok = r.Get(ctx, key); !ok {
			return nil, &Error{
				Op:  op,
				Key: pr.Key[:i+1],
				Got: r,
				Err: ErrNotFound,
			}
		}

		if r, ok = v.(Reader); !ok {
			return nil, &Error{
				Op:   op,
				Key:  pr.Key[:i+1],
				Got:  v,
				Want: Reader(nil),
				Err:  ErrUnexpectedType,
			}
		}
	}

	return r, nil
}

func (pr PrefixedReader) reader() Reader {
	const maxDepth = 128

	key := pr.Key

	for i := 0; i < maxDepth; i++ {
		switch x := pr.R.(type) {
		case PrefixedReader:
			pr = x
			key.Prepend(pr.Key)
		case Prefixed:
			pr = x.PrefixedReader
			key.Prepend(pr.Key)
		default:
			return pr.R
		}
	}

	return nil
}

func (pw PrefixedWriter) Del(ctx context.Context, key string) bool {
	return pw.SafeDel(ctx, key) == nil
}

func (pw PrefixedWriter) Set(ctx context.Context, key string, value any) bool {
	ok, _ := pw.SafeSet(ctx, key, value)
	return ok
}

func (pw PrefixedWriter) Put(ctx context.Context, key string, hint Type) Writer {
	w, _ := pw.SafePut(ctx, key, hint)
	return w
}

func (pw PrefixedWriter) SafeDel(ctx context.Context, key string) error {
	pr, err := pw.reader("Del")
	if err != nil {
		return err
	}

	r, err := pr.base(ctx, "Del")
	if err != nil {
		return err
	}

	switch w := r.(type) {
	case SafeWriter:
		if err := w.SafeDel(ctx, key); err != nil {
			return &Error{
				Op:  "Del",
				Key: append(pw.Key, key),
				Err: err,
			}
		}
	case Writer:
		if ok := w.Del(ctx, key); !ok {
			return &Error{
				Op:  "Del",
				Key: append(pw.Key, key),
				Err: ErrNotFound,
			}
		}
	default:
		return &Error{
			Op:   "Del",
			Key:  append(pw.Key, key),
			Got:  r,
			Want: Writer(nil),
			Err:  ErrUnexpectedType,
		}
	}

	return nil
}

func (pw PrefixedWriter) SafeSet(ctx context.Context, key string, value any) (bool, error) {
	pr, err := pw.reader("Set")
	if err != nil {
		return false, err
	}

	r, err := pr.base(ctx, "Set")
	if err != nil {
		return false, err
	}

	switch w := r.(type) {
	case SafeWriter:
		ok, err := w.SafeSet(ctx, key, value)
		if err != nil {
			return false, &Error{
				Op:  "Set",
				Key: append(pw.Key, key),
				Err: err,
			}
		}

		return ok, nil
	case Writer:
		return w.Set(ctx, key, value), nil
	default:
		return false, &Error{
			Op:   "Set",
			Key:  append(pw.Key, key),
			Got:  r,
			Want: Writer(nil),
			Err:  ErrUnexpectedType,
		}
	}
}

func (pw PrefixedWriter) SafePut(ctx context.Context, key string, hint Type) (Writer, error) {
	var (
		w, k    = pw.writer()
		normkey = append(k, key)
		err     error
	)

	for i, key := range normkey {
		if sw, ok := w.(SafeWriter); ok {
			if w, err = sw.SafePut(ctx, key, hint); err != nil {
				return nil, &Error{
					Op:  "Put",
					Key: normkey[:i+1],
					Got: sw,
					Err: err,
				}
			}
		} else {
			w = w.Put(ctx, key, hint)
		}
	}

	return w, nil
}

func (pw PrefixedWriter) reader(op string) (PrefixedReader, error) {
	w, key := pw.writer()
	r, ok := w.(Reader)
	if !ok {
		return PrefixedReader{}, &Error{
			Op:   op,
			Key:  pw.Key,
			Got:  pw.W,
			Want: Reader(nil),
			Err:  ErrUnexpectedType,
		}
	}

	return PrefixedReader{Key: key, R: r}, nil
}

func (pw PrefixedWriter) writer() (Writer, Key) {
	const maxDepth = 128

	key := pw.Key

	for i := 0; i < maxDepth; i++ {
		switch x := pw.W.(type) {
		case PrefixedWriter:
			pw = x
			key.Prepend(pw.Key)
		case Prefixed:
			pw = x.PrefixedWriter
			key.Prepend(pw.Key)
		default:
			return pw.W, key
		}
	}

	return nil, nil
}
