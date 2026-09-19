package ports

import "context"

type Payment interface {
	Authorize(context.Context, string) error
}
