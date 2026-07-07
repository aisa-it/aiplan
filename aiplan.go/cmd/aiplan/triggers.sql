-- hash columns cause gorm not supported them corrected
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION immutable_array_to_string(text[], text)
    RETURNS text AS $$
SELECT array_to_string($1, $2);
$$ LANGUAGE sql IMMUTABLE STRICT;

CREATE OR REPLACE FUNCTION row_hash(VARIADIC text[])
    RETURNS bytea AS $$
BEGIN
    RETURN digest(immutable_array_to_string($1, '_'), 'sha256');
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, description, logo_id::text, slug, owner_id::text, integration_token, (deleted_at is null)::text)) STORED;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, public::text, identifier, project_lead_id::text, emoji::text, coalesce(cover_image, ''), coalesce(estimate_id, ''), coalesce(rules_script, ''), (deleted_at is null)::text)) STORED;

ALTER TABLE states DROP COLUMN IF EXISTS "hash";
ALTER TABLE states ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, description, color, "group", "default"::text, sequence::text, COALESCE(immutable_array_to_string(from_states::text[], ','), ''))) STORED;

CREATE INDEX IF NOT EXISTS "idx_workspaces_hash" ON "workspaces" USING hash("hash");
CREATE INDEX IF NOT EXISTS "idx_projects_hash" ON "projects" USING hash("hash");
CREATE INDEX IF NOT EXISTS "idx_states_hash" ON "states" USING hash("hash");

-- indexes
CREATE EXTENSION if not exists pg_trgm;
CREATE EXTENSION if not exists btree_gin;
CREATE EXTENSION if not exists btree_gist;
CREATE INDEX IF NOT EXISTS "issues_gin" ON "issues" USING gin("workspace_id","project_id","sequence_id",(sequence_id::text)) WHERE deleted_at is not null;

-- delete deprecated
DO $$
    BEGIN
        DROP TRIGGER IF EXISTS insert_or_update_issues ON issues;
        DROP TRIGGER IF EXISTS issue_name_or_desc_changes ON issues;
        DROP TRIGGER IF EXISTS issue_name_or_desc_create ON issues;
        DROP FUNCTION IF EXISTS insert_issue_vectors();
        DROP FUNCTION IF EXISTS update_issue_vectors();
    END
$$;

-- add sort_order constraint
DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint
            WHERE conname = 'unique_sort_constraint'
              AND conrelid = 'issues'::regclass
        ) THEN
            ALTER TABLE issues ADD CONSTRAINT unique_sort_constraint UNIQUE (parent_id, sort_order) DEFERRABLE INITIALLY DEFERRED;
        END IF;
    END $$;


-- Function for tsvector generation
CREATE OR REPLACE FUNCTION to_tsvector_multilang(name text, description text) RETURNS tsvector AS $$
SELECT setweight(to_tsvector('simple', name || ' ' || coalesce(description, '' )), 'A') ||
       setweight(to_tsvector('russian', name), 'B') ||
       setweight(to_tsvector('russian', coalesce(description, '' )), 'B') ||
       setweight(to_tsvector('english', name), 'C') ||
       setweight(to_tsvector('english', coalesce(description, '' )), 'C')
$$ LANGUAGE sql IMMUTABLE;

-- Function for rank calculation
CREATE OR REPLACE FUNCTION calc_rank(tokens tsvector, project_identifier text, sequence_id real, search_query text)
    RETURNS real
AS $$
SELECT coalesce(ts_rank(tokens, websearch_to_tsquery('simple', search_query)) + ts_rank(tokens, websearch_to_tsquery('russian', search_query)) + ts_rank(tokens, websearch_to_tsquery('english', search_query)), 0) +
       CASE
           WHEN CONCAT(project_identifier, '-', sequence_id) = search_query THEN 50
           ELSE 0
           END
           +
       CASE
           WHEN sequence_id::text = search_query THEN 50
           ELSE 0
           END
$$
    LANGUAGE sql STABLE;

-- Workspace name tokens
CREATE OR REPLACE FUNCTION gen_workspace_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE workspaces SET name_tokens=to_tsvector('russian', lower(name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_workspaces
    AFTER INSERT OR UPDATE
    ON workspaces
    FOR EACH ROW
EXECUTE PROCEDURE gen_workspace_name_tokens();

-- Doc triggers
CREATE OR REPLACE FUNCTION gen_doc_vectors()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.title <> OLD.title OR NEW.content <> OLD.content) THEN
        UPDATE docs SET tokens=to_tsvector_multilang(title, content) where id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_docs
    AFTER INSERT OR UPDATE
    ON docs
    FOR EACH ROW
EXECUTE PROCEDURE gen_doc_vectors();

-- Project name tokens
CREATE OR REPLACE FUNCTION gen_project_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE projects SET name_tokens=to_tsvector('russian', lower(name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_projects
    AFTER INSERT OR UPDATE
    ON projects
    FOR EACH ROW
EXECUTE PROCEDURE gen_project_name_tokens();

-- Sprint name tokens
CREATE OR REPLACE FUNCTION gen_sprint_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE sprints SET name_tokens=to_tsvector('russian', lower(name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_sprints
    AFTER INSERT OR UPDATE
    ON sprints
    FOR EACH ROW
EXECUTE PROCEDURE gen_sprint_name_tokens();

-- Issues triggers
CREATE OR REPLACE FUNCTION gen_issue_vectors()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name OR NEW.description_stripped <> OLD.description_stripped) THEN
        UPDATE issues SET tokens=to_tsvector_multilang(name, description_stripped) where id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_issues
    AFTER INSERT OR UPDATE
    ON issues
    FOR EACH ROW
EXECUTE PROCEDURE gen_issue_vectors();

-- Labels trigger
CREATE OR REPLACE FUNCTION gen_label_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE labels SET name_tokens=to_tsvector('russian', lower(NEW.name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_labels
    AFTER INSERT OR UPDATE
    ON labels
    FOR EACH ROW
EXECUTE PROCEDURE gen_label_name_tokens();

-- States trigger
CREATE OR REPLACE FUNCTION gen_state_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE states SET name_tokens=to_tsvector('russian', lower(NEW.name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_states
    AFTER INSERT OR UPDATE
    ON states
    FOR EACH ROW
EXECUTE PROCEDURE gen_state_name_tokens();

-- Search Filters trigger
CREATE OR REPLACE FUNCTION gen_search_filter_name_tokens()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.name <> OLD.name) THEN
        UPDATE search_filters SET name_tokens=to_tsvector('russian', lower(NEW.name)) WHERE id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER insert_or_update_search_filters
    AFTER INSERT OR UPDATE
    ON search_filters
    FOR EACH ROW
EXECUTE PROCEDURE gen_search_filter_name_tokens();

-- Deferred Notifications trigger for real-time notification delivery
-- Отправляет NOTIFY только для уведомлений, готовых к обработке:
-- sent_at IS NULL - ещё не отправлено
-- time_send < NOW() - время отправки наступило
-- attempt_count < 3 - не превышен лимит попыток
-- JSON включает связанные сущности: workspace, project, user, issue
CREATE OR REPLACE FUNCTION notify_deferred_notification()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
DECLARE
    payload jsonb;
BEGIN
    IF NEW.sent_at IS NULL AND NEW.time_send < NOW() AND NEW.attempt_count < 3 THEN
        SELECT json_build_object(
                       'id', NEW.id,
                       'user_id', NEW.user_id,
                       'issue_id', NEW.issue_id,
                       'project_id', NEW.project_id,
                       'workspace_id', NEW.workspace_id,
                       'notification_type', NEW.notification_type,
                       'delivery_method', NEW.delivery_method,
                       'time_send', NEW.time_send,
                       'attempt_count', NEW.attempt_count,
                       'last_attempt_at', NEW.last_attempt_at,
                       'sent_at', NEW.sent_at,
                       'notification_payload', NEW.notification_payload,
                       'user', CASE WHEN u.id IS NOT NULL THEN json_build_object(
                        'id', u.id,
                        'is_active', u.is_active,
                        'is_integration', u.is_integration,
                        'is_bot', u.is_bot,
                        'telegram_id', u.telegram_id,
                        'user_timezone', u.user_timezone,
                        'settings', u.settings,
                        'email', u.email
                                                               ) END,
                       'workspace', CASE WHEN w.id IS NOT NULL THEN json_build_object(
                        'id', w.id,
                        'name', w.name,
                        'slug', w.slug
                                                                    ) END,
                       'project', CASE WHEN p.id IS NOT NULL THEN json_build_object(
                        'id', p.id,
                        'identifier', p.identifier
                                                                  ) END,
                       'issue', CASE WHEN i.id IS NOT NULL THEN json_build_object(
                        'id', i.id,
                        'name', i.name,
                        'sequence_id', i.sequence_id,
                        'project', i.project_id,
                        'workspace', i.workspace_id,
                        'created_by', i.created_by_id
                                                                ) END
               ) INTO payload
        FROM (SELECT 1) AS dummy
                 LEFT JOIN users u ON u.id = NEW.user_id
                 LEFT JOIN workspaces w ON w.id = NEW.workspace_id
                 LEFT JOIN projects p ON p.id = NEW.project_id
                 LEFT JOIN issues i ON i.id = NEW.issue_id;

        PERFORM pg_notify('notifications', payload::text);
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER deferred_notifications_notify
    AFTER INSERT OR UPDATE
    ON deferred_notifications
    FOR EACH ROW
EXECUTE PROCEDURE notify_deferred_notification();

-- Light users cache sub
CREATE OR REPLACE FUNCTION notify_users_light_changes()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.username IS DISTINCT FROM OLD.username OR
       NEW.email IS DISTINCT FROM OLD.email OR
       NEW.first_name IS DISTINCT FROM OLD.first_name OR
       NEW.last_name IS DISTINCT FROM OLD.last_name OR
       NEW.avatar_id IS DISTINCT FROM OLD.avatar_id OR
       NEW.user_timezone IS DISTINCT FROM OLD.user_timezone OR
       NEW.telegram_id IS DISTINCT FROM OLD.telegram_id OR
       NEW.status_emoji IS DISTINCT FROM OLD.status_emoji OR
       NEW.status IS DISTINCT FROM OLD.status OR
       NEW.status_end_date IS DISTINCT FROM OLD.status_end_date OR
       NEW.created_at IS DISTINCT FROM OLD.created_at OR
       NEW.is_superuser IS DISTINCT FROM OLD.is_superuser OR
       NEW.is_active IS DISTINCT FROM OLD.is_active OR
       NEW.blocked_until IS DISTINCT FROM OLD.blocked_until OR
       NEW.is_onboarded IS DISTINCT FROM OLD.is_onboarded OR
       NEW.is_bot IS DISTINCT FROM OLD.is_bot OR
       NEW.is_integration IS DISTINCT FROM OLD.is_integration THEN

        PERFORM pg_notify(
                'users_light_changes',
                json_build_object(
                        'id', NEW.id,
                        'username', NEW.username,
                        'email', NEW.email,
                        'first_name', NEW.first_name,
                        'last_name', NEW.last_name,
                        'avatar_id', NEW.avatar_id,
                        'user_timezone', NEW.user_timezone,
                        'telegram_id', NEW.telegram_id,
                        'status_emoji', NEW.status_emoji,
                        'status', NEW.status,
                        'status_end_date', NEW.status_end_date,
                        'created_at', NEW.created_at,
                        'is_superuser', NEW.is_superuser,
                        'is_active', NEW.is_active,
                        'blocked_until', NEW.blocked_until,
                        'is_onboarded', NEW.is_onboarded,
                        'is_bot', NEW.is_bot,
                        'is_integration', NEW.is_integration
                )::text
                );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Триггер только при обновлении конкретных полей
CREATE OR REPLACE TRIGGER users_light_notify
    AFTER UPDATE OF username,
        email,
        first_name,
        last_name,
        avatar_id,
        user_timezone,
        telegram_id,
        status_emoji,
        status,
        status_end_date,
        created_at,
        is_superuser,
        is_active,
        blocked_until,
        is_onboarded,
        is_bot,
        is_integration ON users
    FOR EACH ROW
EXECUTE FUNCTION notify_users_light_changes();


ALTER TABLE activity_events DROP CONSTRAINT IF EXISTS entity_fk_check;

ALTER TABLE activity_events
    ADD CONSTRAINT entity_fk_check
        CHECK (
            (entity_type = 0 AND
             workspace_id IS NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS NULL AND
             sprint_id IS NULL AND
             form_id IS NULL)
                OR
            (entity_type = 1 AND
             workspace_id IS NOT NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS NULL AND
             sprint_id IS NULL AND
             form_id IS NULL)
                OR
            (entity_type = 2 AND
             workspace_id IS NOT NULL AND
             project_id IS NOT NULL AND
             issue_id IS NULL AND
             doc_id IS NULL AND
             sprint_id IS NULL AND
             form_id IS NULL)
                OR
            (entity_type = 3 AND
             workspace_id IS NOT NULL AND
             project_id IS NOT NULL AND
             issue_id IS NOT NULL AND
             doc_id IS NULL AND
             sprint_id IS NULL AND
             form_id IS NULL)
                OR
            (entity_type = 4 AND
             workspace_id IS NOT NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS NOT NULL AND
             sprint_id IS NULL AND
             form_id IS NULL)
                OR
            (entity_type = 5 AND
             workspace_id IS NOT NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS  NULL AND
             sprint_id IS NULL AND
             form_id IS NOT NULL)
                OR
            (entity_type = 6 AND
             workspace_id IS NOT NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS NULL AND
             sprint_id IS NOT NULL AND
             form_id IS NULL)
            );

-- ============================================================================
-- Cached issue counters
--   sub_issues_count    — non-deleted issues with parent_id = self.id
--   link_count          — non-deleted issue_links with issue_id = self.id
--   attachment_count    — issue_attachments with issue_id = self.id
--   linked_issues_count — linked_issues rows where id1 = self.id OR id2 = self.id
--   comments_count      — non-deleted issue_comments with issue_id = self.id
-- Поддерживаются триггерами AFTER INSERT/UPDATE/DELETE на соответствующих таблицах.
-- ============================================================================

-- One-shot backfill: запускается только если триггеры ещё не были созданы.
-- На последующих запусках CreateTriggers() пропускается (триггер уже есть).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'sync_issue_sub_issues_count'
          AND tgrelid = 'issues'::regclass
          AND NOT tgisinternal
    ) THEN
        UPDATE issues i SET
            sub_issues_count    = COALESCE((SELECT count(*) FROM issues c            WHERE c.parent_id = i.id AND c.deleted_at IS NULL), 0),
            link_count          = COALESCE((SELECT count(*) FROM issue_links         WHERE issue_id = i.id AND deleted_at IS NULL), 0),
            attachment_count    = COALESCE((SELECT count(*) FROM issue_attachments   WHERE issue_id = i.id), 0),
            linked_issues_count = COALESCE((SELECT count(*) FROM linked_issues       WHERE id1 = i.id OR id2 = i.id), 0),
            comments_count      = COALESCE((SELECT count(*) FROM issue_comments      WHERE issue_id = i.id AND deleted_at IS NULL), 0);
    END IF;
END $$;

-- sub_issues_count: триггер на самой таблице issues (self-reference через parent_id).
-- Gate против каскадной рекурсии вверх по дереву родителей: если parent_id и
-- deleted_at не изменились — выходим сразу.
CREATE OR REPLACE FUNCTION sync_sub_issues_count()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.parent_id IS NOT DISTINCT FROM OLD.parent_id
       AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN
        RETURN NULL;
    END IF;

    -- Пересчёт для OLD.parent_id если он изменился или строка удалена
    IF (TG_OP = 'DELETE' OR (TG_OP = 'UPDATE' AND NEW.parent_id IS DISTINCT FROM OLD.parent_id))
       AND OLD.parent_id IS NOT NULL THEN
        UPDATE issues SET sub_issues_count =
            (SELECT count(*) FROM issues WHERE parent_id = OLD.parent_id AND deleted_at IS NULL)
            WHERE id = OLD.parent_id;
    END IF;

    -- Пересчёт для NEW.parent_id на INSERT/UPDATE
    IF TG_OP IN ('INSERT', 'UPDATE') AND NEW.parent_id IS NOT NULL THEN
        UPDATE issues SET sub_issues_count =
            (SELECT count(*) FROM issues WHERE parent_id = NEW.parent_id AND deleted_at IS NULL)
            WHERE id = NEW.parent_id;
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sync_issue_sub_issues_count
    AFTER INSERT OR UPDATE OR DELETE
    ON issues
    FOR EACH ROW
    EXECUTE PROCEDURE sync_sub_issues_count();

-- link_count: триггер на issue_links (soft-delete через deleted_at).
CREATE OR REPLACE FUNCTION sync_issue_link_count()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.issue_id IS NOT DISTINCT FROM OLD.issue_id
       AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN
        RETURN NULL;
    END IF;

    IF TG_OP = 'UPDATE' AND NEW.issue_id IS DISTINCT FROM OLD.issue_id THEN
        UPDATE issues SET link_count =
            (SELECT count(*) FROM issue_links WHERE issue_id = OLD.issue_id AND deleted_at IS NULL)
            WHERE id = OLD.issue_id;
    END IF;

    IF TG_OP = 'DELETE' THEN
        UPDATE issues SET link_count =
            (SELECT count(*) FROM issue_links WHERE issue_id = OLD.issue_id AND deleted_at IS NULL)
            WHERE id = OLD.issue_id;
    ELSE
        UPDATE issues SET link_count =
            (SELECT count(*) FROM issue_links WHERE issue_id = NEW.issue_id AND deleted_at IS NULL)
            WHERE id = NEW.issue_id;
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sync_issue_link_count
    AFTER INSERT OR UPDATE OR DELETE
    ON issue_links
    FOR EACH ROW
    EXECUTE PROCEDURE sync_issue_link_count();

-- attachment_count: триггер на issue_attachments (без soft-delete).
CREATE OR REPLACE FUNCTION sync_issue_attachment_count()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.issue_id IS NOT DISTINCT FROM OLD.issue_id THEN
        RETURN NULL;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        UPDATE issues SET attachment_count =
            (SELECT count(*) FROM issue_attachments WHERE issue_id = OLD.issue_id)
            WHERE id = OLD.issue_id;
    END IF;

    IF TG_OP = 'DELETE' THEN
        UPDATE issues SET attachment_count =
            (SELECT count(*) FROM issue_attachments WHERE issue_id = OLD.issue_id)
            WHERE id = OLD.issue_id;
    ELSE
        UPDATE issues SET attachment_count =
            (SELECT count(*) FROM issue_attachments WHERE issue_id = NEW.issue_id)
            WHERE id = NEW.issue_id;
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sync_issue_attachment_count
    AFTER INSERT OR UPDATE OR DELETE
    ON issue_attachments
    FOR EACH ROW
    EXECUTE PROCEDURE sync_issue_attachment_count();

-- linked_issues_count: триггер на linked_issues, обе стороны связки (id1, id2).
CREATE OR REPLACE FUNCTION sync_issue_linked_issues_count()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.id1 IS NOT DISTINCT FROM OLD.id1
       AND NEW.id2 IS NOT DISTINCT FROM OLD.id2 THEN
        RETURN NULL;
    END IF;

    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        UPDATE issues SET linked_issues_count =
            (SELECT count(*) FROM linked_issues WHERE id1 = issues.id OR id2 = issues.id)
            WHERE id IN (OLD.id1, OLD.id2);
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        UPDATE issues SET linked_issues_count =
            (SELECT count(*) FROM linked_issues WHERE id1 = issues.id OR id2 = issues.id)
            WHERE id IN (NEW.id1, NEW.id2);
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sync_issue_linked_issues_count
    AFTER INSERT OR UPDATE OR DELETE
    ON linked_issues
    FOR EACH ROW
    EXECUTE PROCEDURE sync_issue_linked_issues_count();

-- comments_count: триггер на issue_comments (soft-delete через deleted_at).
CREATE OR REPLACE FUNCTION sync_issue_comments_count()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.issue_id IS NOT DISTINCT FROM OLD.issue_id
       AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN
        RETURN NULL;
    END IF;

    IF TG_OP = 'UPDATE' AND NEW.issue_id IS DISTINCT FROM OLD.issue_id THEN
        UPDATE issues SET comments_count =
            (SELECT count(*) FROM issue_comments WHERE issue_id = OLD.issue_id AND deleted_at IS NULL)
            WHERE id = OLD.issue_id;
    END IF;

    IF TG_OP = 'DELETE' THEN
        UPDATE issues SET comments_count =
            (SELECT count(*) FROM issue_comments WHERE issue_id = OLD.issue_id AND deleted_at IS NULL)
            WHERE id = OLD.issue_id;
    ELSE
        UPDATE issues SET comments_count =
            (SELECT count(*) FROM issue_comments WHERE issue_id = NEW.issue_id AND deleted_at IS NULL)
            WHERE id = NEW.issue_id;
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sync_issue_comments_count
    AFTER INSERT OR UPDATE OR DELETE
    ON issue_comments
    FOR EACH ROW
    EXECUTE PROCEDURE sync_issue_comments_count();

-- ============================================================================
-- Уведомление об изменении состава workspace_summary:
-- отправляет workspace_id в канал workspace_summary_changes
-- при добавлении/обновлении/удалении проектов, форм и спринтов.
-- ============================================================================
CREATE OR REPLACE FUNCTION notify_workspace_summary_changes_projects()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
DECLARE
    ws_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ws_id := OLD.workspace_id;
    ELSE
        ws_id := NEW.workspace_id;
    END IF;

    PERFORM pg_notify('workspace_summary_changes', ws_id::text);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER project_workspace_summary_notify
    AFTER INSERT OR UPDATE OR DELETE
    ON projects
    FOR EACH ROW
    EXECUTE FUNCTION notify_workspace_summary_changes_projects();

CREATE OR REPLACE FUNCTION notify_workspace_summary_changes_forms()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
DECLARE
    ws_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ws_id := OLD.workspace_id;
    ELSE
        ws_id := NEW.workspace_id;
    END IF;

    PERFORM pg_notify('workspace_summary_changes', ws_id::text);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER form_workspace_summary_notify
    AFTER INSERT OR UPDATE OR DELETE
    ON forms
    FOR EACH ROW
    EXECUTE FUNCTION notify_workspace_summary_changes_forms();

CREATE OR REPLACE FUNCTION notify_workspace_summary_changes_sprints()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
DECLARE
    ws_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ws_id := OLD.workspace_id;
    ELSE
        ws_id := NEW.workspace_id;
    END IF;

    PERFORM pg_notify('workspace_summary_changes', ws_id::text);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sprint_workspace_summary_notify
    AFTER INSERT OR UPDATE OR DELETE
    ON sprints
    FOR EACH ROW
    EXECUTE FUNCTION notify_workspace_summary_changes_sprints();

-- Статистика спринта в workspace_summary считается по sprint_issues/issues/states
-- (см. cache.WorkspaceSummaryCache.fetch), поэтому кэш нужно инвалидировать и при
-- изменении состава спринта, и при изменении полей issue, влияющих на бакет статуса.
CREATE OR REPLACE FUNCTION notify_workspace_summary_changes_sprint_issues()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
DECLARE
    ws_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        ws_id := OLD.workspace_id;
    ELSE
        ws_id := NEW.workspace_id;
    END IF;

    PERFORM pg_notify('workspace_summary_changes', ws_id::text);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER sprint_issues_workspace_summary_notify
    AFTER INSERT OR UPDATE OR DELETE
    ON sprint_issues
    FOR EACH ROW
    EXECUTE FUNCTION notify_workspace_summary_changes_sprint_issues();

CREATE OR REPLACE FUNCTION notify_workspace_summary_changes_issue_sprint_stats()
    RETURNS TRIGGER
    LANGUAGE PLPGSQL
AS $$
BEGIN
    IF NEW.state_id IS NOT DISTINCT FROM OLD.state_id
       AND NEW.start_date IS NOT DISTINCT FROM OLD.start_date
       AND NEW.completed_at IS NOT DISTINCT FROM OLD.completed_at THEN
        RETURN NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM sprint_issues WHERE issue_id = NEW.id) THEN
        PERFORM pg_notify('workspace_summary_changes', NEW.workspace_id::text);
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER issue_sprint_stats_workspace_summary_notify
    AFTER UPDATE
    ON issues
    FOR EACH ROW
    EXECUTE FUNCTION notify_workspace_summary_changes_issue_sprint_stats();
