CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    category    TEXT    NOT NULL,
    price_baht  INTEGER NOT NULL CHECK (price_baht > 0),
    available   BOOLEAN NOT NULL DEFAULT true,
    sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id),
    status      TEXT        NOT NULL DEFAULT 'pending_payment'
                CHECK (status IN ('pending_payment','paid','preparing','ready','completed','cancelled')),
    total_baht  INTEGER     NOT NULL CHECK (total_baht > 0),
    note        TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at     TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_status_idx ON orders (status, created_at);

-- Name and price are copied at order time so later menu changes never rewrite history.
CREATE TABLE order_items (
    id              BIGSERIAL PRIMARY KEY,
    order_id        BIGINT  NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id      BIGINT  NOT NULL REFERENCES products(id),
    name            TEXT    NOT NULL,
    unit_price_baht INTEGER NOT NULL,
    qty             INTEGER NOT NULL CHECK (qty > 0)
);

CREATE INDEX order_items_order_idx ON order_items (order_id);
