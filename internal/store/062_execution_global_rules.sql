ALTER TABLE executions ADD COLUMN global_rules_revision integer REFERENCES global_rule_revisions(revision);
