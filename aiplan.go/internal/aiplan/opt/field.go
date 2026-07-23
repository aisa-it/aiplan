package opt

type Field[T any] struct {
	value T
	set   bool
}

func Some[T any](v T) Field[T] {
	return Field[T]{value: v, set: true}
}

func None[T any]() Field[T] {
	return Field[T]{}
}

func (f Field[T]) Value() T {
	return f.value
}

func (f Field[T]) IsSet() bool {
	return f.set
}
