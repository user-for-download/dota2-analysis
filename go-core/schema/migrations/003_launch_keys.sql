-- =====================================================
-- 003_launch_keys.sql — Launch synchronization keys
-- =====================================================
CREATE TABLE IF NOT EXISTS analytics.launch_keys (
    key        TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE analytics.launch_keys IS
'Key-value store for service launch synchronization (e.g., featurizer_ready).';

-- Record migration (version is INT in schema_migrations)
INSERT INTO public.schema_migrations (version, filename) 
VALUES (3, '003_launch_keys.sql')
ON CONFLICT (version) DO NOTHING;
