package ops

import (
	"context"
	"errors"
)

func Transform[T any](value T, fail bool) (T, error) {
	if fail {
		return value, errors.New("generic function failed")
	}
	return value, nil
}

type Store[T any] struct{}

func (Store[T]) Apply(value T, fail bool) (T, error) {
	if fail {
		return value, errors.New("generic method failed")
	}
	return value, nil
}

func (*Store[T]) PointerApply(value T) (T, error) {
	return value, nil
}

func Panic[T any](value T) {
	panic("original generic panic")
}

func Batch[T any](values []T) ([]T, error) {
	return values, nil
}

func Contextual[T any](ctx context.Context, value T) (T, error) {
	return value, nil
}
