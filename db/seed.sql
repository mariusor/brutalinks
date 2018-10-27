-- name: add-account-system
insert into accounts (id, key, handle, email) values (
    -1,
    'dc6f5f5bf55bc1073715c98c69fa7ca8',
    'system',
    'system@localhost'
);

-- name: add-account-anonymous
insert into accounts (id, key, handle, email) values (
    0,
    'eacff9ddf379bd9fc8274c5a9f4cae08',
    'anonymous',
    'anonymous@localhost'
);

-- name: add-item-about
insert into content_items (key, mime_type, title, data, submitted_by) values (
    '162edb32c80d0e6dd3114fbb59d6273b',
    'text/html',
    'about littr.me',
    '<p>' ||
    'This is a new attempt at the social news aggregator paradigm.<br/>' ||
    'It''s based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.' ||
    '</p>',
    -1
);
