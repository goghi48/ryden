ALTER TABLE polls
    DROP CONSTRAINT polls_check;

ALTER TABLE polls
    ADD CONSTRAINT polls_closed_state_check
    CHECK (
        (state = 'open' AND closed_at IS NULL AND selected_option_id IS NULL)
        OR
        (state = 'closed' AND closed_at IS NOT NULL)
    );
