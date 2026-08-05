ALTER TABLE attendance_responses
    DROP CONSTRAINT attendance_responses_status_check,
    ADD CONSTRAINT attendance_responses_status_check
        CHECK (status IN ('going', 'maybe', 'not_going'));
