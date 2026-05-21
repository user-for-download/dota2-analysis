-- =====================================================
-- 001_init.sql — OpenDota pipeline schema (Consolidated)
-- =====================================================

-- ----- Extensions --------------------------------------------------
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;
-- Note: pg_stat_statements requires `shared_preload_libraries = 'pg_stat_statements'`
-- in postgresql.conf to function.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Warn if pg_stat_statements is not active
DO $$
BEGIN
    IF current_setting('shared_preload_libraries', true) NOT LIKE '%pg_stat_statements%' THEN
        RAISE WARNING 'pg_stat_statements not in shared_preload_libraries; query stats will not be tracked';
    END IF;
END $$;

-- =====================================================
-- Helper: apply storage params to all leaf partitions of a parent.
-- Safe on non-partitioned tables too (applies directly).
-- =====================================================
CREATE OR REPLACE FUNCTION apply_storage_params(parent TEXT, params TEXT)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
    r RECORD;
    is_partitioned BOOLEAN;
BEGIN
    SELECT c.relkind = 'p' INTO is_partitioned
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relname = parent;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'table % not found', parent;
    END IF;

    IF NOT is_partitioned THEN
        EXECUTE format('ALTER TABLE %I SET (%s)', parent, params);
        RETURN;
    END IF;

    FOR r IN
        SELECT ch.relname AS part_name
        FROM pg_inherits i
        JOIN pg_class ch   ON ch.oid = i.inhrelid
        JOIN pg_class pt   ON pt.oid = i.inhparent
        JOIN pg_namespace n ON n.oid = ch.relnamespace
        WHERE pt.relname = parent AND n.nspname = 'public'
    LOOP
        EXECUTE format('ALTER TABLE %I SET (%s)', r.part_name, params);
    END LOOP;
END;
$$;

-- =====================================================
-- Game modes
-- =====================================================
CREATE TABLE IF NOT EXISTS game_modes (
    id          SMALLINT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    balanced    BOOLEAN DEFAULT FALSE,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
COMMENT ON TABLE game_modes IS 'Populated by enricher from /constants/game_mode.';

-- =====================================================
-- Lobby types
-- =====================================================
CREATE TABLE IF NOT EXISTS lobby_types (
    id          SMALLINT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    balanced    BOOLEAN DEFAULT FALSE,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
COMMENT ON TABLE lobby_types IS 'Populated by enricher from /constants/lobby_type.';

-- =====================================================
-- Regions
-- =====================================================
CREATE TABLE IF NOT EXISTS regions (
    id          SMALLINT PRIMARY KEY,
    name        TEXT NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_regions_name ON regions(name);
COMMENT ON TABLE regions IS 'Populated by enricher from /constants/region.';

-- =====================================================
-- Patches
-- =====================================================
CREATE TABLE IF NOT EXISTS patches (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    release_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_patches_release_at ON patches(release_at DESC);

-- =====================================================
-- Heroes
-- =====================================================
CREATE TABLE IF NOT EXISTS heroes (
    id              SMALLINT PRIMARY KEY,
    name            TEXT NOT NULL,
    localized_name  TEXT NOT NULL,
    primary_attr    TEXT,
    attack_type     TEXT,
    roles           TEXT[],
    legs            SMALLINT,
    img             TEXT,
    icon            TEXT,
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS heroes_name_idx ON heroes(name);

INSERT INTO heroes (id, name, localized_name)
VALUES (0, 'no_hero', 'No Hero')
ON CONFLICT (id) DO NOTHING;
COMMENT ON TABLE heroes IS 'Populated by enricher from /heroes; id=0 is a stub for empty draft slots.';

-- =====================================================
-- Hero FK stub safety net (consolidated from 002_fix_hero_fk_stubs.sql)
-- Prevents FK violations when parser ingests matches
-- referencing heroes not yet loaded by the enricher.
-- =====================================================
CREATE OR REPLACE FUNCTION ensure_hero_stubs(p_hero_ids SMALLINT[])
RETURNS INTEGER
LANGUAGE plpgsql AS $$
DECLARE
    inserted INTEGER;
BEGIN
    IF p_hero_ids IS NULL OR array_length(p_hero_ids, 1) IS NULL THEN
        RETURN 0;
    END IF;

    WITH ins AS (
        INSERT INTO heroes (id, name, localized_name)
        SELECT DISTINCT hid,
                        'unknown_' || hid::text,
                        'Unknown Hero ' || hid::text
        FROM unnest(p_hero_ids) AS t(hid)
        WHERE hid IS NOT NULL
        ON CONFLICT (id) DO NOTHING
        RETURNING 1
    )
    SELECT COUNT(*) INTO inserted FROM ins;

    RETURN inserted;
END;
$$;

COMMENT ON FUNCTION ensure_hero_stubs(SMALLINT[]) IS
'Bulk-insert stub hero rows. Enricher must overwrite via ON CONFLICT DO UPDATE on next /heroes run.';

CREATE OR REPLACE FUNCTION trg_ensure_hero_stub()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.hero_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM heroes WHERE id = NEW.hero_id) THEN
        INSERT INTO heroes (id, name, localized_name)
        VALUES (NEW.hero_id,
                'unknown_' || NEW.hero_id::text,
                'Unknown Hero ' || NEW.hero_id::text)
        ON CONFLICT (id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION trg_ensure_hero_stub() IS
'BEFORE INSERT trigger: auto-creates hero stubs to prevent FK violations on new patch heroes.';

CREATE INDEX IF NOT EXISTS idx_heroes_name_unknown
    ON heroes (name text_pattern_ops)
    WHERE name LIKE 'unknown\_%' ESCAPE '\';

CREATE OR REPLACE VIEW v_unknown_heroes AS
SELECT id, name, localized_name, updated_at,
       NOW() - updated_at AS age
FROM heroes
WHERE name LIKE 'unknown\_%' ESCAPE '\'
ORDER BY id;

COMMENT ON VIEW v_unknown_heroes IS
'Stub heroes awaiting enrichment. Alert if any row.age > ~1h: /heroes enricher is lagging.';

-- =====================================================
-- ITEMS_IDS
-- =====================================================
CREATE TABLE IF NOT EXISTS item_ids (
    id          INT PRIMARY KEY,
    key         TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_item_ids_key ON item_ids (key);

-- =====================================================
-- Hero stats (from /heroStats endpoint)
-- =====================================================
CREATE TABLE IF NOT EXISTS hero_stats (
    id                      SMALLINT PRIMARY KEY REFERENCES heroes(id) ON DELETE CASCADE,
    base_health             INTEGER,
    base_mana               INTEGER,
    base_armor              REAL,
    base_mr                 REAL,
    base_attack_min         SMALLINT,
    base_attack_max         SMALLINT,
    base_str                SMALLINT,
    base_agi                SMALLINT,
    base_int                SMALLINT,
    str_gain                REAL,
    agi_gain                REAL,
    int_gain                REAL,
    attack_range            SMALLINT,
    projectile_speed        SMALLINT,
    attack_rate             REAL,
    move_speed              SMALLINT,
    turn_rate               REAL,
    cm_enabled              BOOLEAN,
    turbo_picks             INTEGER,
    turbo_wins              INTEGER,
    pro_picks               INTEGER,
    pro_wins                INTEGER,
    pro_bans                INTEGER,
    pub_picks               INTEGER,
    pub_wins                INTEGER,
    pub_win_rate            REAL,
    pro_win_rate            REAL,
    updated_at              TIMESTAMPTZ DEFAULT NOW()
);

-- =====================================================
-- Items
-- =====================================================
CREATE TABLE IF NOT EXISTS items (
    id              INTEGER PRIMARY KEY,
    key             TEXT NOT NULL UNIQUE,
    dname           TEXT,
    qual            TEXT,
    behavior        TEXT,
    lore            TEXT,
    cooldown        TEXT,
    mana_cost       TEXT,
    cost            INTEGER,
    secret_shop     BOOLEAN DEFAULT FALSE,
    side_shop       BOOLEAN DEFAULT FALSE,
    recipe          BOOLEAN DEFAULT FALSE,
    created         BOOLEAN DEFAULT FALSE,
    img             TEXT,
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
COMMENT ON TABLE items IS 'Populated by enricher from /constants/items.';

-- =====================================================
-- Abilities
-- =====================================================
CREATE TABLE IF NOT EXISTS abilities (
    key          TEXT PRIMARY KEY,
    id           INTEGER,
    dname        TEXT NOT NULL DEFAULT '',
    behavior     JSONB,
    target_team  TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    img          TEXT NOT NULL DEFAULT '',
    mana_cost    TEXT NOT NULL DEFAULT '',
    cooldown     TEXT NOT NULL DEFAULT '',
    attrib       JSONB,
    is_talent    BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_abilities_dname ON abilities (dname) WHERE dname <> '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_abilities_id ON abilities (id) WHERE id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_abilities_talent_key ON abilities (key) WHERE is_talent = TRUE;
CREATE INDEX IF NOT EXISTS idx_abilities_behavior_gin ON abilities USING GIN (behavior jsonb_path_ops);

COMMENT ON TABLE abilities IS 'Populated by enricher from /constants/abilities.';

-- =====================================================
-- Hero Relations (Abilities, Talents, Facets)
-- =====================================================
CREATE TABLE IF NOT EXISTS hero_abilities (
    hero_name TEXT NOT NULL REFERENCES heroes(name) ON DELETE CASCADE,
    slot      SMALLINT NOT NULL CHECK (slot >= 0 AND slot <= 15),
    ability   TEXT NOT NULL REFERENCES abilities(key) ON DELETE CASCADE,
    PRIMARY KEY (hero_name, slot)
);
CREATE INDEX IF NOT EXISTS idx_hero_abilities_ability ON hero_abilities(ability);
COMMENT ON TABLE hero_abilities IS 'Hero ability slots (0-15). Populated by enricher from hero_abilities.json.';

CREATE TABLE IF NOT EXISTS hero_talents (
    hero_name TEXT NOT NULL REFERENCES heroes(name) ON DELETE CASCADE,
    ability   TEXT NOT NULL REFERENCES abilities(key) ON DELETE CASCADE,
    level     SMALLINT NOT NULL CHECK (level BETWEEN 1 AND 4),
    PRIMARY KEY (hero_name, ability)
);
CREATE INDEX IF NOT EXISTS idx_hero_talents_ability ON hero_talents(ability);
COMMENT ON TABLE hero_talents IS 'Hero talents with level tiers (1-4). Populated by enricher from hero_abilities.json.';

CREATE TABLE IF NOT EXISTS hero_facets (
    hero_name    TEXT NOT NULL REFERENCES heroes(name) ON DELETE CASCADE,
    slot         SMALLINT NOT NULL,
    name         TEXT NOT NULL,
    title        TEXT,
    description  TEXT,
    icon         TEXT,
    color        TEXT,
    gradient_id  SMALLINT,
    deprecated   BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (hero_name, slot)
);
CREATE INDEX IF NOT EXISTS idx_hero_facets_hero ON hero_facets(hero_name);
COMMENT ON TABLE hero_facets IS 'Hero facets (7.34+). Populated by enricher from hero_abilities.json.';

-- =====================================================
-- Leagues / tournaments
-- =====================================================
CREATE TABLE IF NOT EXISTS leagues (
    leagueid    BIGINT PRIMARY KEY,
    name        TEXT,
    tier        TEXT,
    ticket      TEXT,
    banner      TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_leagues_tier ON leagues(tier) WHERE tier IS NOT NULL;

-- =====================================================
-- Teams
-- =====================================================
CREATE TABLE IF NOT EXISTS teams (
    team_id         BIGINT PRIMARY KEY,
    name            TEXT NOT NULL,
    tag             TEXT,
    logo_url        TEXT,
    rating          REAL,
    wins            INT,
    losses          INT,
    last_match_time BIGINT,
    delta           REAL,
    match_id        BIGINT,
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_teams_name_trgm ON teams USING GIN (name gin_trgm_ops);

-- =====================================================
-- Players (Steam profiles)
-- =====================================================
CREATE TABLE IF NOT EXISTS players (
    account_id          BIGINT PRIMARY KEY,
    steamid             TEXT,
    personaname         TEXT,
    avatar              TEXT,
    avatarmedium        TEXT,
    avatarfull          TEXT,
    profileurl          TEXT,
    loccountrycode      TEXT,
    plus                BOOLEAN DEFAULT FALSE,
    cheese              INTEGER DEFAULT 0,
    fh_unavailable      BOOLEAN DEFAULT FALSE,
    last_login          TIMESTAMPTZ,
    last_match_time     TIMESTAMPTZ,
    full_history_time   TIMESTAMPTZ,
    profile_time        TIMESTAMPTZ,
    rank_tier_time      TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_players_personaname_trgm ON players USING GIN (personaname gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_players_last_match_time ON players(last_match_time DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_players_full_history_time ON players(full_history_time ASC NULLS FIRST);
CREATE INDEX IF NOT EXISTS idx_players_profile_time ON players(profile_time ASC NULLS FIRST);
CREATE INDEX IF NOT EXISTS idx_players_rank_tier_time ON players(rank_tier_time ASC NULLS FIRST);

COMMENT ON COLUMN players.last_match_time IS 'Derived from MAX(player_matches.start_time).';
COMMENT ON COLUMN players.last_login IS 'From /players/{id}.profile.last_login; loader must parse as UTC.';

-- =====================================================
-- Notable (pro) players
-- =====================================================
-- FK to players omitted because players may not be populated.
CREATE TABLE IF NOT EXISTS notable_players (
    account_id      BIGINT PRIMARY KEY,
    name            TEXT,
    country_code    TEXT,
    fantasy_role    SMALLINT,
    team_id         BIGINT REFERENCES teams(team_id) ON DELETE SET NULL,
    team_name       TEXT,
    team_tag        TEXT,
    is_pro          BOOLEAN DEFAULT TRUE,
    locked_until    BIGINT,
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notable_players_team_id ON notable_players(team_id) WHERE team_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_notable_players_team_name_trgm ON notable_players USING GIN (team_name gin_trgm_ops) WHERE team_name IS NOT NULL;

COMMENT ON COLUMN notable_players.locked_until IS 'Epoch seconds (BIGINT) — consistent with other *_time columns. NULL means not locked.';

-- =====================================================
-- Player rank history
-- =====================================================
CREATE TABLE IF NOT EXISTS player_ranks (
    account_id              BIGINT NOT NULL,
    recorded_at             TIMESTAMPTZ NOT NULL,
    rank_tier               SMALLINT CHECK (rank_tier IS NULL OR rank_tier BETWEEN 0 AND 85),
    leaderboard_rank        INTEGER,
    solo_competitive_rank   INTEGER,
    competitive_rank        INTEGER,
    match_id                BIGINT,
    PRIMARY KEY (account_id, recorded_at)
);
CREATE INDEX IF NOT EXISTS idx_player_ranks_account ON player_ranks(account_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_player_ranks_leaderboard ON player_ranks(leaderboard_rank) WHERE leaderboard_rank IS NOT NULL;

COMMENT ON COLUMN player_ranks.match_id IS 'Logical FK to matches(match_id). NOT enforced at DB level; optional snapshot context for rank change.';

-- =====================================================
-- Team rating
-- =====================================================
CREATE TABLE IF NOT EXISTS team_rating (
    team_id         BIGINT PRIMARY KEY REFERENCES teams(team_id) ON DELETE CASCADE,
    rating          REAL    NOT NULL DEFAULT 0,
    wins            INTEGER NOT NULL DEFAULT 0,
    losses          INTEGER NOT NULL DEFAULT 0,
    last_match_time BIGINT  NOT NULL DEFAULT 0,
    last_match_id   BIGINT  NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_team_rating_rating ON team_rating(rating DESC);

-- =====================================================
-- Team matches
-- =====================================================
CREATE TABLE IF NOT EXISTS team_matches (
    team_id    BIGINT NOT NULL,
    match_id   BIGINT NOT NULL,
    start_time BIGINT NOT NULL,
    is_radiant BOOLEAN NOT NULL,
    win        BOOLEAN NOT NULL,
    leagueid   INTEGER,
    PRIMARY KEY (team_id, match_id)
);
CREATE INDEX IF NOT EXISTS idx_team_matches_team_id ON team_matches(team_id);
CREATE INDEX IF NOT EXISTS idx_team_matches_match_id ON team_matches(match_id);

COMMENT ON COLUMN team_matches.match_id IS 'Logical FK to matches(match_id). NOT enforced at DB level.';

-- =====================================================
-- Pro Players (from /api/proPlayers)
-- =====================================================
CREATE TABLE IF NOT EXISTS pro_players (
    account_id      BIGINT PRIMARY KEY,
    steamid         TEXT,
    personaname     TEXT,
    name            TEXT,
    country_code    TEXT,
    fantasy_role    SMALLINT,
    team_id         BIGINT,
    team_name       TEXT,
    team_tag        TEXT,
    is_pro          BOOLEAN,
    is_locked       BOOLEAN,
    avatar          TEXT,
    last_match_time TIMESTAMPTZ,
    last_login      TIMESTAMPTZ,
    full_history_time TIMESTAMPTZ,
    cheese          SMALLINT,
    fh_unavailable  BOOLEAN,
    loccountrycode  TEXT,
    plus            BOOLEAN,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pro_players_team_idx ON pro_players (team_id);

-- =====================================================
-- Schema migrations tracking
-- =====================================================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INT PRIMARY KEY,
    filename   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Insert initial migration version (runner also manages this, but we insert here for safety)
INSERT INTO schema_migrations (version, filename) VALUES (1, '001_init.sql')
ON CONFLICT (version) DO NOTHING;

-- =====================================================
-- Matches (RANGE partitioned on start_time, quarterly)
-- =====================================================
CREATE TABLE IF NOT EXISTS matches (
    match_id                BIGINT NOT NULL,
    match_seq_num           BIGINT,
    start_time              BIGINT NOT NULL,
    duration                INTEGER NOT NULL CHECK (duration >= 0),
    radiant_win             BOOLEAN,
    tower_status_radiant    SMALLINT,
    tower_status_dire       SMALLINT,
    barracks_status_radiant SMALLINT,
    barracks_status_dire    SMALLINT,
    radiant_score           SMALLINT CHECK (radiant_score IS NULL OR radiant_score >= 0),
    dire_score              SMALLINT CHECK (dire_score    IS NULL OR dire_score    >= 0),
    first_blood_time        INTEGER,
    lobby_type              SMALLINT,        -- FK dropped for reliability
    game_mode               SMALLINT,        -- FK dropped
    cluster                 SMALLINT,
    region                  SMALLINT,        -- FK dropped
    skill                   SMALLINT,
    engine                  SMALLINT,
    human_players           SMALLINT,
    version                 SMALLINT,
    patch_id                INTEGER,         -- FK dropped
    positive_votes          INTEGER DEFAULT 0,
    negative_votes          INTEGER DEFAULT 0,
    leagueid                INTEGER,         -- FK dropped
    series_id               INTEGER,
    series_type             SMALLINT,
    radiant_team_id         BIGINT,          -- FK dropped
    dire_team_id            BIGINT,          -- FK dropped
    radiant_captain         BIGINT,
    dire_captain            BIGINT,
    replay_salt             BIGINT,
    replay_url              TEXT,
    pauses                  JSONB,
    is_parsed               BOOLEAN DEFAULT FALSE,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (match_id, start_time)
) PARTITION BY RANGE (start_time);

COMMENT ON COLUMN matches.start_time  IS 'Unix epoch seconds. PK component; updates are forbidden. Determines partition.';
COMMENT ON COLUMN matches.radiant_win IS 'Nullable per OpenDota API (abandoned/incomplete matches).';
COMMENT ON COLUMN matches.pauses      IS 'Array of {time,duration} pause events from API.';

-- Quarterly partitions 2024-2027
CREATE TABLE IF NOT EXISTS matches_2024_q1 PARTITION OF matches FOR VALUES FROM (1704067200) TO (1711929600);
CREATE TABLE IF NOT EXISTS matches_2024_q2 PARTITION OF matches FOR VALUES FROM (1711929600) TO (1719792000);
CREATE TABLE IF NOT EXISTS matches_2024_q3 PARTITION OF matches FOR VALUES FROM (1719792000) TO (1727740800);
CREATE TABLE IF NOT EXISTS matches_2024_q4 PARTITION OF matches FOR VALUES FROM (1727740800) TO (1735689600);
CREATE TABLE IF NOT EXISTS matches_2025_q1 PARTITION OF matches FOR VALUES FROM (1735689600) TO (1743465600);
CREATE TABLE IF NOT EXISTS matches_2025_q2 PARTITION OF matches FOR VALUES FROM (1743465600) TO (1751328000);
CREATE TABLE IF NOT EXISTS matches_2025_q3 PARTITION OF matches FOR VALUES FROM (1751328000) TO (1759276800);
CREATE TABLE IF NOT EXISTS matches_2025_q4 PARTITION OF matches FOR VALUES FROM (1759276800) TO (1767225600);
CREATE TABLE IF NOT EXISTS matches_2026_q1 PARTITION OF matches FOR VALUES FROM (1767225600) TO (1775001600);
CREATE TABLE IF NOT EXISTS matches_2026_q2 PARTITION OF matches FOR VALUES FROM (1775001600) TO (1782864000);
CREATE TABLE IF NOT EXISTS matches_2026_q3 PARTITION OF matches FOR VALUES FROM (1782864000) TO (1790812800);
CREATE TABLE IF NOT EXISTS matches_2026_q4 PARTITION OF matches FOR VALUES FROM (1790812800) TO (1798761600);
CREATE TABLE IF NOT EXISTS matches_2027_q1 PARTITION OF matches FOR VALUES FROM (1798761600) TO (1806624000);
CREATE TABLE IF NOT EXISTS matches_2027_q2 PARTITION OF matches FOR VALUES FROM (1806624000) TO (1814486400);
CREATE TABLE IF NOT EXISTS matches_2027_q3 PARTITION OF matches FOR VALUES FROM (1814486400) TO (1822435200);
CREATE TABLE IF NOT EXISTS matches_2027_q4 PARTITION OF matches FOR VALUES FROM (1822435200) TO (1830384000);
CREATE TABLE IF NOT EXISTS matches_default PARTITION OF matches DEFAULT;

CREATE INDEX IF NOT EXISTS idx_matches_match_id     ON matches(match_id);
CREATE INDEX IF NOT EXISTS idx_matches_start_time   ON matches(start_time DESC);
CREATE INDEX IF NOT EXISTS brin_matches_start_time  ON matches USING BRIN (start_time) WITH (pages_per_range = 32);
CREATE INDEX IF NOT EXISTS idx_matches_leagueid     ON matches(leagueid, start_time DESC) WHERE leagueid IS NOT NULL AND leagueid > 0;
CREATE INDEX IF NOT EXISTS idx_matches_radiant_team ON matches(radiant_team_id, start_time DESC) WHERE radiant_team_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matches_dire_team    ON matches(dire_team_id, start_time DESC)    WHERE dire_team_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matches_series       ON matches(series_id, start_time DESC)       WHERE series_id IS NOT NULL AND series_id > 0;
CREATE INDEX IF NOT EXISTS idx_matches_patch        ON matches(patch_id, start_time DESC)        WHERE patch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matches_unparsed     ON matches(match_id) WHERE is_parsed = FALSE;
CREATE INDEX IF NOT EXISTS idx_matches_recent_covering ON matches(start_time DESC) INCLUDE (match_id, radiant_win, duration, leagueid);

SELECT apply_storage_params(
    'matches',
    'autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01'
);

-- =====================================================
-- player_matches (HOT, RANGE partitioned)
-- =====================================================
CREATE TABLE IF NOT EXISTS player_matches (
    match_id                BIGINT NOT NULL,
    player_slot             SMALLINT NOT NULL,  -- API: nullable, range not guaranteed
    start_time              BIGINT NOT NULL,
    account_id              BIGINT,
    hero_id                 SMALLINT NOT NULL REFERENCES heroes(id),
    hero_variant            SMALLINT,
    is_radiant              BOOLEAN NOT NULL,
    win                     BOOLEAN,
    duration                INTEGER NOT NULL,
    patch_id                INTEGER,
    lobby_type              SMALLINT,
    game_mode               SMALLINT,
    rank_tier               SMALLINT CHECK (rank_tier IS NULL OR rank_tier BETWEEN 0 AND 85),
    kills                   SMALLINT NOT NULL DEFAULT 0,
    deaths                  SMALLINT NOT NULL DEFAULT 0,
    assists                 SMALLINT NOT NULL DEFAULT 0,
    level                   SMALLINT,
    net_worth               INTEGER,
    gold                    INTEGER,
    gold_spent              INTEGER,
    gold_per_min            SMALLINT,
    xp_per_min              SMALLINT,
    last_hits               SMALLINT,
    denies                  SMALLINT,
    hero_damage             INTEGER,
    tower_damage            INTEGER,
    hero_healing            INTEGER,
    item_0                  INTEGER,
    item_1                  INTEGER,
    item_2                  INTEGER,
    item_3                  INTEGER,
    item_4                  INTEGER,
    item_5                  INTEGER,
    item_neutral            INTEGER,
    backpack_0              INTEGER,
    backpack_1              INTEGER,
    backpack_2              INTEGER,
    backpack_3              INTEGER,
    lane                    SMALLINT,
    lane_role               SMALLINT,
    is_roaming              BOOLEAN,
    party_id                INTEGER,
    party_size              SMALLINT,
    stuns                   REAL,
    obs_placed              SMALLINT,
    sen_placed              SMALLINT,
    creeps_stacked          SMALLINT,
    camps_stacked           SMALLINT,
    rune_pickups            SMALLINT,
    firstblood_claimed      BOOLEAN,
    teamfight_participation REAL,
    towers_killed           SMALLINT,
    roshans_killed          SMALLINT,
    observers_placed        SMALLINT,
    leaver_status           SMALLINT,
    gold_t                  INTEGER[],
    xp_t                    INTEGER[],
    lh_t                    INTEGER[],
    dn_t                    INTEGER[],
    times                   INTEGER[],
    throw_gold              INTEGER,
    comeback_gold           INTEGER,
    loss_gold               INTEGER,
    win_gold                INTEGER,
    PRIMARY KEY (match_id, player_slot, start_time)
) PARTITION BY RANGE (start_time);

COMMENT ON COLUMN player_matches.match_id      IS 'Logical FK to matches. NOT enforced at DB level.';
COMMENT ON COLUMN player_matches.gold_t        IS 'Per-minute total gold (from API gold_t)';
COMMENT ON COLUMN player_matches.throw_gold    IS 'Gold advantage lost when losing (from API throw)';

-- Quarterly partitions 2024-2027
CREATE TABLE IF NOT EXISTS player_matches_2024_q1 PARTITION OF player_matches FOR VALUES FROM (1704067200) TO (1711929600);
CREATE TABLE IF NOT EXISTS player_matches_2024_q2 PARTITION OF player_matches FOR VALUES FROM (1711929600) TO (1719792000);
CREATE TABLE IF NOT EXISTS player_matches_2024_q3 PARTITION OF player_matches FOR VALUES FROM (1719792000) TO (1727740800);
CREATE TABLE IF NOT EXISTS player_matches_2024_q4 PARTITION OF player_matches FOR VALUES FROM (1727740800) TO (1735689600);
CREATE TABLE IF NOT EXISTS player_matches_2025_q1 PARTITION OF player_matches FOR VALUES FROM (1735689600) TO (1743465600);
CREATE TABLE IF NOT EXISTS player_matches_2025_q2 PARTITION OF player_matches FOR VALUES FROM (1743465600) TO (1751328000);
CREATE TABLE IF NOT EXISTS player_matches_2025_q3 PARTITION OF player_matches FOR VALUES FROM (1751328000) TO (1759276800);
CREATE TABLE IF NOT EXISTS player_matches_2025_q4 PARTITION OF player_matches FOR VALUES FROM (1759276800) TO (1767225600);
CREATE TABLE IF NOT EXISTS player_matches_2026_q1 PARTITION OF player_matches FOR VALUES FROM (1767225600) TO (1775001600);
CREATE TABLE IF NOT EXISTS player_matches_2026_q2 PARTITION OF player_matches FOR VALUES FROM (1775001600) TO (1782864000);
CREATE TABLE IF NOT EXISTS player_matches_2026_q3 PARTITION OF player_matches FOR VALUES FROM (1782864000) TO (1790812800);
CREATE TABLE IF NOT EXISTS player_matches_2026_q4 PARTITION OF player_matches FOR VALUES FROM (1790812800) TO (1798761600);
CREATE TABLE IF NOT EXISTS player_matches_2027_q1 PARTITION OF player_matches FOR VALUES FROM (1798761600) TO (1806624000);
CREATE TABLE IF NOT EXISTS player_matches_2027_q2 PARTITION OF player_matches FOR VALUES FROM (1806624000) TO (1814486400);
CREATE TABLE IF NOT EXISTS player_matches_2027_q3 PARTITION OF player_matches FOR VALUES FROM (1814486400) TO (1822435200);
CREATE TABLE IF NOT EXISTS player_matches_2027_q4 PARTITION OF player_matches FOR VALUES FROM (1822435200) TO (1830384000);
CREATE TABLE IF NOT EXISTS player_matches_default PARTITION OF player_matches DEFAULT;

CREATE INDEX IF NOT EXISTS idx_pm_hero ON player_matches(hero_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_pm_hero_patch ON player_matches(hero_id, patch_id) WHERE patch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pm_account_hero_time ON player_matches(account_id, hero_id, start_time DESC) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pm_account_time ON player_matches(account_id, start_time DESC) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS brin_player_matches_start_time ON player_matches USING BRIN (start_time) WITH (pages_per_range = 32);

SELECT apply_storage_params(
    'player_matches',
    'autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01'
);

-- =====================================================
-- player_match_details — COLD (HASH partitioned, 32 parts)
-- =====================================================
CREATE TABLE IF NOT EXISTS player_match_details (
    match_id                    BIGINT NOT NULL,
    player_slot                 SMALLINT NOT NULL,  -- API: nullable, range not guaranteed
    damage                      JSONB, damage_taken JSONB, damage_inflictor JSONB, damage_inflictor_received JSONB,
    damage_targets              JSONB, hero_hits JSONB, max_hero_hit JSONB, ability_uses JSONB, ability_targets JSONB,
    ability_upgrades_arr        JSONB, item_uses JSONB, gold_reasons JSONB, xp_reasons JSONB, killed JSONB, killed_by JSONB,
    kill_streaks                JSONB, multi_kills JSONB, life_state JSONB, lane_pos JSONB, obs JSONB, sen JSONB, actions JSONB,
    pings                       JSONB, runes JSONB, purchase JSONB, obs_log JSONB, sen_log JSONB, obs_left_log JSONB, sen_left_log JSONB,
    purchase_log                JSONB, kills_log JSONB, buyback_log JSONB, runes_log JSONB, connection_log JSONB, permanent_buffs JSONB,
    neutral_tokens_log          JSONB, neutral_item_history JSONB, additional_units JSONB, cosmetics JSONB, benchmarks JSONB,
    all_word_counts             JSONB, my_word_counts JSONB,
    PRIMARY KEY (match_id, player_slot)
) PARTITION BY HASH (match_id);

CREATE TABLE IF NOT EXISTS player_match_details_p0 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 0);
CREATE TABLE IF NOT EXISTS player_match_details_p1 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 1);
CREATE TABLE IF NOT EXISTS player_match_details_p2 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 2);
CREATE TABLE IF NOT EXISTS player_match_details_p3 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 3);
CREATE TABLE IF NOT EXISTS player_match_details_p4 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 4);
CREATE TABLE IF NOT EXISTS player_match_details_p5 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 5);
CREATE TABLE IF NOT EXISTS player_match_details_p6 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 6);
CREATE TABLE IF NOT EXISTS player_match_details_p7 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 7);
CREATE TABLE IF NOT EXISTS player_match_details_p8 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 8);
CREATE TABLE IF NOT EXISTS player_match_details_p9 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 9);
CREATE TABLE IF NOT EXISTS player_match_details_p10 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 10);
CREATE TABLE IF NOT EXISTS player_match_details_p11 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 11);
CREATE TABLE IF NOT EXISTS player_match_details_p12 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 12);
CREATE TABLE IF NOT EXISTS player_match_details_p13 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 13);
CREATE TABLE IF NOT EXISTS player_match_details_p14 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 14);
CREATE TABLE IF NOT EXISTS player_match_details_p15 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 15);
CREATE TABLE IF NOT EXISTS player_match_details_p16 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 16);
CREATE TABLE IF NOT EXISTS player_match_details_p17 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 17);
CREATE TABLE IF NOT EXISTS player_match_details_p18 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 18);
CREATE TABLE IF NOT EXISTS player_match_details_p19 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 19);
CREATE TABLE IF NOT EXISTS player_match_details_p20 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 20);
CREATE TABLE IF NOT EXISTS player_match_details_p21 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 21);
CREATE TABLE IF NOT EXISTS player_match_details_p22 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 22);
CREATE TABLE IF NOT EXISTS player_match_details_p23 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 23);
CREATE TABLE IF NOT EXISTS player_match_details_p24 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 24);
CREATE TABLE IF NOT EXISTS player_match_details_p25 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 25);
CREATE TABLE IF NOT EXISTS player_match_details_p26 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 26);
CREATE TABLE IF NOT EXISTS player_match_details_p27 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 27);
CREATE TABLE IF NOT EXISTS player_match_details_p28 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 28);
CREATE TABLE IF NOT EXISTS player_match_details_p29 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 29);
CREATE TABLE IF NOT EXISTS player_match_details_p30 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 30);
CREATE TABLE IF NOT EXISTS player_match_details_p31 PARTITION OF player_match_details FOR VALUES WITH (MODULUS 32, REMAINDER 31);

-- =====================================================
-- Picks / Bans (HASH partitioned, 16 parts)
-- =====================================================
CREATE TABLE IF NOT EXISTS picks_bans (
    match_id        BIGINT NOT NULL,
    ord             SMALLINT NOT NULL,
    is_pick         BOOLEAN NOT NULL,
    hero_id         SMALLINT NOT NULL REFERENCES heroes(id),
    team            SMALLINT NOT NULL,  -- API integer, values 0-3 observed
    PRIMARY KEY (match_id, ord)
) PARTITION BY HASH (match_id);

CREATE INDEX IF NOT EXISTS idx_picks_bans_hero ON picks_bans(hero_id, is_pick);

CREATE TABLE IF NOT EXISTS picks_bans_p0 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE IF NOT EXISTS picks_bans_p1 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE IF NOT EXISTS picks_bans_p2 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE IF NOT EXISTS picks_bans_p3 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE IF NOT EXISTS picks_bans_p4 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE IF NOT EXISTS picks_bans_p5 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE IF NOT EXISTS picks_bans_p6 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE IF NOT EXISTS picks_bans_p7 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE IF NOT EXISTS picks_bans_p8 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE IF NOT EXISTS picks_bans_p9 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE IF NOT EXISTS picks_bans_p10 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE IF NOT EXISTS picks_bans_p11 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE IF NOT EXISTS picks_bans_p12 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE IF NOT EXISTS picks_bans_p13 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE IF NOT EXISTS picks_bans_p14 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE IF NOT EXISTS picks_bans_p15 PARTITION OF picks_bans FOR VALUES WITH (MODULUS 16, REMAINDER 15);

-- =====================================================
-- Draft timings (HASH partitioned, 16 parts)
-- =====================================================
CREATE TABLE IF NOT EXISTS draft_timings (
    match_id          BIGINT   NOT NULL,
    ord               SMALLINT NOT NULL,
    pick              BOOLEAN  NOT NULL,
    active_team       SMALLINT,  -- API: values 0-3 observed in tournament lobbies
    hero_id           SMALLINT REFERENCES heroes(id),
    player_slot       SMALLINT,  -- API: values 0-9+ in CM matches, not guaranteed
    extra_time        INTEGER,
    total_time_taken  INTEGER,
    PRIMARY KEY (match_id, ord)
) PARTITION BY HASH (match_id);

CREATE INDEX IF NOT EXISTS idx_draft_timings_hero ON draft_timings(hero_id);

CREATE TABLE IF NOT EXISTS draft_timings_p0 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE IF NOT EXISTS draft_timings_p1 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE IF NOT EXISTS draft_timings_p2 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE IF NOT EXISTS draft_timings_p3 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE IF NOT EXISTS draft_timings_p4 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE IF NOT EXISTS draft_timings_p5 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE IF NOT EXISTS draft_timings_p6 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE IF NOT EXISTS draft_timings_p7 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE IF NOT EXISTS draft_timings_p8 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE IF NOT EXISTS draft_timings_p9 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE IF NOT EXISTS draft_timings_p10 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE IF NOT EXISTS draft_timings_p11 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE IF NOT EXISTS draft_timings_p12 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE IF NOT EXISTS draft_timings_p13 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE IF NOT EXISTS draft_timings_p14 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE IF NOT EXISTS draft_timings_p15 PARTITION OF draft_timings FOR VALUES WITH (MODULUS 16, REMAINDER 15);

-- =====================================================
-- Match events (HASH partitioned, 16 parts)
-- Identity IDs are NOT stable. MUST delete prior to re-insert.
-- =====================================================
CREATE TABLE IF NOT EXISTS match_objectives (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    match_id        BIGINT NOT NULL,
    start_time      BIGINT NOT NULL,
    time            INTEGER NOT NULL,
    type            TEXT NOT NULL,
    slot            SMALLINT,
    player_slot     SMALLINT,
    team            SMALLINT,
    key             TEXT,
    value           INTEGER,
    unit            TEXT,
    raw             JSONB,
    PRIMARY KEY (match_id, id)
) PARTITION BY HASH (match_id);

COMMENT ON TABLE match_objectives IS 'HASH partitioned on match_id. Loader MUST DELETE WHERE match_id = $1 before re-inserting.';

CREATE TABLE IF NOT EXISTS match_objectives_p0 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE IF NOT EXISTS match_objectives_p1 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE IF NOT EXISTS match_objectives_p2 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE IF NOT EXISTS match_objectives_p3 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE IF NOT EXISTS match_objectives_p4 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE IF NOT EXISTS match_objectives_p5 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE IF NOT EXISTS match_objectives_p6 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE IF NOT EXISTS match_objectives_p7 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE IF NOT EXISTS match_objectives_p8 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE IF NOT EXISTS match_objectives_p9 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE IF NOT EXISTS match_objectives_p10 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE IF NOT EXISTS match_objectives_p11 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE IF NOT EXISTS match_objectives_p12 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE IF NOT EXISTS match_objectives_p13 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE IF NOT EXISTS match_objectives_p14 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE IF NOT EXISTS match_objectives_p15 PARTITION OF match_objectives FOR VALUES WITH (MODULUS 16, REMAINDER 15);

CREATE TABLE IF NOT EXISTS match_chat (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    match_id        BIGINT NOT NULL,
    start_time      BIGINT NOT NULL,
    time            INTEGER NOT NULL,
    type            TEXT,
    player_slot     SMALLINT,
    unit            TEXT,
    key             TEXT,
    PRIMARY KEY (match_id, id)
) PARTITION BY HASH (match_id);

COMMENT ON TABLE match_chat IS 'Loader MUST DELETE WHERE match_id = $1 before re-inserting.';

CREATE TABLE IF NOT EXISTS match_chat_p0 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE IF NOT EXISTS match_chat_p1 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE IF NOT EXISTS match_chat_p2 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE IF NOT EXISTS match_chat_p3 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE IF NOT EXISTS match_chat_p4 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE IF NOT EXISTS match_chat_p5 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE IF NOT EXISTS match_chat_p6 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE IF NOT EXISTS match_chat_p7 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE IF NOT EXISTS match_chat_p8 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE IF NOT EXISTS match_chat_p9 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE IF NOT EXISTS match_chat_p10 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE IF NOT EXISTS match_chat_p11 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE IF NOT EXISTS match_chat_p12 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE IF NOT EXISTS match_chat_p13 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE IF NOT EXISTS match_chat_p14 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE IF NOT EXISTS match_chat_p15 PARTITION OF match_chat FOR VALUES WITH (MODULUS 16, REMAINDER 15);

CREATE TABLE IF NOT EXISTS match_teamfights (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    match_id        BIGINT NOT NULL,
    start_time      BIGINT NOT NULL,
    end_time        INTEGER NOT NULL,
    last_death      INTEGER,
    deaths          SMALLINT,
    players         JSONB,
    PRIMARY KEY (match_id, id)
) PARTITION BY HASH (match_id);

COMMENT ON TABLE match_teamfights IS 'Loader MUST DELETE WHERE match_id = $1 before re-inserting.';

CREATE TABLE IF NOT EXISTS match_teamfights_p0 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 0);
CREATE TABLE IF NOT EXISTS match_teamfights_p1 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 1);
CREATE TABLE IF NOT EXISTS match_teamfights_p2 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 2);
CREATE TABLE IF NOT EXISTS match_teamfights_p3 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 3);
CREATE TABLE IF NOT EXISTS match_teamfights_p4 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 4);
CREATE TABLE IF NOT EXISTS match_teamfights_p5 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 5);
CREATE TABLE IF NOT EXISTS match_teamfights_p6 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 6);
CREATE TABLE IF NOT EXISTS match_teamfights_p7 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 7);
CREATE TABLE IF NOT EXISTS match_teamfights_p8 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 8);
CREATE TABLE IF NOT EXISTS match_teamfights_p9 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 9);
CREATE TABLE IF NOT EXISTS match_teamfights_p10 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 10);
CREATE TABLE IF NOT EXISTS match_teamfights_p11 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 11);
CREATE TABLE IF NOT EXISTS match_teamfights_p12 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 12);
CREATE TABLE IF NOT EXISTS match_teamfights_p13 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 13);
CREATE TABLE IF NOT EXISTS match_teamfights_p14 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 14);
CREATE TABLE IF NOT EXISTS match_teamfights_p15 PARTITION OF match_teamfights FOR VALUES WITH (MODULUS 16, REMAINDER 15);

-- =====================================================
-- Match advantages (HASH partitioned, 8 parts)
-- =====================================================
CREATE TABLE IF NOT EXISTS match_advantages (
    match_id            BIGINT NOT NULL,
    radiant_gold_adv    INTEGER[],
    radiant_xp_adv      INTEGER[],
    PRIMARY KEY (match_id)
) PARTITION BY HASH (match_id);

CREATE TABLE IF NOT EXISTS match_advantages_p0 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE IF NOT EXISTS match_advantages_p1 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 1);
CREATE TABLE IF NOT EXISTS match_advantages_p2 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 2);
CREATE TABLE IF NOT EXISTS match_advantages_p3 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 3);
CREATE TABLE IF NOT EXISTS match_advantages_p4 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 4);
CREATE TABLE IF NOT EXISTS match_advantages_p5 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 5);
CREATE TABLE IF NOT EXISTS match_advantages_p6 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 6);
CREATE TABLE IF NOT EXISTS match_advantages_p7 PARTITION OF match_advantages FOR VALUES WITH (MODULUS 8, REMAINDER 7);

-- =====================================================
-- Match cosmetics
-- =====================================================
CREATE TABLE IF NOT EXISTS match_cosmetics (
    match_id  BIGINT PRIMARY KEY,
    cosmetics JSONB NOT NULL
);

-- =====================================================
-- Cosmetics catalog
-- =====================================================
CREATE TABLE IF NOT EXISTS cosmetics (
    item_id             INTEGER PRIMARY KEY,
    name                TEXT,
    prefab              TEXT,
    creation_date       TIMESTAMPTZ,
    image_inventory     TEXT,
    image_path          TEXT,
    item_description    TEXT,
    item_name           TEXT,
    item_rarity         TEXT,
    item_type_name      TEXT,
    used_by_heroes      TEXT[]
);
COMMENT ON COLUMN cosmetics.used_by_heroes IS 'Array of hero identifiers.';
CREATE INDEX IF NOT EXISTS idx_cosmetics_heroes ON cosmetics USING GIN (used_by_heroes);

-- =====================================================
-- Public matches (RANGE partitioned, quarterly)
-- =====================================================
CREATE TABLE IF NOT EXISTS public_matches (
    match_id        BIGINT NOT NULL,
    start_time      BIGINT NOT NULL,
    duration        INTEGER,
    radiant_win     BOOLEAN,
    lobby_type      SMALLINT,
    game_mode       SMALLINT,
    avg_rank_tier   SMALLINT CHECK (avg_rank_tier IS NULL OR avg_rank_tier BETWEEN 0 AND 85),
    radiant_team    SMALLINT[],
    dire_team       SMALLINT[],
    PRIMARY KEY (match_id, start_time)
) PARTITION BY RANGE (start_time);

CREATE TABLE IF NOT EXISTS public_matches_2024_q1 PARTITION OF public_matches FOR VALUES FROM (1704067200) TO (1711929600);
CREATE TABLE IF NOT EXISTS public_matches_2024_q2 PARTITION OF public_matches FOR VALUES FROM (1711929600) TO (1719792000);
CREATE TABLE IF NOT EXISTS public_matches_2024_q3 PARTITION OF public_matches FOR VALUES FROM (1719792000) TO (1727740800);
CREATE TABLE IF NOT EXISTS public_matches_2024_q4 PARTITION OF public_matches FOR VALUES FROM (1727740800) TO (1735689600);
CREATE TABLE IF NOT EXISTS public_matches_2025_q1 PARTITION OF public_matches FOR VALUES FROM (1735689600) TO (1743465600);
CREATE TABLE IF NOT EXISTS public_matches_2025_q2 PARTITION OF public_matches FOR VALUES FROM (1743465600) TO (1751328000);
CREATE TABLE IF NOT EXISTS public_matches_2025_q3 PARTITION OF public_matches FOR VALUES FROM (1751328000) TO (1759276800);
CREATE TABLE IF NOT EXISTS public_matches_2025_q4 PARTITION OF public_matches FOR VALUES FROM (1759276800) TO (1767225600);
CREATE TABLE IF NOT EXISTS public_matches_2026_q1 PARTITION OF public_matches FOR VALUES FROM (1767225600) TO (1775001600);
CREATE TABLE IF NOT EXISTS public_matches_2026_q2 PARTITION OF public_matches FOR VALUES FROM (1775001600) TO (1782864000);
CREATE TABLE IF NOT EXISTS public_matches_2026_q3 PARTITION OF public_matches FOR VALUES FROM (1782864000) TO (1790812800);
CREATE TABLE IF NOT EXISTS public_matches_2026_q4 PARTITION OF public_matches FOR VALUES FROM (1790812800) TO (1798761600);
CREATE TABLE IF NOT EXISTS public_matches_2027_q1 PARTITION OF public_matches FOR VALUES FROM (1798761600) TO (1806624000);
CREATE TABLE IF NOT EXISTS public_matches_2027_q2 PARTITION OF public_matches FOR VALUES FROM (1806624000) TO (1814486400);
CREATE TABLE IF NOT EXISTS public_matches_2027_q3 PARTITION OF public_matches FOR VALUES FROM (1814486400) TO (1822435200);
CREATE TABLE IF NOT EXISTS public_matches_2027_q4 PARTITION OF public_matches FOR VALUES FROM (1822435200) TO (1830384000);
CREATE TABLE IF NOT EXISTS public_matches_default PARTITION OF public_matches DEFAULT;

CREATE INDEX IF NOT EXISTS idx_public_matches_rank_time ON public_matches(avg_rank_tier, start_time DESC) WHERE avg_rank_tier IS NOT NULL;
CREATE INDEX IF NOT EXISTS brin_public_matches_start_time ON public_matches USING BRIN (start_time) WITH (pages_per_range = 32);

SELECT apply_storage_params(
    'public_matches',
    'autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01'
);

-- =====================================================
-- Player timeseries (per-minute expanded, RANGE partitioned)
-- =====================================================
CREATE TABLE IF NOT EXISTS player_timeseries (
    match_id    BIGINT   NOT NULL,
    player_slot SMALLINT NOT NULL,
    minute      SMALLINT NOT NULL,
    start_time  BIGINT   NOT NULL,
    hero_id     SMALLINT NOT NULL,
    account_id  BIGINT,
    patch_id    INTEGER,
    gold        INTEGER,
    xp          INTEGER,
    lh          SMALLINT,
    dn          SMALLINT,
    PRIMARY KEY (match_id, player_slot, minute, start_time)
) PARTITION BY RANGE (start_time);

CREATE TABLE IF NOT EXISTS player_timeseries_2024_q1 PARTITION OF player_timeseries FOR VALUES FROM (1704067200) TO (1711929600);
CREATE TABLE IF NOT EXISTS player_timeseries_2024_q2 PARTITION OF player_timeseries FOR VALUES FROM (1711929600) TO (1719792000);
CREATE TABLE IF NOT EXISTS player_timeseries_2024_q3 PARTITION OF player_timeseries FOR VALUES FROM (1719792000) TO (1727740800);
CREATE TABLE IF NOT EXISTS player_timeseries_2024_q4 PARTITION OF player_timeseries FOR VALUES FROM (1727740800) TO (1735689600);

CREATE TABLE IF NOT EXISTS player_timeseries_2025_q1 PARTITION OF player_timeseries FOR VALUES FROM (1735689600) TO (1743465600);
CREATE TABLE IF NOT EXISTS player_timeseries_2025_q2 PARTITION OF player_timeseries FOR VALUES FROM (1743465600) TO (1751328000);
CREATE TABLE IF NOT EXISTS player_timeseries_2025_q3 PARTITION OF player_timeseries FOR VALUES FROM (1751328000) TO (1759276800);
CREATE TABLE IF NOT EXISTS player_timeseries_2025_q4 PARTITION OF player_timeseries FOR VALUES FROM (1759276800) TO (1767225600);

CREATE TABLE IF NOT EXISTS player_timeseries_2026_q1 PARTITION OF player_timeseries FOR VALUES FROM (1767225600) TO (1775001600);
CREATE TABLE IF NOT EXISTS player_timeseries_2026_q2 PARTITION OF player_timeseries FOR VALUES FROM (1775001600) TO (1782864000);
CREATE TABLE IF NOT EXISTS player_timeseries_2026_q3 PARTITION OF player_timeseries FOR VALUES FROM (1782864000) TO (1790812800);
CREATE TABLE IF NOT EXISTS player_timeseries_2026_q4 PARTITION OF player_timeseries FOR VALUES FROM (1790812800) TO (1798761600);

CREATE TABLE IF NOT EXISTS player_timeseries_2027_q1 PARTITION OF player_timeseries FOR VALUES FROM (1798761600) TO (1806624000);
CREATE TABLE IF NOT EXISTS player_timeseries_2027_q2 PARTITION OF player_timeseries FOR VALUES FROM (1806624000) TO (1814486400);
CREATE TABLE IF NOT EXISTS player_timeseries_2027_q3 PARTITION OF player_timeseries FOR VALUES FROM (1814486400) TO (1822435200);
CREATE TABLE IF NOT EXISTS player_timeseries_2027_q4 PARTITION OF player_timeseries FOR VALUES FROM (1822435200) TO (1830384000);
CREATE TABLE IF NOT EXISTS player_timeseries_default PARTITION OF player_timeseries DEFAULT;

CREATE INDEX IF NOT EXISTS idx_player_timeseries_account ON player_timeseries (account_id, match_id) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_player_timeseries_hero ON player_timeseries (hero_id, minute);
CREATE INDEX IF NOT EXISTS idx_player_timeseries_patch ON player_timeseries (patch_id, minute) WHERE patch_id IS NOT NULL;

SELECT apply_storage_params(
    'player_timeseries',
    'autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01'
);

-- =====================================================
-- Job queue & migration log
-- =====================================================
CREATE TABLE IF NOT EXISTS job_queue (
    id              BIGSERIAL PRIMARY KEY,
    type            TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT DEFAULT 'pending',
    priority        SMALLINT DEFAULT 0,
    attempts        INT DEFAULT 0,
    last_error      TEXT,
    locked_at      TIMESTAMPTZ,
    locked_by      TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_job_queue_status_priority ON job_queue(status, priority, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_job_queue_locked ON job_queue(locked_by, locked_at) WHERE locked_by IS NOT NULL;

CREATE TABLE IF NOT EXISTS migration_log (
    source_match_id BIGINT PRIMARY KEY,
    status          TEXT NOT NULL,
    error           TEXT,
    migrated_at     TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_migration_log_failed ON migration_log(migrated_at DESC) WHERE status <> 'ok';

-- =====================================================
-- Ingest outcomes (dedicated audit log)
-- =====================================================
CREATE TABLE IF NOT EXISTS ingest_outcomes (
    id              BIGSERIAL PRIMARY KEY,
    match_id        BIGINT NOT NULL,
    status         TEXT NOT NULL,
    note           TEXT,
    created_at     TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ingest_outcomes_match_id ON ingest_outcomes(match_id);
CREATE INDEX IF NOT EXISTS idx_ingest_outcomes_created_at ON ingest_outcomes(created_at DESC);

-- =====================================================
-- Partition Health View
-- =====================================================
CREATE OR REPLACE VIEW v_default_partition_health AS
SELECT 'matches_default'           AS partition, (SELECT COUNT(*) FROM matches_default)           AS rows UNION ALL
SELECT 'player_matches_default'    AS partition, (SELECT COUNT(*) FROM player_matches_default)    AS rows UNION ALL
SELECT 'public_matches_default'    AS partition, (SELECT COUNT(*) FROM public_matches_default)    AS rows UNION ALL
SELECT 'player_timeseries_default' AS partition, (SELECT COUNT(*) FROM player_timeseries_default) AS rows;

COMMENT ON VIEW v_default_partition_health IS 'Alert if any row > 0; indicates missing time-range partitions.';

-- =====================================================
-- Partition Maintenance Helpers
-- =====================================================
CREATE OR REPLACE FUNCTION ensure_time_partition(parent_table TEXT, y INT, q INT)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
    q_start BIGINT;
    q_end   BIGINT;
    part_name TEXT;
BEGIN
    IF q < 1 OR q > 4 THEN
        RAISE EXCEPTION 'quarter must be in 1..4, got %', q;
    END IF;

    q_start := EXTRACT(EPOCH FROM make_timestamptz(y, (q-1)*3 + 1, 1, 0, 0, 0, 'UTC'))::BIGINT;
    IF q = 4 THEN
        q_end := EXTRACT(EPOCH FROM make_timestamptz(y+1, 1, 1, 0, 0, 0, 'UTC'))::BIGINT;
    ELSE
        q_end := EXTRACT(EPOCH FROM make_timestamptz(y, q*3 + 1, 1, 0, 0, 0, 'UTC'))::BIGINT;
    END IF;

    part_name := format('%s_%s_q%s', parent_table, y, q);

    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = part_name) THEN
        RETURN;
    END IF;

    EXECUTE format(
        'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%s) TO (%s)',
        part_name, parent_table, q_start, q_end
    );

    IF parent_table IN ('matches', 'player_matches', 'public_matches', 'player_timeseries') THEN
        EXECUTE format(
            'ALTER TABLE %I SET (autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01)',
            part_name
        );
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION ensure_future_time_partitions(parents TEXT[], quarters_ahead INT DEFAULT 4)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE
    now_ts TIMESTAMPTZ := NOW();
    cur_y  INT := EXTRACT(YEAR    FROM now_ts)::INT;
    cur_q  INT := ((EXTRACT(MONTH FROM now_ts)::INT - 1) / 3) + 1;
    i INT; y INT; q INT; p TEXT;
BEGIN
    FOREACH p IN ARRAY parents LOOP
        FOR i IN 0..quarters_ahead LOOP
            y := cur_y; q := cur_q + i;
            WHILE q > 4 LOOP
                q := q - 4; y := y + 1;
            END LOOP;
            PERFORM ensure_time_partition(p, y, q);
        END LOOP;
    END LOOP;
END;
$$;

-- =====================================================
-- Automation: updated_at triggers
-- =====================================================
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- =====================================================
-- Immutability: forbid start_time updates on time-range partitioned tables
-- =====================================================
CREATE OR REPLACE FUNCTION raise_immutable_column()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'column % is immutable in table %', TG_ARGV[0], TG_TABLE_NAME;
END;
$$;

DO $$
DECLARE
    tbl TEXT[] := ARRAY['matches', 'player_matches', 'public_matches', 'player_timeseries'];
    t TEXT;
BEGIN
    FOREACH t IN ARRAY tbl LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%s_no_start_time_update ON %s', t, t);
        EXECUTE format('CREATE TRIGGER trg_%1$s_no_start_time_update BEFORE UPDATE OF start_time ON %1$s FOR EACH ROW WHEN (OLD.start_time IS DISTINCT FROM NEW.start_time) EXECUTE FUNCTION raise_immutable_column(''start_time'')', t);
    END LOOP;
END $$;

DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT c.table_schema, c.table_name
        FROM information_schema.columns c
        JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
        WHERE c.column_name = 'updated_at' AND c.table_schema = 'public' AND t.table_type = 'BASE TABLE'
          AND NOT EXISTS (
              SELECT 1 FROM pg_inherits i
              JOIN pg_class ch ON ch.oid = i.inhrelid
              JOIN pg_namespace n ON n.oid = ch.relnamespace
              WHERE n.nspname = c.table_schema AND ch.relname = c.table_name
          )
    LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%2$s_set_updated_at ON %1$I.%2$I', r.table_schema, r.table_name);
        EXECUTE format('CREATE TRIGGER trg_%2$s_set_updated_at BEFORE UPDATE ON %1$I.%2$I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', r.table_schema, r.table_name);
    END LOOP;
END $$;

-- =====================================================
-- Attach hero-stub safety-net triggers to FK-bound tables
-- (consolidated from 002_fix_hero_fk_stubs.sql)
-- =====================================================
DO $$
DECLARE
    t TEXT;
    tbls TEXT[] := ARRAY['draft_timings', 'picks_bans', 'player_matches'];
BEGIN
    FOREACH t IN ARRAY tbls LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%s_hero_stub ON %I', t, t);
        EXECUTE format(
            'CREATE TRIGGER trg_%1$s_hero_stub
             BEFORE INSERT ON %1$I
             FOR EACH ROW
             EXECUTE FUNCTION trg_ensure_hero_stub()',
            t
        );
    END LOOP;
END $$;
