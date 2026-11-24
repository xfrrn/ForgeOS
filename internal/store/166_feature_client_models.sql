ALTER TABLE feature_clients
    ADD COLUMN available_models jsonb NOT NULL DEFAULT '[]';
