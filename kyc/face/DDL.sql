-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS users_forwarded_to_face_kyc
(
    forwarded_at   TIMESTAMP NOT NULL,
    user_id        TEXT    not null references users (id) ON DELETE CASCADE,
    face_id        TEXT NOT NULL,
    primary key (user_id)
);

ALTER TABLE users_forwarded_to_face_kyc
    ADD COLUMN IF NOT EXISTS face_id TEXT;

CREATE INDEX IF NOT EXISTS users_forwarded_to_face_kyc_forwarded_at_ix ON users_forwarded_to_face_kyc (forwarded_at);