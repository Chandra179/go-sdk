-- name: GetUserByProviderAndSubject :one
-- Retrieve a user by OAuth provider and subject ID
SELECT id, provider, subject_id, email, full_name, created_at, updated_at
FROM users
WHERE provider = $1 AND subject_id = $2;

-- name: CreateUser :one
-- Insert a new user into the database
INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, provider, subject_id, email, full_name, created_at, updated_at;

-- name: GetOrCreateUser :one
-- Insert a new user or return existing user if duplicate key violation occurs
-- This uses ON CONFLICT to handle the unique constraint on (provider, subject_id)
INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
ON CONFLICT (provider, subject_id) DO UPDATE SET
    updated_at = EXCLUDED.updated_at
RETURNING id, provider, subject_id, email, full_name, created_at, updated_at;

-- name: GetUserByID :one
-- Retrieve a user by their unique ID
SELECT id, provider, subject_id, email, full_name, created_at, updated_at
FROM users
WHERE id = $1;

-- name: UpdateUser :one
-- Update an existing user's information
UPDATE users
SET email = $2,
    full_name = $3,
    updated_at = $4
WHERE id = $1
RETURNING id, provider, subject_id, email, full_name, created_at, updated_at;

-- name: DeleteUser :exec
-- Remove a user from the database
DELETE FROM users
WHERE id = $1;