CREATE TABLE IF NOT EXISTS files (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    size int NOT NULL,
    path text UNIQUE NOT NULL,
    code text UNIQUE NOT NULL,
    expiry timestamp(0) with time zone NOT NULL,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    last_updated timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    user_id bigint NOT NULL REFERENCES users ON DELETE CASCADE
);