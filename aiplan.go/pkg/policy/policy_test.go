package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
)

// stubAuthorizer — движок с заранее заданным ответом.
type stubAuthorizer struct {
	verdict engine.Verdict
	err     error
	calls   int
}

func (s *stubAuthorizer) Authorize(context.Context, engine.AuthzRequest) (engine.Verdict, error) {
	s.calls++
	return s.verdict, s.err
}

const act = engine.ActionIssueUpdate

func TestAuthorizePrimaryDecides(t *testing.T) {
	cases := []struct {
		name        string
		primary     engine.Verdict
		wantErr     bool
		wantFbCalls int
	}{
		{name: "разрешил — запасной не спрашивается", primary: engine.Allow, wantErr: false},
		{name: "запретил — запасной не спрашивается", primary: engine.Deny, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			primary := &stubAuthorizer{verdict: c.primary}
			fallback := &stubAuthorizer{verdict: engine.Allow}

			err := New(primary, fallback).Authorize(context.Background(), act, nil)

			if (err != nil) != c.wantErr {
				t.Fatalf("ошибка %v, ожидалась=%v", err, c.wantErr)
			}
			if fallback.calls != c.wantFbCalls {
				t.Errorf("запасной движок вызван %d раз, ожидалось %d", fallback.calls, c.wantFbCalls)
			}
		})
	}
}

// Движок без мнения передаёт решение ядру.
func TestAuthorizeFallsBackOnDefault(t *testing.T) {
	primary := &stubAuthorizer{verdict: engine.Default}
	fallback := &stubAuthorizer{verdict: engine.Allow}

	if err := New(primary, fallback).Authorize(context.Background(), act, nil); err != nil {
		t.Fatalf("запасной движок разрешил, но получен отказ: %v", err)
	}
	if fallback.calls != 1 {
		t.Errorf("запасной движок вызван %d раз, ожидался 1", fallback.calls)
	}
}

// Никто не принял решения — запрет. Расширение прав не должно
// возникать из отсутствия правила.
func TestAuthorizeDeniesWhenNobodyDecides(t *testing.T) {
	primary := &stubAuthorizer{verdict: engine.Default}
	fallback := &stubAuthorizer{verdict: engine.Default}

	err := New(primary, fallback).Authorize(context.Background(), act, nil)
	if err == nil {
		t.Fatal("без решения обоих движков доступ разрешён")
	}
	if !errors.Is(err, apierrors.ErrIssueForbidden) {
		t.Errorf("ошибка %v, ожидался отказ в доступе", err)
	}
}

// Движок не задан вовсе — запрет, а не паника.
func TestAuthorizeWithoutEngines(t *testing.T) {
	if err := New(nil, nil).Authorize(context.Background(), act, nil); err == nil {
		t.Fatal("без движков доступ разрешён")
	}
}

// Ошибка движка не превращается в отказ: причина должна дойти до вызывающего.
func TestAuthorizePropagatesError(t *testing.T) {
	boom := errors.New("сбой загрузки")
	primary := &stubAuthorizer{err: boom}
	fallback := &stubAuthorizer{verdict: engine.Allow}

	err := New(primary, fallback).Authorize(context.Background(), act, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("ошибка %v, ожидалась %v", err, boom)
	}
	if fallback.calls != 0 {
		t.Error("после ошибки основного движка спрошен запасной")
	}
}

// Собственная ошибка движка доходит до клиента вместо стандартной.
func TestAuthorizeUsesVerdictError(t *testing.T) {
	custom := apierrors.ErrForbiddenState
	primary := &stubAuthorizer{verdict: engine.Verdict{
		Decision: engine.DecisionDeny,
		Error:    &custom,
	}}

	err := New(primary, nil).Authorize(context.Background(), act, nil)
	if !errors.Is(err, custom) {
		t.Fatalf("ошибка %v, ожидалась %v", err, custom)
	}
}

// Отказ хотя бы по одному действию запрещает операцию целиком.
func TestAuthorizeAll(t *testing.T) {
	deny := &stubAuthorizer{verdict: engine.Deny}
	if err := New(deny, nil).AuthorizeAll(context.Background(),
		[]engine.Action{engine.ActionIssueSetState}, nil); err == nil {
		t.Fatal("запрещённое действие пропущено")
	}

	allow := &stubAuthorizer{verdict: engine.Allow}
	actions := []engine.Action{engine.ActionIssueSetState, engine.ActionIssueSetLabels}
	if err := New(allow, nil).AuthorizeAll(context.Background(), actions, nil); err != nil {
		t.Fatalf("разрешённые действия отклонены: %v", err)
	}
	if allow.calls != len(actions) {
		t.Errorf("проверено %d действий, ожидалось %d", allow.calls, len(actions))
	}

	// Пустой набор — проверять нечего.
	if err := New(deny, nil).AuthorizeAll(context.Background(), nil, nil); err != nil {
		t.Errorf("пустой набор действий отклонён: %v", err)
	}
}
