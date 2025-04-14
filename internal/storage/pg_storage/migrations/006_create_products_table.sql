CREATE TABLE IF NOT EXISTS products
(
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id         UUID             DEFAULT NULL,
    reception_id      UUID             DEFAULT NULL,
    product_type      product_types,
    registration_date TIMESTAMP        DEFAULT NOW(),
    FOREIGN KEY (author_id) REFERENCES clients (id)
        ON DELETE CASCADE
        ON UPDATE CASCADE,
    FOREIGN KEY (reception_id) REFERENCES receptions (id)
        ON DELETE CASCADE
        ON UPDATE CASCADE
);
