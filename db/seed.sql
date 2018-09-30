-- name: add-account-system
insert into accounts (id, key, handle, email) values (
    -1,
    '29fc2269252dd76fa7e4b6d193f51a3f3cd21fdf30e44f34ec138d7e803cf0c3',
    'system',
    'system@localhost'
);

-- name: add-account-anonymous
insert into accounts (id, key, handle, email) values (
    0,
    '77b7b7215e8d78452dc40da9efbb65fdc918c757844387aa0f88143762495c6b',
    'anonymous',
    'anonymous@localhost'
);

-- name: add-item-about
insert into content_items (key, mime_type, title, data, submitted_by) values (
    'cb615f8863b197b86a08354911b93c0fc3d365061a83bb6482f8ac67c871d192',
    'text/html',
    'about littr.me',
    '<p>' ||
    'This is a new attempt at the social news aggregator paradigm.<br/>' ||
    'It''s based on the ActivityPub web specification and as such tries to leverage federation to prevent some of the pitfalls found in similar existing communities.' ||
    '</p>',
    -1
);
