package limiter

import (
	"log/slog"
	"plugin"

	"github.com/gofrs/uuid"
)

type LimiterInt interface {
	CanCreateWorkspace(userId uuid.UUID) bool
	CanCreateProject(userId uuid.UUID, workspaceId uuid.UUID) bool
	CanAddWorkspaceMember(userId uuid.UUID, workspaceId uuid.UUID) bool
	CanAddAttachment(userId uuid.UUID, workspaceId uuid.UUID) bool
}

var Limiter LimiterInt = CommunityLimiter{}

func Init(pluginPath string) {
	if pluginPath == "" {
		slog.Info("Using Community limiter")
		return
	}
	p, err := plugin.Open(pluginPath)
	if err != nil {
		slog.Error("Fail open limiter plugin, backoff to Community limiter", "err", err)
		return
	}

	limiterSymbol, err := p.Lookup("LimiterPlugin")
	if err != nil {
		slog.Error("Fail lookup LimiterPlugin symbol, backoff to Community limiter", "err", err)
		return
	}

	var ok bool
	Limiter, ok = limiterSymbol.(LimiterInt)
	if !ok {
		slog.Error("Plugin doesn't implement LimiterInt interface, backoff to Community limiter")
		Limiter = CommunityLimiter{}
		return
	}
}

type CommunityLimiter struct{}

func (c CommunityLimiter) CanCreateWorkspace(userId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanCreateProject(userId uuid.UUID, workspaceId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanAddWorkspaceMember(userId uuid.UUID, workspaceId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanAddAttachment(userId uuid.UUID, workspaceId uuid.UUID) bool {
	return true
}
