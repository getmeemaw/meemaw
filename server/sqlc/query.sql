-- name: Status :one
SELECT 1;

------- INSERTS -------

-- name: AddUser :one
INSERT INTO users (foreign_key)
VALUES ($1)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: AddWallet :one
INSERT INTO wallets (user_id, public_address, encrypted_dkg_results, nonce)
VALUES (sqlc.arg('UserId'), sqlc.arg('PublicAddress'), sqlc.arg('EncryptedDkgResults'), sqlc.arg('Nonce'))
ON CONFLICT DO NOTHING
RETURNING *;

-- name: AddDevice :one
INSERT INTO devices (user_id, wallet_id, user_agent)
VALUES (sqlc.arg('UserId'), sqlc.arg('WalletId'), sqlc.arg('UserAgent'))
ON CONFLICT DO NOTHING
RETURNING *;

------- SELECTS -------

-- name: GetFirstUser :one
SELECT * FROM users
LIMIT 1;

-- name: GetUserByForeignKey :one
SELECT * FROM users
WHERE foreign_key = sqlc.arg('ForeignKey')
LIMIT 1;

-- name: GetUserWallets :many
SELECT * FROM wallets
WHERE user_id = sqlc.arg('UserId');

-- name: GetUserDevices :many
SELECT * FROM devices
WHERE user_id = sqlc.arg('UserId');

-- name: GetWalletByAddress :one
SELECT * FROM wallets
WHERE public_address = sqlc.arg('PublicAddress');

-- name: GetUserByAddress :one
SELECT users.* 
FROM wallets
INNER JOIN users ON wallets.user_id = users.id
WHERE wallets.public_address = sqlc.arg('PublicAddress');

-- name: GetUserSigningParameters :one
SELECT wallets.*
FROM users
LEFT JOIN wallets ON users.id = wallets.user_id
WHERE users.foreign_key = sqlc.arg('ForeignKey'); -- will need to add this eventually