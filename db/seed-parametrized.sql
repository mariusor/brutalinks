-- name: test-accounts
INSERT INTO "accounts" ("id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags")
VALUES (
    DEFAULT,
    $1,
    $2,
    $3,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $4,
    DEFAULT
);

-- name: test-items
INSERT INTO "items" ("id", "key", "mime_type", "title", "data", "submitted_by", "score", "submitted_at", "updated_at", "path", "metadata", "flags")
VALUES (
    DEFAULT,
    $1,
    $2,
    $3,
    $4,
    $5,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $6,
    $7,
    DEFAULT
);

-- name: test-votes
INSERT INTO public.votes (id, submitted_by, submitted_at, updated_at, item_id, weight, flags)
VALUES (
    DEFAULT,
    $1,
    DEFAULT,
    DEFAULT,
    $2,
    $3,
    DEFAULT
);
