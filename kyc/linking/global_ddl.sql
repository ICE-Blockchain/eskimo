-- SPDX-License-Identifier: ice License 1.0

CREATE TABLE IF NOT EXISTS linked_user_accounts
(
    linked_at      TIMESTAMP NOT NULL,
    tenant         TEXT NOT NULL,
    user_id        TEXT NOT NULL,
    linked_tenant  TEXT NOT NULL,
    linked_user_id TEXT NOT NULL,
    has_kyc        BOOLEAN NOT NULL DEFAULT false,
    primary key (user_id, linked_user_id, tenant, linked_tenant)
);

CREATE INDEX IF NOT EXISTS linked_user_accounts_linked_user_id ON linked_user_accounts (linked_user_id);