CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    foreign_key text NOT NULL DEFAULT ''
);

-- CREATE UNIQUE INDEX user_identifier ON public.users USING btree (foreign_key);

CREATE TABLE wallets (
    id BIGSERIAL PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    public_address text NOT NULL DEFAULT '',
    encrypted_dkg_results bytea NOT NULL DEFAULT E'\\x',
    nonce bytea NOT NULL DEFAULT E'\\x'
);

-- CREATE UNIQUE INDEX wallet_identifier ON public.wallets USING btree (user_id, public_address);

CREATE TABLE devices (
    id BIGSERIAL PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    wallet_id bigint NOT NULL REFERENCES wallets(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    user_agent text NOT NULL DEFAULT ''
);

-- CREATE UNIQUE INDEX device_identifier ON public.devices USING btree (user_id, wallet_id, user_agent);