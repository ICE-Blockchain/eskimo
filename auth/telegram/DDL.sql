-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS telegram_sign_ins (
           created_at                             timestamp NOT NULL,
           token_issued_at                        timestamp,
           issued_token_seq                       BIGINT DEFAULT 0 NOT NULL,
           previously_issued_token_seq            BIGINT DEFAULT 0 NOT NULL,
           telegram_user_id                       text NOT NULL,
           telegram_bot_id                        text NOT NULL,
           user_id                                TEXT,
           primary key(telegram_user_id))
           WITH (FILLFACTOR = 70);
CREATE INDEX IF NOT EXISTS telegram_sign_ins ON telegram_sign_ins (user_id);
ALTER TABLE telegram_sign_ins ADD COLUMN IF NOT EXISTS telegram_bot_id text NOT NULL DEFAULT '';