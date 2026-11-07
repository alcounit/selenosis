package auth

import "context"

type credentialsKeyType struct{}

var credentialsKey = credentialsKeyType{}

func WithCredentials(ctx context.Context, username, password string) context.Context {
	return context.WithValue(ctx, credentialsKey, credentials{User: username, Pass: password})
}

func CredentialsFrom(ctx context.Context) (credentials, bool) {
	c, ok := ctx.Value(credentialsKey).(credentials)
	return c, ok
}
