-- hash columns cause gorm not supported them corrected
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION row_hash(VARIADIC text[])
RETURNS bytea AS $$
BEGIN
    RETURN digest(array_to_string($1, '_'), 'sha256');
END;
$$ LANGUAGE plpgsql IMMUTABLE;

ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, description, logo_id::text, slug, owner_id::text, integration_token, (deleted_at is null)::text)) STORED;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, public::text, identifier, project_lead_id::text, emoji::text, coalesce(cover_image, ''), coalesce(estimate_id, ''), coalesce(rules_script, ''), (deleted_at is null)::text)) STORED;
ALTER TABLE states ADD COLUMN IF NOT EXISTS "hash" bytea GENERATED ALWAYS AS (row_hash(name, description, color, "group", "default"::text, sequence::text)) STORED;

CREATE INDEX IF NOT EXISTS "idx_workspaces_hash" ON "workspaces" USING hash("hash");
CREATE INDEX IF NOT EXISTS "idx_projects_hash" ON "projects" USING hash("hash");
CREATE INDEX IF NOT EXISTS "idx_states_hash" ON "states" USING hash("hash");

-- indexes
CREATE EXTENSION if not exists pg_trgm;
CREATE EXTENSION if not exists btree_gin;
CREATE EXTENSION if not exists btree_gist;
CREATE INDEX IF NOT EXISTS "issues_gin" ON "issues" USING gin("workspace_id","project_id","sequence_id",(sequence_id::text)) WHERE deleted_at is not null;

-- delete deprecated
DO
$$
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
                'settings', u.settings
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
                'project_id', i.project_id,
                'created_by_id', i.created_by_id
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
