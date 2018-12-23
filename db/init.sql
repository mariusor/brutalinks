-- name: drop-tables
drop table if exists items;
drop table if exists accounts;
drop table if exists votes;
drop table if exists instances;

-- name: create-accounts
create table accounts (
  id serial constraint accounts_pk primary key,
  key char(32) unique,
  handle varchar,
  email varchar unique,
  score bigint default 0,
  created_at timestamp default current_timestamp,
  updated_at timestamp default current_timestamp,
  metadata jsonb default '{}',
  flags bit(8) default 0::bit(8)
);

-- name: create-items
create table items (
  id serial constraint items_pk primary key,
  key char(32) unique,
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
  id serial constraint votes_pk primary key,
  submitted_by int references accounts(id),
  submitted_at timestamp default current_timestamp,
  updated_at timestamp default current_timestamp,
  item_id  int references items(id),
  weight int,
  flags bit(8) default 0::bit(8),
  constraint unique_vote_submitted_item unique (submitted_by, item_id)
);

-- name: create-instances
create table instances
(
  id serial constraint instances_pk primary key,
  name varchar not null,
  description text,
  url varchar unique not null,
  inbox varchar unique,
  metadata jsonb default '{}',
  flags bit(8) default 0::bit(8)
);
