ALTER TABLE decision_questions ADD COLUMN feedback_id bigint UNIQUE REFERENCES preview_feedback(id);
ALTER TABLE decision_questions DROP CONSTRAINT decision_questions_execution_id_request_key_key;
CREATE UNIQUE INDEX decision_question_request_keys ON decision_questions(execution_id,request_key) WHERE feedback_id IS NULL;
