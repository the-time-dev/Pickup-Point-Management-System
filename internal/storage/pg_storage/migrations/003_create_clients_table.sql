CREATE TABLE IF NOT EXISTS clients
(
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(60)         NOT NULL,
    moderator     BOOL                NOT NULL,
    employee      BOOL                NOT NULL,
    created_at    TIMESTAMP        DEFAULT NOW()
);
