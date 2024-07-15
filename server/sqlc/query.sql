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
INSERT INTO devices (user_id, wallet_id, user_agent, peer_id)
VALUES (sqlc.arg('UserId'), sqlc.arg('WalletId'), sqlc.arg('UserAgent'), sqlc.arg('PeerId'))
ON CONFLICT DO NOTHING
RETURNING *;

-- name: Dkg :one
WITH new_user AS (
    INSERT INTO users (foreign_key)
    VALUES (sqlc.arg('ForeignKey'))
    ON CONFLICT DO NOTHING
    RETURNING id AS user_id
),
new_wallet AS (
    INSERT INTO wallets (user_id, public_address, encrypted_dkg_results, nonce)
    SELECT user_id, sqlc.arg('PublicAddress'), sqlc.arg('EncryptedDkgResults'), sqlc.arg('Nonce')
    FROM new_user
    ON CONFLICT DO NOTHING
    RETURNING id AS wallet_id, user_id
)
INSERT INTO devices (user_id, wallet_id, user_agent, peer_id)
SELECT user_id, wallet_id, sqlc.arg('UserAgent'), sqlc.arg('PeerId')
FROM new_wallet
ON CONFLICT DO NOTHING
RETURNING *;

-- name: AddPeer :one
WITH existing_user AS (
    SELECT id AS user_id
    FROM users
    WHERE foreign_key = sqlc.arg('ForeignKey')
),
updated_wallet AS (
    UPDATE wallets
    SET encrypted_dkg_results = sqlc.arg('EncryptedDkgResults'),
        nonce = sqlc.arg('Nonce')
    FROM existing_user
    WHERE wallets.user_id = existing_user.user_id
    RETURNING wallets.id AS wallet_id, wallets.user_id
)
INSERT INTO devices (user_id, wallet_id, user_agent, peer_id)
SELECT user_id, wallet_id, sqlc.arg('UserAgent'), sqlc.arg('PeerId')
FROM updated_wallet
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