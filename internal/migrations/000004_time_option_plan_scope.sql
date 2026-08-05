ALTER TABLE time_options
    ADD COLUMN plan_option_id uuid;

ALTER TABLE time_options
    ADD CONSTRAINT time_options_plan_option_fk
    FOREIGN KEY (meeting_id, plan_option_id)
    REFERENCES plan_options (meeting_id, id)
    ON DELETE CASCADE;

ALTER TABLE time_options
    DROP CONSTRAINT time_options_meeting_id_starts_at_ends_at_key;

ALTER TABLE time_options
    ADD CONSTRAINT time_options_meeting_scope_starts_ends_key
    UNIQUE NULLS NOT DISTINCT (meeting_id, plan_option_id, starts_at, ends_at);

CREATE INDEX time_options_meeting_plan_idx
    ON time_options (meeting_id, plan_option_id, position);
