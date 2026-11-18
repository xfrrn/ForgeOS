-- Some transitions can be written by different actors. Older rows retain unknown origins.
ALTER TABLE task_events ADD COLUMN origin text CHECK (origin IN ('owner','worker','github','management'));
