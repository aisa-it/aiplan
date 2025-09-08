package aiplan

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/apierrors"
	"sheff.online/aiplan/internal/aiplan/dao"
	filestorage "sheff.online/aiplan/internal/aiplan/file-storage"
)

func (s *Services) getSwaggerJSON(c echo.Context) error {
	f, err := os.Open("docs/swagger.json")
	if err != nil {
		return EError(c, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	data := make(map[string]interface{}, 0)
	if err := dec.Decode(&data); err != nil {
		return EError(c, err)
	}
	data["host"] = cfg.WebURL
	return c.JSON(http.StatusOK, data)
}

func (s *Services) uploadAssetForm(tx *gorm.DB, file *multipart.FileHeader, dstAsset *dao.FileAsset, metadata filestorage.Metadata) error {
	assetSrc, err := file.Open()
	if err != nil {
		return err
	}
	defer assetSrc.Close()

	if dstAsset.Id.IsNil() {
		dstAsset.Id = dao.GenUUID()
	}

	dstAsset.Name = file.Filename
	dstAsset.FileSize = int(file.Size)
	dstAsset.ContentType = file.Header.Get("Content-Type")

	if err := s.storage.SaveReader(
		assetSrc,
		file.Size,
		dstAsset.Id,
		dstAsset.ContentType,
		&metadata,
	); err != nil {
		return err
	}

	return tx.Create(&dstAsset).Error
}

func (s *Services) uploadAvatarForm(tx *gorm.DB, file *multipart.FileHeader, dstAsset *dao.FileAsset) error {
	assetSrc, err := file.Open()
	if err != nil {
		return err
	}
	defer assetSrc.Close()

	if dstAsset.Id.IsNil() {
		dstAsset.Id = dao.GenUUID()
	}

	dataType := file.Header.Get("Content-Type")

	dstAsset.Name = file.Filename
	dstAsset.FileSize = int(file.Size)
	dstAsset.ContentType = dataType

	dataSize := 0
	var data io.Reader

	switch dataType {
	case "image/gif", "image/jpeg", "image/png":
		data, dataSize, dataType, err = imageThumbnail(assetSrc, dataType)
		if err != nil {
			return err
		}
	default:
		return apierrors.ErrUnsupportedAvatarType
	}

	if err := s.storage.SaveReader(
		data,
		int64(dataSize),
		dstAsset.Id,
		dataType,
		&filestorage.Metadata{},
	); err != nil {
		return err
	}

	return tx.Create(&dstAsset).Error
}

func activityMigrate(db *gorm.DB) {
	var oldAct []dao.EntityActivity
	db.FindInBatches(&oldAct, 100, func(tx *gorm.DB, batch int) error {
		var ids []string
		var idsForNotify []string
		var issueAct []dao.IssueActivity
		var projectAct []dao.ProjectActivity
		var formAct []dao.FormActivity
		var workspaceAct []dao.WorkspaceActivity
		var rootAct []dao.RootActivity

		//var formAct []dao.FormActivity
		for _, activity := range oldAct {
			switch activity.EntityType {
			case "issue":
				if activity.Field != nil && activity.IssueId != nil {
					is := dao.IssueActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          "",                // в зависимости от нового поведения
						Field:         activity.Field,    // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						IssueId:       *activity.IssueId,
						ProjectId:     *activity.ProjectId,
						WorkspaceId:   activity.WorkspaceId,
						ActorId:       activity.ActorId,
						NewIdentifier: activity.NewIdentifier, // в зависимости от нового поведения
						OldIdentifier: activity.OldIdentifier, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}

					switch *activity.Field {
					case "priority", "state", "target_date", "name", "description", "blocks", "blocking", "start_date", "completed_at":
						is.Verb = "updated"
					case "assignees", "watchers", "labels", "parent", "sub_issue", "linked":
						if is.NewIdentifier == nil {
							is.Verb = "removed"
						} else {
							is.Verb = "added"
						}
					case "attachment", "link", "comment":
						var id string
						if activity.NewIdentifier != nil && activity.OldIdentifier != nil {
							is.Verb = "updated"
						} else if activity.NewIdentifier != nil {
							id = *activity.NewIdentifier
							is.Verb = "created"
						} else {
							if activity.OldIdentifier != nil {
								id = *activity.OldIdentifier
							}
							is.Verb = "deleted"
						}

						if *activity.Field == "attachment" {
							var issueAttachment dao.IssueAttachment
							if err := tx.Where("id = ?", id).First(&issueAttachment).Error; err != nil {
								if gorm.ErrRecordNotFound == err {
									is.OldIdentifier = nil
									is.NewIdentifier = nil
								} else {
									continue
								}
								if v, ok := issueAttachment.Attributes["name"]; ok {
									is.NewValue = v.(string)
								}
							}
						}

						if *activity.Field == "comment" {
							if is.Verb == "updated" {
								tmp := "comment"
								is.Field = &tmp
							}

							var issueComment dao.IssueComment
							if err := tx.Where("id = ?", id).First(&issueComment).Error; err != nil {
								if gorm.ErrRecordNotFound == err {
									is.OldIdentifier = nil
									is.NewIdentifier = nil
								} else {
									continue
								}
							}
						}
					case "issue_transfer":
						idsForNotify = append(idsForNotify, activity.Id)
						ids = append(ids, activity.Id)
						continue
					default:
						continue
					}
					issueAct = append(issueAct, is)
					ids = append(ids, activity.Id)

				} else {
					if activity.Verb == "deleted" {
						ids = append(ids, activity.Id)
						continue
					} else {
						field := "issue"
						var issue dao.Issue
						if err := tx.Preload("Project").
							Where("id = ?", activity.IssueId).
							First(&issue).Error; err != nil {
							continue
						}

						pa := dao.ProjectActivity{
							Id:            activity.Id,
							CreatedAt:     activity.CreatedAt,
							Verb:          "created",         // в зависимости от нового поведения
							Field:         &field,            // в зависимости от нового поведения
							OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
							NewValue:      issue.String(),
							Comment:       activity.Comment,
							ProjectId:     *activity.ProjectId,
							WorkspaceId:   activity.WorkspaceId,
							ActorId:       activity.ActorId,
							NewIdentifier: activity.IssueId, // в зависимости от нового поведения
							OldIdentifier: nil,              // в зависимости от нового поведения
							Notified:      activity.Notified,
							TelegramMsgId: activity.TelegramMsgId,
						}
						projectAct = append(projectAct, pa)
						ids = append(ids, activity.Id)
					}
				}

			case "project":
				if activity.Field != nil && *activity.Field != "" {
					pa := dao.ProjectActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          "",                // в зависимости от нового поведения
						Field:         activity.Field,    // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						ProjectId:     *activity.ProjectId,
						WorkspaceId:   activity.WorkspaceId,
						ActorId:       activity.ActorId,
						NewIdentifier: activity.NewIdentifier, // в зависимости от нового поведения
						OldIdentifier: activity.OldIdentifier, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}
					switch *activity.Field {
					case "name", "emoji", "identifier", "public", "role", "default_assignees", "default_watchers", "project_lead", "default":
						pa.Verb = "updated"
					case "state", "label":
						var id string
						if activity.NewIdentifier != nil && activity.OldIdentifier != nil {
							pa.Verb = "updated"
						} else if activity.NewIdentifier != nil {
							id = *activity.NewIdentifier
							pa.Verb = "created"
						} else {
							if activity.OldIdentifier != nil {
								id = *activity.OldIdentifier
							}
							pa.Verb = "deleted"
						}

						if *activity.Field == "state" {
							var state dao.State
							if err := tx.Where("id = ?", id).First(&state).Error; err != nil {
								if gorm.ErrRecordNotFound == err {
									pa.OldIdentifier = nil
									pa.NewIdentifier = nil
								} else {
									continue
								}
							}
						}

						if *activity.Field == "label" {
							var label dao.Label
							if err := tx.Where("id = ?", id).First(&label).Error; err != nil {
								if gorm.ErrRecordNotFound == err {
									pa.OldIdentifier = nil
									pa.NewIdentifier = nil
								} else {
									continue
								}
							}
						}

					case "status_name", "status_description", "status_color", "status_group", "label_name", "label_color":
						ids = append(ids, activity.Id)
						continue
					case "member":
						var id string
						if activity.NewIdentifier != nil {
							id = *activity.NewIdentifier
						}
						if activity.OldIdentifier != nil {
							id = *activity.OldIdentifier
						}
						var user dao.User
						if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
							continue
						}
						userStr := user.GetString()

						if activity.Verb == "added" {
							pa.Verb = "created"
							pa.NewValue = userStr
						} else if activity.Verb == "deleted" {
							pa.Verb = "deleted"
							pa.OldValue = &userStr
						}
					default:
						continue
					}
					projectAct = append(projectAct, pa)
					ids = append(ids, activity.Id)

				} else {
					field := "project"
					wa := dao.WorkspaceActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          activity.Verb,     // в зависимости от нового поведения
						Field:         &field,            // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						WorkspaceId:   activity.WorkspaceId,
						ActorId:       activity.ActorId,
						NewIdentifier: nil, // в зависимости от нового поведения
						OldIdentifier: nil, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}

					if wa.Verb == "created" {
						wa.NewIdentifier = activity.ProjectId
					}

					var project dao.Project
					if err := tx.Preload("Workspace").
						Where("id = ?", activity.ProjectId).
						First(&project).Error; err != nil {
						if gorm.ErrRecordNotFound == err {
							wa.OldIdentifier = nil
							wa.NewIdentifier = nil
						} else {
							continue
						}
					}
					workspaceAct = append(workspaceAct, wa)
					ids = append(ids, activity.Id)
				}
			case "workspace":
				if activity.Field != nil && *activity.Field != "" {
					wa := dao.WorkspaceActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          "",                // в зависимости от нового поведения
						Field:         activity.Field,    // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						WorkspaceId:   activity.WorkspaceId,
						ActorId:       activity.ActorId,
						NewIdentifier: activity.NewIdentifier, // в зависимости от нового поведения
						OldIdentifier: activity.OldIdentifier, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}
					switch *activity.Field {
					case "role", "name", "logo", "integration_token":
						wa.Verb = "updated"
					case "member":
						if activity.NewIdentifier == nil {
							wa.Verb = "deleted"
						}
						if activity.OldIdentifier == nil {
							wa.Verb = "created"
						}

					default:
						continue
					}
					workspaceAct = append(workspaceAct, wa)
					ids = append(ids, activity.Id)
				} else {
					field := "workspace"
					ra := dao.RootActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          activity.Verb,     // в зависимости от нового поведения
						Field:         &field,            // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						ActorId:       activity.ActorId,
						NewIdentifier: nil, // в зависимости от нового поведения
						OldIdentifier: nil, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}

					if ra.Verb == "created" {
						ra.NewIdentifier = &activity.WorkspaceId
					}

					var workspace dao.Workspace
					if err := tx.
						Where("id = ?", activity.WorkspaceId).
						First(&workspace).Error; err != nil {
						if gorm.ErrRecordNotFound == err {
							ra.OldIdentifier = nil
							ra.NewIdentifier = nil
						} else {
							continue
						}
					}
					rootAct = append(rootAct, ra)
					ids = append(ids, activity.Id)
				}

			case "form":
				if activity.Field != nil && *activity.Field != "" {
					fa := dao.FormActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          "",                // в зависимости от нового поведения
						Field:         activity.Field,    // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						WorkspaceId:   activity.WorkspaceId,
						FormId:        *activity.FormId,
						ActorId:       activity.ActorId,
						NewIdentifier: activity.NewIdentifier, // в зависимости от нового поведения
						OldIdentifier: activity.OldIdentifier, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId}
					switch *activity.Field {
					case "fields", "end_date", "title", "description", "auth_require":
						if *activity.Field == "description" {
							fa.NewIdentifier = nil
						}
						fa.Verb = "updated"
					case "answer":
						fa.Verb = "created"

					default:
						continue
					}
					formAct = append(formAct, fa)
					ids = append(ids, activity.Id)

				} else {
					//TODO - создание форм
					field := "form"
					wa := dao.WorkspaceActivity{
						Id:            activity.Id,
						CreatedAt:     activity.CreatedAt,
						Verb:          activity.Verb,     // в зависимости от нового поведения
						Field:         &field,            // в зависимости от нового поведения
						OldValue:      activity.OldValue, //TODO убрать все <nil> & в зависимости от нового поведения
						NewValue:      activity.NewValue,
						Comment:       activity.Comment,
						WorkspaceId:   activity.WorkspaceId,
						ActorId:       activity.ActorId,
						NewIdentifier: nil, // в зависимости от нового поведения
						OldIdentifier: nil, // в зависимости от нового поведения
						Notified:      activity.Notified,
						TelegramMsgId: activity.TelegramMsgId,
					}

					if wa.Verb == "created" {
						wa.NewIdentifier = activity.FormId
					}

					var form dao.Form
					if err := tx.Preload("Workspace").
						Where("id = ?", activity.FormId).
						First(&form).Error; err != nil {
						if gorm.ErrRecordNotFound == err {
							wa.OldIdentifier = nil
							wa.NewIdentifier = nil
						} else {
							continue
						}
					}
					workspaceAct = append(workspaceAct, wa)
					ids = append(ids, activity.Id)

				}
			}
		}

		if err := db.Transaction(func(tx *gorm.DB) error {
			if len(issueAct) > 0 {
				if err := tx.Create(&issueAct).Error; err != nil {
					return err
				}

				for _, activity := range issueAct {
					if err := tx.Model(&dao.UserNotifications{}).
						Where("entity_activity_id = ?", activity.Id).
						Updates(map[string]interface{}{
							"issue_activity_id":  activity.Id,
							"entity_activity_id": nil,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(projectAct) > 0 {
				if err := tx.Create(&projectAct).Error; err != nil {
					return err
				}

				for _, activity := range projectAct {
					if err := tx.Model(&dao.UserNotifications{}).
						Where("entity_activity_id = ?", activity.Id).
						Updates(map[string]interface{}{
							"project_activity_id": activity.Id,
							"entity_activity_id":  nil,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(formAct) > 0 {
				if err := tx.Create(&formAct).Error; err != nil {
					return err
				}

				for _, activity := range formAct {
					if err := tx.Model(&dao.UserNotifications{}).
						Where("entity_activity_id = ?", activity.Id).
						Updates(map[string]interface{}{
							"form_activity_id":   activity.Id,
							"entity_activity_id": nil,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(workspaceAct) > 0 {
				if err := tx.Create(&workspaceAct).Error; err != nil {
					return err
				}

				for _, activity := range workspaceAct {
					if err := tx.Model(&dao.UserNotifications{}).
						Where("entity_activity_id = ?", activity.Id).
						Updates(map[string]interface{}{
							"workspace_activity_id": activity.Id,
							"entity_activity_id":    nil,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(rootAct) > 0 {
				if err := tx.Create(&rootAct).Error; err != nil {
					return err
				}

				for _, activity := range rootAct {
					if err := tx.Model(&dao.UserNotifications{}).
						Where("entity_activity_id = ?", activity.Id).
						Updates(map[string]interface{}{
							"root_activity_id":   activity.Id,
							"entity_activity_id": nil,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(idsForNotify) > 0 {
				if err := tx.Where("entity_activity_id in (?)", idsForNotify).Unscoped().Delete(&dao.UserNotifications{}).Error; err != nil {
					return err
				}

			}

			if len(ids) > 0 {
				if err := tx.Where("id in (?)", ids).Delete(&dao.EntityActivity{}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			fmt.Println("ERRR", err)
			return err
		}
		format := "%-30s %6d activities"
		slog.Info(fmt.Sprintf(format, "from entityActivity:", len(ids)))
		slog.Info(fmt.Sprintf(format, " - to issueActivity:", len(issueAct)))
		slog.Info(fmt.Sprintf(format, " - to projectActivity:", len(projectAct)))
		slog.Info(fmt.Sprintf(format, " - to formActivity:", len(formAct)))
		slog.Info(fmt.Sprintf(format, " - to workspaceActivity:", len(workspaceAct)))
		slog.Info(fmt.Sprintf(format, " - to rootActivity:", len(rootAct)))

		return nil
	})
}
