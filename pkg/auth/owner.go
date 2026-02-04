package auth

import "context"

type Owner struct {
	Name string
}

type ownerKeyType struct{}

var ownerKey = ownerKeyType{}

func WithOwner(ctx context.Context, o Owner) context.Context {
	return context.WithValue(ctx, ownerKey, o)
}

func OwnerFrom(ctx context.Context) (Owner, bool) {
	o, ok := ctx.Value(ownerKey).(Owner)
	return o, ok
}
