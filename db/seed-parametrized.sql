-- name: test-accounts
INSERT INTO "accounts"
  ("id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags")
VALUES (
    $1,
    $2,
    $3,
    $4,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $5,
    DEFAULT
);

-- name: test-items
INSERT INTO "items"
  ("id", "key", "mime_type", "title", "data", "submitted_by", "score", "submitted_at", "updated_at", "path", "metadata", "flags")
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
INSERT INTO "votes"
  ("id", "submitted_by", "submitted_at", "updated_at", "item_id", "weight", "flags")
VALUES (
    DEFAULT,
    $1,
    DEFAULT,
    DEFAULT,
    $2,
    $3,
    DEFAULT
);

-- name: test-instances
INSERT INTO "instances"
  ("id", "name", "description", "url", "inbox", "metadata", "flags")
VALUES (
  $1,
  $2,
  $3,
  $4,
  $5,
  $6,
  DEFAULT
);

-- name: test-oauth-clients
INSERT INTO "client"
  ("id", "secret", "extra", "redirect_uri")
VALUES (
  $1,
  $2,
  $3,
  $4
);
