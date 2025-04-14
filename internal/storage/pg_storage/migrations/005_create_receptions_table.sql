CREATE TABLE IF NOT EXISTS receptions
(
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id         UUID             DEFAULT NULL,
    pvz_id            UUID             DEFAULT NULL,
    activity          BOOL             DEFAULT TRUE,
    registration_date TIMESTAMP        DEFAULT NOW(),
    FOREIGN KEY (author_id) REFERENCES clients (id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,
    FOREIGN KEY (pvz_id) REFERENCES pvz (id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);
