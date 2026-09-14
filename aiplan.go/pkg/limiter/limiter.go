package limiter

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/gofrs/uuid"
)

type LimiterInt interface {
	GetWorkspaceLimitInfo(workspaceId uuid.UUID) *dto.WorkspaceLimitsInfo

	CanCreateWorkspace(userId uuid.UUID) bool
	CanCreateProject(workspaceId uuid.UUID) bool
	CanAddWorkspaceMember(workspaceId uuid.UUID) bool
	CanAddAttachment(workspaceId uuid.UUID) bool

	GetRemainingWorkspaces(userId uuid.UUID) int
	GetRemainingProjects(workspaceId uuid.UUID) int
	GetRemainingInvites(workspaceId uuid.UUID) int
	GetRemainingAttachments(workspaceId uuid.UUID) int
}

var Limiter LimiterInt = CommunityLimiter{}

func Init(cfg *config.Config) {
	if cfg.ExternalLimiter.URL == nil {
		slog.Info("Using Community limiter")
		return
	}
	Limiter = NewExternalLimiter(cfg.ExternalLimiter.URL)
}

// SetLimiter подставляет собственную реализацию лимитов — для сборок,
// которые считают лимиты по своим правилам и не используют внешний сервис.
// Вызывать до старта сервера.
func SetLimiter(l LimiterInt) {
	if l == nil {
		return
	}
	Limiter = l
}

type CommunityLimiter struct{}

func (c CommunityLimiter) GetWorkspaceLimitInfo(workspaceId uuid.UUID) *dto.WorkspaceLimitsInfo {
	return &dto.WorkspaceLimitsInfo{
		TariffName: "community",
	}
}

func (c CommunityLimiter) CanCreateWorkspace(userId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanCreateProject(workspaceId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanAddWorkspaceMember(workspaceId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) CanAddAttachment(workspaceId uuid.UUID) bool {
	return true
}

func (c CommunityLimiter) GetRemainingWorkspaces(userId uuid.UUID) int {
	return 99999999
}
func (c CommunityLimiter) GetRemainingProjects(workspaceId uuid.UUID) int {
	return 99999999
}
func (c CommunityLimiter) GetRemainingInvites(workspaceId uuid.UUID) int {
	return 99999999
}
func (c CommunityLimiter) GetRemainingAttachments(workspaceId uuid.UUID) int {
	return 99999999999999
}
