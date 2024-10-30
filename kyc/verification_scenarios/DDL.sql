-- SPDX-License-Identifier: ice License 1.0
CREATE TABLE IF NOT EXISTS verification_distribution_kyc_unsuccessful_attempts  (
                    created_at                timestamp NOT NULL,
                    reason                    text      NOT NULL,
                    user_id                   text      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                    social                    text      NOT NULL CHECK (social = 'twitter' OR social = 'cmc'),
                    PRIMARY KEY (user_id, created_at, social));

