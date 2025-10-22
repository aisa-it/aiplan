package limiter

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
)

type LimiterInt interface {
	GetWorkspaceLimitInfo(creatorId uuid.UUID, workspaceId uuid.UUID) dto.WorkspaceLimitsInfo

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
	if cfg.ExternalLimiter == nil {
		slog.Info("Using Community limiter")
		return
	}
	Limiter = NewExternalLimiter(cfg.ExternalLimiter)
}

type CommunityLimiter struct{}

func (c CommunityLimiter) GetWorkspaceLimitInfo(creatorId uuid.UUID, workspaceId uuid.UUID) dto.WorkspaceLimitsInfo {
	return dto.WorkspaceLimitsInfo{
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
