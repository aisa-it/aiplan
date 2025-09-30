package business

import (
	"gorm.io/gorm"
)

func (b *Business) ReplaceUser(origUserId string, newUserId string) error {
	return b.db.Transaction(func(tx *gorm.DB) error {
		for _, expr := range userChangeExprs {
			if err := tx.Exec(expr, newUserId, origUserId).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UPDATE THIS LIST IF ADD NEW TABLE WITH FK TO USER ID
var (
	userChangeExprs = []string{
		"update docs set created_by_id=? where created_by_id=?",
		"update doc_comments set actor_id=? where actor_id=?",
		"update projects set project_lead_id=? where project_lead_id=?",
		"update issues set created_by_id=? where created_by_id=?",
		"update issue_comments set actor_id=? where actor_id=?",
		"update file_assets set created_by_id=? where created_by_id=?",
		"update workspaces set owner_id=? where owner_id=?",
		"update users set created_by_id=? where created_by_id=?",
		"update users set updated_by_id=? where updated_by_id=?",
		"update comment_reactions set user_id=? where user_id=?",
		"update deferred_notifications set user_id=? where user_id=?",
		"update doc_activities set actor_id=? where actor_id=?",
		"update doc_comment_reactions set user_id=? where user_id=?",
		"update doc_editors set editor_id=? where editor_id=?",
		"update doc_readers set reader_id=? where reader_id=?",
		"update doc_watchers set watcher_id=? where watcher_id=?",
		"update forms set created_by_id=? where created_by_id=?",
		"update entity_activities set actor_id=? where actor_id=?",
		"update form_activities set actor_id=? where actor_id=?",
		"update form_answers set created_by_id=? where created_by_id=?",
		"update issue_activities set actor_id=? where actor_id=?",
		"update issue_assignees set assignee_id=? where assignee_id=?",
		"update issue_links set created_by_id=? where created_by_id=?",
		"update issue_properties set user_id=? where user_id=?",
		"update issue_templates set created_by_id=? where created_by_id=?",
		"update issue_templates set updated_by_id=? where updated_by_id=?",
		"update issue_watchers set watcher_id=? where watcher_id=?",
		"update project_activities set actor_id=? where actor_id=?",
		"update project_members set member_id=? where member_id=?",
		"update project_members set created_by_id=? where created_by_id=?",
		"update release_notes set author_id=? where author_id=?",
		"update activities set actor_id=? where actor_id=?",
		"update rules_log set user_id=? where user_id=?",
		"update search_filters set author_id=? where author_id=?",
		"update user_search_filters set user_id=? where user_id=?",
		"update sessions_resets set user_id=? where user_id=?",
		"update tariffication set user_id=? where user_id=?",
		"update team_members set member_id=? where member_id=?",
		"update user_feedbacks set user_id=? where user_id=?",
		"update workspace_activities set actor_id=? where actor_id=?",
		"update user_notifications set user_id=? where user_id=?",
		"update user_notifications set author_id=? where author_id=?",
		"update workspace_backups set created_by=? where created_by=?",
		"update workspace_members set member_id=? where member_id=?",
		"update workspace_members set created_by_id=? where created_by_id=?",
		"update issue_description_locks set user_id=? where user_id=?",
	}
)
