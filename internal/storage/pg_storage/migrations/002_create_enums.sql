DO
$$
    BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'cities') THEN
            CREATE TYPE cities AS ENUM ('Москва', 'Санкт-Петербург', 'Казань');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'product_types') THEN
            CREATE TYPE product_types AS ENUM ('электроника', 'одежда', 'обувь');
        END IF;
    END
$$;
