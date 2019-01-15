-- name: create-role-with-pass
CREATE ROLE "%s" LOGIN PASSWORD '%s';
-- name: create-db-for-role
CREATE DATABASE "%s" OWNER "%s";
