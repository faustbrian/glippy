// Package workload is deliberately dense valid Go used by Phase 0 baselines.
package workload

import (
	"context"
	"errors"
)

var errContextRequired = errors.New("context required")

type client[T any] struct{ values []T }

func (c *client[T]) discover(context.Context) error { return errContextRequired }

func (c *client[T]) execute(ctx context.Context, values ...T) (T, error) {
	var zero T
	if len(values) == 0 {
		return zero, errors.New("missing value")
	}
	return values[0], nil
}

func run(ctx context.Context, c *client[string], foo, bar, baz bool) (string, error) {
	if err := c.discover(ctx); !errors.Is(err, errContextRequired) {
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result, err := c.execute(ctx, "operation", "GET", "/", "application/json")
	if foo && bar && baz && result != "" {
		return result, err
	}
	return "", err
}
