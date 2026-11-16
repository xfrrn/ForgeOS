CREATE INDEX project_head_checks ON work_items(project_id,id DESC) WHERE kind='git_head';
