CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE places (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    category        TEXT NOT NULL,
    geom            geography(Point, 4326) NOT NULL,
    wikidata_qid    TEXT,
    source          TEXT NOT NULL,
    source_richness TEXT,
    status          TEXT NOT NULL DEFAULT 'active',
    removed_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX places_geom_idx ON places USING GIST (geom);

CREATE TABLE scripts (
    id           TEXT PRIMARY KEY,
    place_id     TEXT NOT NULL REFERENCES places(id),
    language     TEXT NOT NULL,
    text         TEXT NOT NULL,
    source_text  TEXT,
    status       TEXT NOT NULL DEFAULT 'draft',
    reviewer_id  TEXT REFERENCES users(id),
    reviewed_at  TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (place_id, language)
);

CREATE TABLE audio_files (
    id             TEXT PRIMARY KEY,
    script_id      TEXT NOT NULL REFERENCES scripts(id),
    voice_id       TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'queued',
    storage_url    TEXT,
    timestamps_url TEXT,
    duration_ms    BIGINT,
    failure_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
