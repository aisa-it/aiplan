// Пакет aiplan предоставляет функциональность для миграции задач между проектами в системе планирования.
// Он включает в себя копирование задач, обновление связанных данных (комментарии, реакции, ссылки), и обработку изменений статусов задач.
//
// Основные возможности:
//   - Копирование задач с сохранением связанных данных.
//   - Обновление статусов задач при миграции.
//   - Обработка связанных задач (комментарии, реакции, ссылки).
//   - Поддержка миграции задач с учетом различных типов статусов и меток.
package aiplan

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTargetProjectNotFound  = "target project not found"
	ErrMigrationIssueNotFound = "issue not found"
	ErrLabelNotFound          = "label not found"
	ErrConflictIssuesNames    = "issues with conflicted names"

	ErrAuthorNotAProjectMember = "source author not a target project member"
	ErrUserNotAProjectMember   = "you are not a target project member"
	ErrAssigneesNotFound       = "source assignees that not a members of target project"
	ErrWatchersNotFound        = "source watchers that not a members of target project"
	ErrAssigneeRoleInvalid     = "issue assignment is not allowed for assignee with current role of target project"
	ErrWatcherRoleInvalid      = "issue watching is not allowed for watcher with current role of target project"

	ErrStateNotFound  = "source state that does not exist in target project"
	ErrLabelsNotFound = "source labels that does not exist in target project"

	ErrOther = "Error"
)

type ErrClause struct {
	Error           string     `json:"error"`
	SrcIssueId      *uuid.UUID `json:"src_issue_id,omitempty"`
	IssueSequenceId int        `json:"issue_sequence_id,omitempty"`
	Type            string     `json:"type,omitempty"`
	Entities        []string   `json:"entities,omitempty"`
}

func (s *Services) AddIssueMigrationServices(g *echo.Group) {
	g.POST("workspaces/:workspaceSlug/issues/migrate/", s.migrateIssues)
	g.POST("workspaces/:workspaceSlug/issues/migrate/byLabel/", s.migrateIssuesByLabel)
}

// migrateIssues godoc
// @id migrateIssues
// @Summary Задачи (миграции): миграция задачи рабочего пространства
// @Description Мигрирует задачу из одного проекта в другой с опциональной поддержкой связанных задач и удаления исходной задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param target_project query string true "ID целевого проекта"
// @Param src_issue query string true "ID исходной задачи"
// @Param linked_issues query bool false "Мигрировать связанные задачи" default(false)
// @Param delete_src query bool false "Удалить исходную задачу после миграции" default(false)
// @Param create_entities query bool false "Создать не достающие label, state" default(false)
// @Param data body NewIssueParam false "Идентификаторы связанных задач"
// @Success 201 {object} NewIssueID "ID созданной задачи"
// @Failure 400 {object} map[string]interface{} "Некорректные параметры запроса или данные"
// / @Failure 404 {object} apierrors.DefinedError "Целевой проект или исходная задача не найдены"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/issues/migrate [post]
func (s *Services) migrateIssues(c echo.Context) error {
	user := *c.(AuthContext).User

	targetProjectId := ""
	srcIssueId := ""
	linkedIssues := false
	deleteSrc := false
	createEntities := false

	if err := echo.QueryParamsBinder(c).
		String("target_project", &targetProjectId).
		String("src_issue", &srcIssueId).
		Bool("linked_issues", &linkedIssues).
		Bool("delete_src", &deleteSrc).
		Bool("create_entities", &createEntities).
		BindError(); err != nil {
		return EError(c, err)
	}

	var param *NewIssueParam
	if err := c.Bind(&param); err != nil && !errors.Is(err, io.EOF) {
		return EError(c, err)
	}

	var labelIds, stateIds []string
	stateMap := make(map[string]dao.State)

	var targetProject, srcProject dao.Project

	if err := s.db.Where("id = ?", targetProjectId).First(&targetProject).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": []ErrClause{
					{
						Error: ErrTargetProjectNotFound,
						Type:  "project",
					},
				},
			})
		}
		return EError(c, err)
	}

	var srcIssue dao.Issue
	if err := s.db.Where("id = ?", srcIssueId).
		Preload(clause.Associations).
		First(&srcIssue).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": []ErrClause{
					{
						Error: ErrMigrationIssueNotFound,
						Type:  "issue",
					},
				},
			})
		}
		return EError(c, err)
	}
	if linkedIssues == false && param != nil {

		if v, ok := param.StateId.GetValue(); ok {
			var state dao.State
			if err := s.db.Where("workspace_id = ?", srcIssue.WorkspaceId).
				Where("project_id = ?", srcIssue.ProjectId).
				Where("id = ?", param.StateId.String()).
				First(&state).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return c.JSON(http.StatusBadRequest, map[string]interface{}{
						"errors": []ErrClause{
							{
								Error: ErrStateNotFound,
								Type:  "issue",
							},
						},
					})
				}
				return EError(c, err)
			}

			srcIssue.StateId = v
			srcIssue.State = &state
		}

		if v, ok := param.Priority.GetValue(); ok {
			srcIssue.Priority = v
		}

		if v, ok := param.AssignersIds.GetValue(); ok {
			if v == nil {
				srcIssue.AssigneeIDs = []string{}
			} else {
				srcIssue.AssigneeIDs = *v
			}
		}

		if v, ok := param.TargetDate.GetValue(); ok {
			if v == nil {
				srcIssue.TargetDate = nil
			} else {
				date, err := utils.FormatDateStr(*v, "2006-01-02T15:04:05Z07:00", nil)
				if err != nil {
					return EErrorDefined(c, apierrors.ErrGeneric)
				}

				if d, err := utils.FormatDate(date); err != nil {
					return EErrorDefined(c, apierrors.ErrGeneric)
				} else {
					if time.Now().After(d) {
						return EErrorDefined(c, apierrors.ErrIssueTargetDateExp)
					}
					srcIssue.TargetDate = &types.TargetDateTimeZ{Time: d}
				}
			}
		}
	}

	err := srcIssue.FetchLinkedIssues(s.db)
	if err != nil {
		return EError(c, err)
	}

	srcProject = *srcIssue.Project
	if srcProject.ID == targetProject.ID {
		linkedIssues = false
		deleteSrc = false
		createEntities = false
	}

	var srcIssues []dao.Issue
	if linkedIssues {
		srcIssues = dao.GetIssueFamily(srcIssue, s.db)
	} else {
		srcIssues = []dao.Issue{srcIssue}
	}

	if len(srcIssues) == 0 {
		return c.NoContent(http.StatusNotModified)
	}

	idsMap := make(map[uuid.UUID]uuid.UUID)
	idsCommentMap := make(map[uuid.UUID]uuid.UUID)
	linkedIds := make(map[string]struct{})
	var srcIssueUUId uuid.UUID
	var familyIds, newFamilyIds []string
	preparedIssues := make([]IssueCheckResult, len(srcIssues))

	// Checks
	{
		var errors []ErrClause
		if _, exist := dao.IsProjectMember(s.db, user.ID, targetProjectId); !exist {
			errors = append(errors, ErrClause{
				Error: ErrUserNotAProjectMember,
			})
		}

		for i, issue := range srcIssues {
			result, err := s.CheckIssueBeforeMigrate(issue, targetProject)
			if err != nil {
				return EError(c, err)
			}
			if deleteSrc {
				srcIssues[i].ProjectId = targetProjectId
				srcIssues[i].Project = &targetProject
				srcIssues[i].StateId = &result.TargetState.ID
				if result.TargetState.ID != "" {
					srcIssues[i].State = &result.TargetState
				}
			}
			errors = append(errors, result.Errors...)
			preparedIssues[i] = result
			srcIssueUUId = result.SrcIssue.ID
			idsMap[result.SrcIssue.ID] = result.TargetId
			familyIds = append(familyIds, result.SrcIssue.ID.String())
			newFamilyIds = append(newFamilyIds, result.TargetId.String())
		}

		if !deleteSrc {
			for _, issue := range srcIssues {
				newId1, okId1 := idsMap[issue.ID]
				if !okId1 {
					continue
				}
				for _, id := range issue.LinkedIssuesIDs {
					newId2, okId2 := idsMap[id]

					if okId2 {
						key := linkedIdToStringKey(newId1.String(), newId2.String())
						linkedIds[key] = struct{}{}
					}
				}
			}
		}

		var comments []dao.IssueComment
		if err := s.db.Where("issue_id in ?", familyIds).Find(&comments).Error; err != nil {
			return EError(c, err)
		}
		for _, comment := range comments {
			idsCommentMap[comment.Id] = dao.GenUUID()
		}

		if createEntities {
			newErr := make([]ErrClause, 0)
			for _, errClause := range errors {
				switch errClause.Type {
				case "state":
					stateIds = append(stateIds, errClause.Entities...)
				case "label":
					labelIds = append(labelIds, errClause.Entities...)
				default:
					newErr = append(newErr, errClause)
				}
			}
			errors = newErr
		}

		if len(errors) > 0 {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": errors,
			})
		}
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {

		if len(labelIds) > 0 {
			var labels []dao.Label
			if err := tx.
				Where("workspace_id = ?", targetProject.WorkspaceId).
				Where("id in (?)", labelIds).Find(&labels).Error; err != nil {
				return err
			}

			var newLabels []dao.Label
			for _, label := range labels {
				id := dao.GenUUID()
				l := dao.Label{
					ID:          id.String(),
					Name:        label.Name,
					Description: label.Description,
					Color:       label.Color,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,

					WorkspaceId: targetProject.WorkspaceId,
					ProjectId:   targetProject.ID,
				}
				newLabels = append(newLabels, l)
				uuidLabel, err := uuid.FromString(label.ID)
				if err != nil {
					return err
				}
				idsMap[uuidLabel] = id
			}
			if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
				return err
			}
		}

		if len(stateIds) > 0 {
			var states []dao.State
			if err := tx.
				Where("workspace_id = ?", targetProject.WorkspaceId).
				Where("id in (?)", stateIds).Find(&states).Error; err != nil {
				return err
			}

			for _, state := range states {
				id := dao.GenUUID()
				st := dao.State{
					ID:          id.String(),
					Name:        state.Name,
					Description: state.Description,
					Color:       state.Color,
					Slug:        state.Slug,
					Group:       state.Group,

					CreatedById: &user.ID,
					UpdatedById: &user.ID,

					WorkspaceId: targetProject.WorkspaceId,
					ProjectId:   targetProject.ID,
				}

				stateMap[st.ID] = st
				if err := updateStatesGroup(tx, &st, "create"); err != nil {
				}
				uuidState, err := uuid.FromString(state.ID)
				if err != nil {
					return err
				}
				idsMap[uuidState] = id
			}
		}
		return nil
	}); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []ErrClause{
				{
					Error: ErrOther,
					Type:  "transfer",
				},
			},
		})
	}

	// Create issues
	if err := s.db.Transaction(func(tx *gorm.DB) error {

		for _, issue := range preparedIssues {
			if deleteSrc {
				if err := migrateIssueMove(issue, user, tx, idsMap, !linkedIssues); err != nil {
					return err
				}
			} else {
				if err := migrateIssueCopy(issue, user, tx, idsMap, idsCommentMap, !linkedIssues); err != nil {
					return err
				}
			}
		}

		if deleteSrc {
			var seqId int
			// Calculate sequence id
			var lastId sql.NullInt64
			row := tx.Model(&dao.Issue{}).
				Select("max(sequence_id)").
				Unscoped().
				Where("project_id = ?", targetProjectId).
				Row()
			if err := row.Scan(&lastId); err != nil {
				return err
			}

			// Just use the last ID specified (which should be the greatest) and add one to it
			if lastId.Valid {
				seqId = int(lastId.Int64)
			} else {
				seqId = 0
			}

			stateCreated := createEntities && len(stateIds) > 0

			for i := range srcIssues {
				seqId++
				srcIssues[i].SequenceId = seqId
				if !linkedIssues && srcIssues[i].Parent != nil {
					srcIssues[i].Parent = nil
					srcIssues[i].ParentId = uuid.NullUUID{}
				}

				if stateCreated && srcIssues[i].StateId != nil && *srcIssues[i].StateId == "" {
					id := srcIssues[i].State.ID
					idSt, err := uuid.FromString(id)
					if err != nil {
						return err
					}
					stateTmp := stateMap[idsMap[idSt].String()]
					srcIssues[i].State = &stateTmp
					srcIssues[i].StateId = &stateTmp.ID
				}
			}

			err := stateActivityUpdate(tx, familyIds, srcIssue.ProjectId, targetProjectId)
			if err != nil {
				return err
			}

			if err := tx.
				Where("(block_id IN (?) AND blocked_by_id NOT IN (?) AND project_id IN (?, ?)) OR (blocked_by_id IN (?) AND block_id NOT IN (?) AND project_id IN (?, ?))",
					familyIds, familyIds, srcIssue.ProjectId, targetProjectId,
					familyIds, familyIds, srcIssue.ProjectId, targetProjectId).
				Delete(&dao.IssueBlocker{}).Error; err != nil {
				return err
			}

			if !linkedIssues {
				if err := tx.Model(&dao.Issue{}).
					Where("id NOT IN (?)", familyIds).
					Where("parent_id IN (?)", familyIds).
					Update("parent_id", nil).Error; err != nil {
					return err
				}
			}

			{
				if err := tx.
					Where("(id1 IN ? and id2 NOT IN ?) OR (id1 NOT IN ? and id2 IN ?)", familyIds, familyIds, familyIds, familyIds).
					Delete(&dao.LinkedIssues{}).Error; err != nil {
					return err
				}
			}

			if err := tx.Omit(clause.Associations).Save(&srcIssues).Error; err != nil {
				return err
			}
		} else {
			for key := range linkedIds {
				ids := strings.Split(key, " ")
				if len(ids) == 2 {
					uuid1, err := uuid.FromString(ids[0])
					if err != nil {
						continue
					}
					uuid2, err := uuid.FromString(ids[1])
					if err != nil {
						continue
					}

					tmp := dao.LinkedIssues{Id1: uuid1, Id2: uuid2}
					if err := tx.Create(&tmp).Error; err != nil {
						continue
					}
				}
			}
		}

		return nil
	}); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []ErrClause{
				{
					Error: ErrOther,
					Type:  "transfer",
				},
			},
		})
	}

	var newId string
	if deleteSrc {
		newId = srcIssueId
		for _, issue := range srcIssues {
			requestMap := make(map[string]interface{})
			currentMap := make(map[string]interface{})

			requestMap["parent_key"] = "project_id"
			requestMap["old_entity"] = "project"
			requestMap["new_entity"] = "project"
			requestMap["old_title"] = issue.Name

			requestMap["project_id"] = targetProject.ID
			currentMap["project_id"] = srcProject.ID
			requestMap["parent_title"] = targetProject.Identifier
			currentMap["parent_title"] = srcProject.Identifier

			err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_MOVE_ACTIVITY, requestMap, currentMap, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}

			err = tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, nil, nil, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}

			delIssue := issue
			delIssue.Project = &srcProject
			delIssue.ProjectId = srcProject.ID

			err = tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_REMOVE_ACTIVITY, requestMap, nil, delIssue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
	} else {
		var newIssues []dao.Issue
		s.db.Joins("Project").Where("issues.id in (?)", newFamilyIds).Find(&newIssues)

		for _, issue := range newIssues {
			data := map[string]interface{}{"custom_verb": "copied"}
			err = tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, data, nil, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
		newId = idsMap[srcIssueUUId].String()
	}

	return c.JSON(http.StatusCreated, NewIssueID{Id: newId})
}

// migrateIssuesByLabel godoc
// @id migrateIssuesByLabel
// @Summary Задачи (миграции): миграция задач по метке рабочего пространства
// @Description Мигрирует все задачи с определенной меткой из одного проекта в другой с опциональной поддержкой связанных задач и удаления исходных задач
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param target_project query string true "ID целевого проекта"
// @Param src_label query string true "ID исходной метки"
// @Param linked_issues query bool false "Мигрировать связанные задачи" default(false)
// @Param delete_src query bool false "Удалить исходные задачи после миграции" default(false)
// @Param create_entities query bool false "Создать не достающие label, state" default(false)
// @Success 204 "Задачи успешно мигрированы"
// @Failure 400 {object} map[string]interface{} "Некорректные параметры запроса или данные"
// @Failure 404 {object} apierrors.DefinedError "Целевой проект или исходная метка не найдены"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/issues/migrate/byLabel [post]
func (s *Services) migrateIssuesByLabel(c echo.Context) error {
	user := *c.(AuthContext).User

	targetProjectId := ""
	srcLabelId := ""
	linkedIssues := false
	deleteSrc := false
	createEntities := false

	if err := echo.QueryParamsBinder(c).
		String("target_project", &targetProjectId).
		String("src_label", &srcLabelId).
		Bool("linked_issues", &linkedIssues).
		Bool("delete_src", &deleteSrc).
		Bool("create_entities", &createEntities).
		BindError(); err != nil {
		return EError(c, err)
	}

	var labelIds, stateIds []string
	stateMap := make(map[string]dao.State)

	var targetProject, srcProject dao.Project
	if err := s.db.Where("id = ?", targetProjectId).First(&targetProject).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": []ErrClause{
					{
						Error: ErrTargetProjectNotFound,
						Type:  "project",
					},
				},
			})
		}
		return EError(c, err)
	}

	var srcLabel dao.Label
	if err := s.db.Where("id = ?", srcLabelId).
		Preload(clause.Associations).
		First(&srcLabel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": []ErrClause{
					{
						Error: ErrLabelNotFound,
						Type:  "label",
					},
				},
			})
		}
		return EError(c, err)
	}

	srcProject = *srcLabel.Project

	var srcIssues []dao.Issue
	if err := s.db.Preload(clause.Associations).Where("id in (?)", s.db.Select("issue_id").Where("label_id = ?", srcLabel.ID).Model(&dao.IssueLabel{})).
		Where("project_id = ?", srcLabel.ProjectId).
		Find(&srcIssues).Error; err != nil {
		return EError(c, err)
	}

	if linkedIssues {
		srcIssueMap := make(map[uuid.UUID]dao.Issue)
		for i, srcIssue := range srcIssues {
			srcIssues[i].FetchLinkedIssues(s.db)
			srcIssueMap[srcIssue.ID] = srcIssue
			for _, issue := range dao.GetIssueFamily(srcIssue, s.db) {
				srcIssueMap[issue.ID] = issue
			}
		}
		srcIssues = srcIssues[:0]
		for _, issue := range srcIssueMap {
			srcIssues = append(srcIssues, issue)
		}
	}

	if len(srcIssues) == 0 {
		return c.NoContent(http.StatusNotModified)
	}

	idsMap := make(map[uuid.UUID]uuid.UUID)
	idsCommentMap := make(map[uuid.UUID]uuid.UUID)
	linkedIds := make(map[string]struct{})

	var targetIds, newTargetIds []string
	preparedIssues := make([]IssueCheckResult, len(srcIssues))

	// Checks
	{
		var errors []ErrClause
		if _, exist := dao.IsProjectMember(s.db, user.ID, targetProjectId); !exist {
			errors = append(errors, ErrClause{
				Error: ErrUserNotAProjectMember,
			})
		}

		for i, issue := range srcIssues {
			result, err := s.CheckIssueBeforeMigrate(issue, targetProject)
			if err != nil {
				return EError(c, err)
			}
			if deleteSrc {
				srcIssues[i].ProjectId = targetProjectId
				srcIssues[i].Project = &targetProject
				srcIssues[i].StateId = &result.TargetState.ID
				if result.TargetState.ID != "" {
					srcIssues[i].State = &result.TargetState
				}
			}
			errors = append(errors, result.Errors...)
			preparedIssues[i] = result
			idsMap[result.SrcIssue.ID] = result.TargetId
			targetIds = append(targetIds, result.SrcIssue.ID.String())
			newTargetIds = append(newTargetIds, result.TargetId.String())
		}

		if !deleteSrc {
			for _, issue := range srcIssues {
				newId1, okId1 := idsMap[issue.ID]
				if !okId1 {
					continue
				}
				for _, id := range issue.LinkedIssuesIDs {
					newId2, okId2 := idsMap[id]

					if okId2 {
						key := linkedIdToStringKey(newId1.String(), newId2.String())
						linkedIds[key] = struct{}{}
					}
				}
			}
		}

		var comments []dao.IssueComment
		if err := s.db.Where("issue_id in ?", targetIds).Find(&comments).Error; err != nil {
			return EError(c, err)
		}
		for _, comment := range comments {
			idsCommentMap[comment.Id] = dao.GenUUID()
		}

		if createEntities {
			newErr := make([]ErrClause, 0)
			for _, errClause := range errors {
				switch errClause.Type {
				case "state":
					stateIds = append(stateIds, errClause.Entities...)
				case "label":
					labelIds = append(labelIds, errClause.Entities...)
				default:
					newErr = append(newErr, errClause)
				}
			}
			errors = newErr
		}

		if len(errors) > 0 {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"errors": errors,
			})
		}
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {

		if len(labelIds) > 0 {
			var labels []dao.Label
			if err := tx.
				Where("workspace_id = ?", targetProject.WorkspaceId).
				Where("id in (?)", labelIds).Find(&labels).Error; err != nil {
				return err
			}

			var newLabels []dao.Label
			for _, label := range labels {
				id := dao.GenUUID()
				l := dao.Label{
					ID:          id.String(),
					Name:        label.Name,
					Description: label.Description,
					Color:       label.Color,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,

					WorkspaceId: targetProject.WorkspaceId,
					ProjectId:   targetProject.ID,
				}
				newLabels = append(newLabels, l)
				uuidLabel, err := uuid.FromString(label.ID)
				if err != nil {
					return err
				}
				idsMap[uuidLabel] = id
			}
			if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
				return err
			}
		}

		if len(stateIds) > 0 {
			var states []dao.State
			if err := tx.
				Where("workspace_id = ?", targetProject.WorkspaceId).
				Where("id in (?)", stateIds).Find(&states).Error; err != nil {
				return err
			}

			for _, state := range states {
				id := dao.GenUUID()
				st := dao.State{
					ID:          id.String(),
					Name:        state.Name,
					Description: state.Description,
					Color:       state.Color,
					Slug:        state.Slug,
					Group:       state.Group,

					CreatedById: &user.ID,
					UpdatedById: &user.ID,

					WorkspaceId: targetProject.WorkspaceId,
					ProjectId:   targetProject.ID,
				}

				stateMap[st.ID] = st
				if err := updateStatesGroup(tx, &st, "create"); err != nil {
				}
				uuidState, err := uuid.FromString(state.ID)
				if err != nil {
					return err
				}
				idsMap[uuidState] = id
			}
		}
		return nil
	}); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []ErrClause{
				{
					Error: ErrOther,
					Type:  "transfer",
				},
			},
		})
	}

	// Create issues
	if err := s.db.Transaction(func(tx *gorm.DB) error {

		for _, issue := range preparedIssues {
			if deleteSrc {
				if err := migrateIssueMove(issue, user, tx, idsMap, !linkedIssues); err != nil {
					return err
				}
			} else {
				if err := migrateIssueCopy(issue, user, tx, idsMap, idsCommentMap, !linkedIssues); err != nil {
					return err
				}
			}
		}

		if deleteSrc {
			var seqId int
			// Calculate sequence id
			var lastId sql.NullInt64
			row := tx.Model(&dao.Issue{}).
				Select("max(sequence_id)").
				Unscoped().
				Where("project_id = ?", targetProjectId).
				Row()
			if err := row.Scan(&lastId); err != nil {
				return err
			}

			// Just use the last ID specified (which should be the greatest) and add one to it
			if lastId.Valid {
				seqId = int(lastId.Int64)
			} else {
				seqId = 0
			}

			stateCreated := createEntities && len(stateIds) > 0

			for i := range srcIssues {
				seqId++
				srcIssues[i].SequenceId = seqId
				if !linkedIssues && srcIssues[i].Parent != nil {
					if _, ok := idsMap[srcIssues[i].Parent.ID]; !ok {
						srcIssues[i].Parent = nil
						srcIssues[i].ParentId = uuid.NullUUID{}
					}
				}

				if stateCreated && srcIssues[i].StateId != nil && *srcIssues[i].StateId == "" {
					id := srcIssues[i].State.ID
					idSt, err := uuid.FromString(id)
					if err != nil {
						return err
					}
					stateTmp := stateMap[idsMap[idSt].String()]
					srcIssues[i].State = &stateTmp
					srcIssues[i].StateId = &stateTmp.ID
				}
			}

			err := stateActivityUpdate(tx, targetIds, srcProject.ID, targetProjectId)
			if err != nil {
				return err
			}

			if err := tx.
				Where("(block_id IN (?) AND blocked_by_id NOT IN (?) AND project_id IN (?, ?)) OR (blocked_by_id IN (?) AND block_id NOT IN (?) AND project_id IN (?, ?))",
					targetIds, targetIds, srcLabel.ProjectId, targetProjectId,
					targetIds, targetIds, srcLabel.ProjectId, targetProjectId).
				Delete(&dao.IssueBlocker{}).Error; err != nil {
				return err
			}

			if !linkedIssues {
				if err := tx.Model(&dao.Issue{}).
					Where("id NOT IN (?)", targetIds).
					Where("parent_id IN (?)", targetIds).
					Update("parent_id", nil).Error; err != nil {
					return err
				}
			}

			{
				if err := tx.
					Where("(id1 IN ? and id2 NOT IN ?) OR (id1 NOT IN ? and id2 IN ?)", targetIds, targetIds, targetIds, targetIds).
					Delete(&dao.LinkedIssues{}).Error; err != nil {
					return err
				}
			}

			if err := tx.Omit(clause.Associations).Save(&srcIssues).Error; err != nil {
				return err
			}
		} else {
			for key := range linkedIds {
				ids := strings.Split(key, " ")
				if len(ids) == 2 {
					uuid1, err := uuid.FromString(ids[0])
					if err != nil {
						continue
					}
					uuid2, err := uuid.FromString(ids[1])
					if err != nil {
						continue
					}

					tmp := dao.LinkedIssues{Id1: uuid1, Id2: uuid2}
					if err := tx.Create(&tmp).Error; err != nil {
						continue
					}
				}
			}
		}

		return nil
	}); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"errors": []ErrClause{
				{
					Error: ErrOther,
					Type:  "transfer",
				},
			},
		})
	}

	//errorActivity := map[string]interface{}{
	//  "errors": []ErrClause{
	//    {
	//      Error: ErrOther,
	//      Type:  "Activity",
	//    },
	//  },
	//}
	//if errorActivity != nil {
	//  return c.JSON(http.StatusBadRequest, errorActivity)
	//}

	if deleteSrc {
		for _, issue := range srcIssues {
			requestMap := make(map[string]interface{})
			currentMap := make(map[string]interface{})

			requestMap["parent_key"] = "project_id"
			requestMap["old_entity"] = "project"
			requestMap["new_entity"] = "project"
			requestMap["old_title"] = issue.Name

			requestMap["project_id"] = targetProject.ID
			currentMap["project_id"] = srcProject.ID
			requestMap["parent_title"] = targetProject.Identifier
			currentMap["parent_title"] = srcProject.Identifier

			err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_MOVE_ACTIVITY, requestMap, currentMap, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}

			err = tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, nil, nil, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}

			delIssue := issue
			delIssue.Project = &srcProject
			delIssue.ProjectId = srcProject.ID

			err = tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_REMOVE_ACTIVITY, requestMap, nil, delIssue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
	} else {
		var newIssues []dao.Issue
		s.db.Joins("Project").Where("issues.id in (?)", newTargetIds).Find(&newIssues)

		for _, issue := range newIssues {
			err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, issue, &user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
	}

	return c.NoContent(http.StatusCreated)
}

type IssueCheckResult struct {
	SrcIssue      dao.Issue
	TargetProject dao.Project
	TargetId      uuid.UUID

	TargetState  dao.State
	TargetLabels []dao.Label
	MapLabelIds  map[string]string

	Migrate bool
	Errors  []ErrClause
}

type stateTarget struct {
	Str      string
	Id       string
	Relation bool
}

func (st *stateTarget) getID() *string {
	if st.Relation {
		return &st.Id
	}
	return nil
}

func (s *Services) CheckIssueBeforeMigrate(srcIssue dao.Issue, targetProject dao.Project) (IssueCheckResult, error) {
	res := IssueCheckResult{
		SrcIssue:      srcIssue,
		TargetProject: targetProject,
		TargetId:      dao.GenUUID(),
	}

	// Check memberships
	{
		if _, exist := dao.IsProjectMember(s.db, srcIssue.CreatedById, targetProject.ID); !exist {
			res.Errors = append(res.Errors, ErrClause{
				Error:           ErrAuthorNotAProjectMember,
				SrcIssueId:      &srcIssue.ID,
				IssueSequenceId: srcIssue.SequenceId,
				Type:            "user",
				Entities:        []string{srcIssue.CreatedById},
			})
		}

		assigneeErr := ErrClause{
			Error:           ErrAssigneesNotFound,
			Type:            "user",
			SrcIssueId:      &srcIssue.ID,
			IssueSequenceId: srcIssue.SequenceId,
		}

		assigneeRoleErr := ErrClause{
			Error:           ErrAssigneeRoleInvalid,
			Type:            "user",
			SrcIssueId:      &srcIssue.ID,
			IssueSequenceId: srcIssue.SequenceId,
		}

		for _, assigneeId := range srcIssue.AssigneeIDs {
			role, exist := dao.IsProjectMember(s.db, assigneeId, targetProject.ID)
			if !exist {
				assigneeErr.Entities = append(assigneeErr.Entities, assigneeId)
			}
			if role < types.MemberRole {
				assigneeRoleErr.Entities = append(assigneeRoleErr.Entities, assigneeId)
			}
		}

		if len(assigneeErr.Entities) > 0 {
			res.Errors = append(res.Errors, assigneeErr)
		}

		if len(assigneeRoleErr.Entities) > 0 {
			res.Errors = append(res.Errors, assigneeRoleErr)
		}

		watcherErr := ErrClause{
			Error:           ErrWatchersNotFound,
			Type:            "user",
			SrcIssueId:      &srcIssue.ID,
			IssueSequenceId: srcIssue.SequenceId,
		}
		watcherRoleErr := ErrClause{
			Error:           ErrWatcherRoleInvalid,
			Type:            "user",
			SrcIssueId:      &srcIssue.ID,
			IssueSequenceId: srcIssue.SequenceId,
		}
		for _, watcherId := range srcIssue.WatcherIDs {
			role, exist := dao.IsProjectMember(s.db, watcherId, targetProject.ID)
			if !exist {
				watcherErr.Entities = append(watcherErr.Entities, watcherId)
			}
			if role < types.GuestRole {
				watcherRoleErr.Entities = append(watcherRoleErr.Entities, watcherId)
			}
		}
		if len(watcherErr.Entities) > 0 {
			res.Errors = append(res.Errors, watcherErr)
		}

		if len(watcherRoleErr.Entities) > 0 {
			res.Errors = append(res.Errors, watcherRoleErr)
		}
	}

	// Check state
	{
		if err := s.db.Where("project_id = ?", targetProject.ID).
			Where("name = ?", srcIssue.State.Name).
			Where("\"group\" = ?", srcIssue.State.Group).
			Where("color = ?", srcIssue.State.Color).
			First(&res.TargetState).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				res.Errors = append(res.Errors, ErrClause{
					Error:           ErrStateNotFound,
					SrcIssueId:      &srcIssue.ID,
					IssueSequenceId: srcIssue.SequenceId,
					Type:            "state",
					Entities:        []string{srcIssue.State.ID},
				})
			} else {
				return res, err
			}
		}
	}

	// Check labels
	{
		labelsError := ErrClause{
			Error:           ErrLabelsNotFound,
			Type:            "label",
			SrcIssueId:      &srcIssue.ID,
			IssueSequenceId: srcIssue.SequenceId,
		}
		res.MapLabelIds = make(map[string]string, len(*srcIssue.Labels))

		for _, label := range *srcIssue.Labels {
			var targetLabel dao.Label
			if err := s.db.Where("name = ?", label.Name).
				Where("color = ?", label.Color).
				Where("project_id = ?", targetProject.ID).
				First(&targetLabel).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					labelsError.Entities = append(labelsError.Entities, label.ID)
				} else {
					return res, err
				}
			}
			res.MapLabelIds[label.ID] = targetLabel.ID
			res.TargetLabels = append(res.TargetLabels, targetLabel)
		}
		if len(labelsError.Entities) > 0 {
			res.Errors = append(res.Errors, labelsError)
		}
	}
	return res, nil
}

func migrateIssueMove(issue IssueCheckResult, user dao.User, tx *gorm.DB, idsMap map[uuid.UUID]uuid.UUID, single bool) error {
	srcIssue := issue.SrcIssue
	var familyIds []string
	for key := range idsMap {
		familyIds = append(familyIds, key.String())
	}

	if issue.Migrate {
		return nil
	}

	// Add assignees
	if len(srcIssue.AssigneeIDs) > 0 {
		if err := tx.Model(&dao.IssueAssignee{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}
	if len(srcIssue.AssigneeIDs) == 0 && len(issue.TargetProject.DefaultAssignees) > 0 {
		var newAssignees []dao.IssueAssignee
		for _, assignee := range issue.TargetProject.DefaultAssignees {
			newAssignees = append(newAssignees, dao.IssueAssignee{
				Id:          dao.GenID(),
				AssigneeId:  fmt.Sprint(assignee),
				IssueId:     srcIssue.ID.String(),
				ProjectId:   issue.TargetProject.ID,
				WorkspaceId: srcIssue.WorkspaceId,
				CreatedById: &user.ID,
				UpdatedById: &user.ID,
			})
		}
		if err := tx.CreateInBatches(&newAssignees, 10).Error; err != nil {
			return err
		}
	}

	// Add watchers
	if len(srcIssue.WatcherIDs) > 0 {
		if err := tx.Model(&dao.IssueWatcher{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	if len(srcIssue.WatcherIDs) == 0 && len(issue.TargetProject.DefaultWatchers) > 0 {
		var newWatchers []dao.IssueWatcher
		for _, watcher := range issue.TargetProject.DefaultWatchers {
			newWatchers = append(newWatchers, dao.IssueWatcher{
				Id:          dao.GenID(),
				WatcherId:   fmt.Sprint(watcher),
				IssueId:     srcIssue.ID.String(),
				ProjectId:   issue.TargetProject.ID,
				WorkspaceId: srcIssue.WorkspaceId,
				CreatedById: &user.ID,
				UpdatedById: &user.ID,
			})
		}
		if err := tx.CreateInBatches(&newWatchers, 10).Error; err != nil {
			return err
		}
	}

	// Labels
	{
		for srcLabelId, targetLabelId := range issue.MapLabelIds {
			if targetLabelId == "" {
				uuidLabel, err := uuid.FromString(srcLabelId)
				if err != nil {
					return err
				}
				v, ok := idsMap[uuidLabel]
				if !ok {
					return err
				}
				targetLabelId = v.String()
			}
			if err := tx.Model(&dao.IssueLabel{}).
				Where("issue_id = ? and label_id = ?", srcIssue.ID, srcLabelId).
				Update("project_id", issue.TargetProject.ID).
				Update("label_id", targetLabelId).Error; err != nil {
				return err
			}
		}

	}

	// Comments
	{
		if err := tx.Model(&dao.IssueComment{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	// Attachments
	{
		if err := tx.Model(&dao.IssueAttachment{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	// Links
	{
		if err := tx.Model(&dao.IssueLink{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	// Activities
	{
		if err := tx.Model(&dao.IssueActivity{}).
			Where("issue_id = ?", srcIssue.ID).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	// Blockers
	{
		if err := tx.Model(&dao.IssueBlocker{}).
			Where("blocked_by_id = ? AND block_id in (?)", srcIssue.ID, familyIds).
			Update("project_id", issue.TargetProject.ID).Error; err != nil {
			return err
		}
	}

	// Rules-log
	{
		if err := tx.Where("issue_id = ?", issue.SrcIssue.ID.String()).Delete(&dao.RulesLog{}).Error; err != nil {
			return err
		}
	}
	issue.Migrate = true
	return nil
}

func migrateIssueCopy(issue IssueCheckResult, user dao.User, tx *gorm.DB, idsMap map[uuid.UUID]uuid.UUID, idsCommentMap map[uuid.UUID]uuid.UUID, single bool) error {
	srcIssue := issue.SrcIssue

	if issue.Migrate {
		return nil
	}

	targetIssue := issue.SrcIssue
	targetIssue.ID = issue.TargetId
	targetIssue.ProjectId = issue.TargetProject.ID
	if issue.TargetState.ID == "" {
		ii, _ := uuid.FromString(*issue.SrcIssue.StateId)
		if err := tx.Where("workspace_id = ?", issue.SrcIssue.WorkspaceId).
			Where("project_id = ?", issue.TargetProject.ID).
			Where("id = ?", idsMap[ii]).
			First(&issue.TargetState).Error; err != nil {
			return err
		}
	}
	targetIssue.StateId = &issue.TargetState.ID
	targetIssue.State = &issue.TargetState

	if targetIssue.ParentId.Valid {
		targetIssue.ParentId = uuid.NullUUID{UUID: idsMap[srcIssue.Parent.ID], Valid: true}
	}

	if single {
		targetIssue.ParentId = uuid.NullUUID{}
	}

	tx.Exec("SET session_replication_role = 'replica'")
	if err := dao.CreateIssue(tx, &targetIssue); err != nil {
		return err
	}
	tx.Exec("SET session_replication_role = 'origin'")

	// Add assignees
	var assigneesList []string
	if len(srcIssue.AssigneeIDs) > 0 {
		assigneesList = srcIssue.AssigneeIDs
	} else if len(issue.TargetProject.DefaultAssignees) > 0 {
		assigneesList = issue.TargetProject.DefaultAssignees
	}
	if len(assigneesList) > 0 {
		var newAssignees []dao.IssueAssignee
		for _, assignee := range assigneesList {
			newAssignees = append(newAssignees, dao.IssueAssignee{
				Id:          dao.GenID(),
				AssigneeId:  fmt.Sprint(assignee),
				IssueId:     targetIssue.ID.String(),
				ProjectId:   issue.TargetProject.ID,
				WorkspaceId: srcIssue.WorkspaceId,
				CreatedById: &user.ID,
				UpdatedById: &user.ID,
			})
		}
		if err := tx.CreateInBatches(&newAssignees, 10).Error; err != nil {
			return err
		}
	}

	// Add watchers
	var watchersList []string
	if len(srcIssue.WatcherIDs) > 0 {
		watchersList = srcIssue.WatcherIDs
	} else if len(issue.TargetProject.DefaultWatchers) > 0 {
		watchersList = issue.TargetProject.DefaultWatchers
	}
	if len(watchersList) > 0 {
		var newWatchers []dao.IssueWatcher
		for _, watcher := range watchersList {
			newWatchers = append(newWatchers, dao.IssueWatcher{
				Id:          dao.GenID(),
				WatcherId:   fmt.Sprint(watcher),
				IssueId:     targetIssue.ID.String(),
				ProjectId:   issue.TargetProject.ID,
				WorkspaceId: srcIssue.WorkspaceId,
				CreatedById: &user.ID,
				UpdatedById: &user.ID,
			})
		}
		if err := tx.CreateInBatches(&newWatchers, 10).Error; err != nil {
			return err
		}
	}

	// Labels
	{
		var newLabels []dao.IssueLabel
		for _, label := range issue.TargetLabels {
			newLabels = append(newLabels, dao.IssueLabel{
				Id:          dao.GenID(),
				LabelId:     label.ID,
				IssueId:     targetIssue.ID.String(),
				ProjectId:   issue.TargetProject.ID,
				WorkspaceId: targetIssue.WorkspaceId,
				CreatedById: &user.ID,
				UpdatedById: &user.ID,
			})
		}

		for k, v := range issue.MapLabelIds {
			if v == "" {
				uuidLabel, err := uuid.FromString(k)
				if err != nil {
					return err
				}
				v, ok := idsMap[uuidLabel]
				if !ok {
					return err
				}
				newLabels = append(newLabels, dao.IssueLabel{
					Id:          dao.GenID(),
					LabelId:     v.String(),
					IssueId:     targetIssue.ID.String(),
					ProjectId:   issue.TargetProject.ID,
					WorkspaceId: targetIssue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
		}
		if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
			return err
		}
	}

	// Comments, reactions
	{
		var comments []dao.IssueComment
		var commentIds []uuid.UUID

		if err := tx.Where("issue_id = ?", srcIssue.ID).Find(&comments).Error; err != nil {
			return err
		}

		for i := range comments {
			commentIds = append(commentIds, comments[i].Id)
			comments[i].Id = idsCommentMap[comments[i].Id]
			comments[i].IssueId = targetIssue.ID.String()
			comments[i].ProjectId = issue.TargetProject.ID
			if comments[i].ReplyToCommentId.Valid {
				replyId := idsCommentMap[comments[i].ReplyToCommentId.UUID]
				comments[i].ReplyToCommentId = uuid.NullUUID{UUID: replyId, Valid: true}
			}
		}

		if err := tx.CreateInBatches(&comments, 10).Error; err != nil {
			return err
		}

		var reactions []dao.CommentReaction
		if err := tx.Where("comment_id in ?", commentIds).Find(&reactions).Error; err != nil {
			return err
		}

		for i := range reactions {
			reactions[i].Id = dao.GenID()
			reactions[i].CommentId = idsCommentMap[reactions[i].CommentId]
		}

		if err := tx.CreateInBatches(&reactions, 10).Error; err != nil {
			return err
		}
	}

	// Attachments
	{
		var attachments []dao.IssueAttachment
		if err := tx.Where("issue_id = ?", srcIssue.ID).Find(&attachments).Error; err != nil {
			return err
		}

		for i := range attachments {
			attachments[i].Id = dao.GenID()
			attachments[i].IssueId = targetIssue.ID.String()
			attachments[i].ProjectId = issue.TargetProject.ID
		}

		if err := tx.CreateInBatches(&attachments, 10).Error; err != nil {
			return err
		}
	}

	// Links
	{
		var links []dao.IssueLink
		if err := tx.Where("issue_id = ?", srcIssue.ID).Find(&links).Error; err != nil {
			return err
		}

		for i := range links {
			links[i].Id = dao.GenID()
			links[i].IssueId = targetIssue.ID.String()
			links[i].ProjectId = issue.TargetProject.ID
		}

		if err := tx.CreateInBatches(&links, 10).Error; err != nil {
			return err
		}
	}

	// blockers
	{
		var blocker, newBlockers []dao.IssueBlocker
		if err := tx.Where("blocked_by_id = ?", srcIssue.ID).Find(&blocker).Error; err != nil {
			return err
		}

		for i, block := range blocker {
			if _, ok := idsMap[block.BlockId]; !ok {
				continue
			}
			blocker[i].Id = dao.GenID()
			blocker[i].BlockId = idsMap[block.BlockId]
			blocker[i].BlockedById = idsMap[block.BlockedById]
			blocker[i].ProjectId = issue.TargetProject.ID
			blocker[i].WorkspaceId = issue.TargetProject.WorkspaceId

			newBlockers = append(newBlockers, blocker[i])
		}

		tx.Exec("SET session_replication_role = 'replica'")
		if err := tx.CreateInBatches(&newBlockers, 10).Error; err != nil {
			return err
		}
		tx.Exec("SET session_replication_role = 'origin'")
	}

	issue.Migrate = true
	issue.SrcIssue = targetIssue

	return nil
}

func stateRelation(tx *gorm.DB, srcProject, targetProject string) (error, map[string]stateTarget) {
	var srcStates, targetStates []dao.State
	if err := tx.Where("project_id = ?", srcProject).
		Find(&srcStates).Error; err != nil {
		return err, nil
	}
	if err := tx.Where("project_id = ?", targetProject).
		Find(&targetStates).Error; err != nil {
		return err, nil
	}
	result := make(map[string]stateTarget)
	strState := func(state dao.State) string {
		return fmt.Sprintf("%s-%s-%s", state.Name, state.Group, state.Color)
	}

	for _, state := range srcStates {
		tmp := stateTarget{
			Str:      strState(state),
			Id:       "",
			Relation: false,
		}
		for _, target := range targetStates {
			if tmp.Str == strState(target) {
				tmp.Id = target.ID
				tmp.Relation = true
			}
		}
		result[state.ID] = tmp
	}

	return nil, result
}

func stateActivityUpdate(tx *gorm.DB, ids []string, srcProjectId, targetProjectId string) error {
	{
		var activityState []dao.EntityActivity
		if err := tx.
			Where("issue_id IN ?", ids).
			Where("field = ?", "state").
			Find(&activityState).Error; err != nil {
			return err
		}

		err, stateMap := stateRelation(tx, srcProjectId, targetProjectId)
		if err != nil {
			return err
		}
		for i, activity := range activityState {
			if activity.OldIdentifier != nil {
				oldState := stateMap[*activity.OldIdentifier]
				activityState[i].OldIdentifier = oldState.getID()
			} else {
				activityState[i].OldIdentifier = nil
			}
			if activity.NewIdentifier != nil {
				newState := stateMap[*activity.NewIdentifier]
				activityState[i].NewIdentifier = newState.getID()
			} else {
				activityState[i].NewIdentifier = nil
			}
		}

		if len(activityState) == 0 {
			return nil
		}

		if err := tx.Save(&activityState).Error; err != nil {
			return err
		}
	}
	return nil
}

func linkedIdToStringKey(s1, s2 string) string {
	if s1 < s2 {
		return fmt.Sprintf("%s %s", s1, s2)
	} else if s1 > s2 {
		return fmt.Sprintf("%s %s", s2, s1)
	} else {
		return ""
	}
}

// NewIssueParam изменяемы поля при копировании одиночной задачи
type NewIssueParam struct {
	Priority     types.JSONField[string]   `json:"priority,omitempty" extensions:"x-nullable" swaggertype:"string" enums:"urgent,high,medium,low"`
	TargetDate   types.JSONField[string]   `json:"target_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
	AssignersIds types.JSONField[[]string] `json:"assigner_ids,omitempty" extensions:"x-nullable" swaggertype:"array,string"`
	StateId      types.JSONField[string]   `json:"state_id,omitempty" extensions:"x-nullable" swaggertype:"string"`
}
