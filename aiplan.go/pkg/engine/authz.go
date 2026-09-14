package engine

import "context"

// AuthzRequest — запрос на проверку права.
type AuthzRequest struct {
	Action  Action
	Subject Subject
}

// Authorizer — движок, управляющий правами доступа.
// Опциональный: не реализован — права считает ядро.
type Authorizer interface {
	Authorize(ctx context.Context, req AuthzRequest) (Verdict, error)
}
