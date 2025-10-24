package limiter

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
)

type ExternalLimiter struct {
	host *url.URL
}

func NewExternalLimiter(host *url.URL) *ExternalLimiter {
	return &ExternalLimiter{host: host}
}

func (c ExternalLimiter) GetWorkspaceLimitInfo(workspaceId uuid.UUID) *dto.WorkspaceLimitsInfo {
	resp, err := http.Get(c.host.ResolveReference(&url.URL{Path: "/tariff/" + workspaceId.String()}).String())
	if err != nil {
		slog.Error("Request tariff info", "workspace", workspaceId, "err", err)
		return nil
	}
	defer resp.Body.Close()

	var info dto.WorkspaceLimitsInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		slog.Error("Parse tariff info", "workspace", workspaceId, "err", err)
		return nil
	}
	return &info
}

func (c ExternalLimiter) CanCreateWorkspace(userId uuid.UUID) bool {
	return c.doRequest("/can/create/workspace/by/" + userId.String())
}

func (c ExternalLimiter) CanCreateProject(workspaceId uuid.UUID) bool {
	return c.doRequest("/can/create/workspace/" + workspaceId.String() + "/project")
}

func (c ExternalLimiter) CanAddWorkspaceMember(workspaceId uuid.UUID) bool {
	return c.doRequest("/can/add/workspace/" + workspaceId.String() + "/member")
}

func (c ExternalLimiter) CanAddAttachment(workspaceId uuid.UUID) bool {
	return c.doRequest("/can/add/workspace/" + workspaceId.String() + "/attachment")
}

func (c ExternalLimiter) GetRemainingWorkspaces(userId uuid.UUID) int {
	return c.doRemainRequest("/remain/workspaces/by/" + userId.String())
}
func (c ExternalLimiter) GetRemainingProjects(workspaceId uuid.UUID) int {
	return c.doRemainRequest("/remain/workspace/" + workspaceId.String() + "/projects")
}
func (c ExternalLimiter) GetRemainingInvites(workspaceId uuid.UUID) int {
	return c.doRemainRequest("/remain/workspace/" + workspaceId.String() + "/invites")
}
func (c ExternalLimiter) GetRemainingAttachments(workspaceId uuid.UUID) int {
	return c.doRemainRequest("/remain/workspace/" + workspaceId.String() + "/attachments")
}

func (c ExternalLimiter) doRemainRequest(path string) int {
	resp, err := http.Get(c.host.ResolveReference(&url.URL{Path: path}).String())
	if err != nil {
		slog.Error("Request remains", "err", err)
		return -1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1
	}

	remain, err := strconv.Atoi(resp.Header.Get("X-Entity-Remain"))
	if err != nil {
		slog.Error("Parse remain answer", "raw", resp.Header.Get("X-Entity-Remain"), "err", err)
		return -1
	}
	return remain
}

func (c ExternalLimiter) doRequest(path string) bool {
	resp, err := http.Get(c.host.ResolveReference(&url.URL{Path: path}).String())
	if err != nil {
		slog.Error("Request access rule", "err", err)
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
