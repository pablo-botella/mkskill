package compiler

import (
	"context"
)

type EncoderParams struct {
	IncludeScanData bool
	PrettyOutput    bool
}

// ctxKey is the private type for this package's context keys: no other
// package can forge or collide with it.
type ctxKey int

const encoderParamsKey ctxKey = 0

func WithEncoderParams(ctx context.Context, params *EncoderParams) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, encoderParamsKey, params)
}

// GetEncoderParams recovers the params carried by the run's context; if
// nobody put them there (or ctx is nil), safe defaults — never nil.
func GetEncoderParams(ctx context.Context) *EncoderParams {
	if ctx == nil {
		return &EncoderParams{}
	}
	params, ok := ctx.Value(encoderParamsKey).(*EncoderParams)
	if !ok {
		return &EncoderParams{}
	}
	return params
}
