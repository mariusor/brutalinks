-- name: drop-tables
drop table if exists content_items;
drop table if exists accounts;
drop table if exists votes;

-- name: create-accounts
create table accounts (
  id serial primary key,
  key char(64) unique,
  handle varchar,
  email varchar unique,
  score bigint default 0,
  created_at timestamp default current_timestamp,
  updated_at timestamp default current_timestamp,
  metadata jsonb default '{}',
  flags bit(8) default 0::bit(8)
);

-- name: create-items
create table content_items (
  id serial primary key,
  key char(64) unique,
  mime_type varchar default NULL,
  title varchar default NULL,
  data text default NULL,
  score bigint default 0,
  path ltree default NULL,
  submitted_by int references accounts(id),
  submitted_at timestamp default current_timestamp,
  updated_at timestamp default current_timestamp,
  metadata jsonb default '{}',
  flags bit(8) default 0::bit(8)
);

-- name: create-votes
create table votes (
  id serial primary key,
  submitted_by int references accounts(id),
  submitted_at timestamp default current_timestamp,
  updated_at timestamp default current_timestamp,
  item_id  int references content_items(id),
  weight int,
  flags bit(8) default 0::bit(8),
  constraint unique_vote_submitted_item unique (submitted_by, item_id)
);
