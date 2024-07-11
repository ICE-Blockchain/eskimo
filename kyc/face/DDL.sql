-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS users_forwarded_to_face_kyc
(
    forwarded_at     TIMESTAMP NOT NULL,
    user_id          TEXT    not null references users (id) ON DELETE CASCADE,
    original_account TEXT NOT NULL DEFAULT '',
    primary key (user_id)
);
CREATE INDEX IF NOT EXISTS users_forwarded_to_face_kyc_forwarded_at_ix ON users_forwarded_to_face_kyc (forwarded_at);
ALTER TABLE users_forwarded_to_face_kyc ADD COLUMN IF NOT EXISTS original_account TEXT NOT NULL DEFAULT '';