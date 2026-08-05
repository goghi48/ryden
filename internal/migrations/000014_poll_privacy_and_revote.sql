ALTER TABLE polls
    ADD COLUMN created_by_user_id uuid,
    ADD COLUMN is_anonymous boolean NOT NULL DEFAULT false,
    ADD COLUMN allow_revote boolean NOT NULL DEFAULT true;

UPDATE polls p
SET created_by_user_id = m.owner_id
FROM meetings m
WHERE m.id = p.meeting_id;

ALTER TABLE polls
    ALTER COLUMN created_by_user_id SET NOT NULL,
    ADD CONSTRAINT polls_created_by_user_fk
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE RESTRICT;
