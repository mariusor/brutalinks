-- name: add-account-system
INSERT INTO "accounts" ("id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags") VALUES (
    -1,
    'dc6f5f5bf55bc1073715c98c69fa7ca8',
    'system',
    'system@localhost',
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT
);

-- name: add-account-anonymous
INSERT INTO "accounts" ("id", "key", "handle", "email", "score", "created_at", "updated_at", "metadata", "flags") VALUES (
    0,
    'eacff9ddf379bd9fc8274c5a9f4cae08',
    'anonymous',
    'anonymous@localhost',
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT
);

-- name: add-item-about
INSERT INTO "content_items" ("id", "key", "mime_type", "title", "data", "submitted_by", "score", "submitted_at", "updated_at", "path", "metadata", "flags") VALUES (
    DEFAULT,
    '162edb32c80d0e6dd3114fbb59d6273b',
    'text/html',
    'about littr.me',
    '<p>' ||
    'This is a new attempt at the social news aggregator paradigm.<br/>' ||
    'It''s based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.' ||
    '</p>',
    -1,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT,
    DEFAULT
);

-- name: add-local-instance
INSERT INTO "instances" ("id", "name", "description", "url", "inbox", "metadata", "flags") VALUES (
  0,
  'littr.me',
  'Link aggregator inspired by Reddit and HackerNews using ActivityPub federation.',
  'http://littr.git',
  'http://littr.git/api/self/inbox',
  DEFAULT,
  DEFAULT
);
