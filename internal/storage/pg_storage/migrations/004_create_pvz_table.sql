CREATE TABLE IF NOT EXISTS pvz
(
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id         UUID             DEFAULT NULL,
    city              cities,
    registration_date TIMESTAMP        DEFAULT NOW(),
    FOREIGN KEY (author_id) REFERENCES clients (id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);
