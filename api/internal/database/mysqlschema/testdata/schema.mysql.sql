-- NginxProxyGuard — MySQL schema, derived from the PostgreSQL schema.
--
-- GENERATED, do not edit. Source: migrations/001_init.sql followed by the
-- upgrade list in database/migration.go, replayed by internal/database/mysqlschema.
-- Regenerate: go test ./internal/database/mysqlschema -run Golden -update
--
-- The running server does not read this file: database.runMySQLMigrations
-- builds the same statements at boot and Sync applies whatever is missing.

-- ---------------------------------------------------------------------------
-- Left out, and why
-- ---------------------------------------------------------------------------
-- DO block [Detach orphan chunk inheritance entries (TimescaleDB catalog drift)]
--     left out 1 dynamic EXECUTE (TimescaleDB chunks/partitions/legacy view)
-- DO block [logs_unified view rebuild]
--     left out 2 dynamic EXECUTE (TimescaleDB chunks/partitions/legacy view)
-- DO block [redact URL-borne credentials left in certificate and notification errors]
--     left out 8 data statement(s), a repair of PostgreSQL install history (UPDATE public.certificates SET error_message = regexp_replace(error_message, shapes, kept,...)
-- DO block [compress logs after 1 day (one-shot; only installs still on the 7-day default)]
--     left out 1 data statement(s), a repair of PostgreSQL install history (INSERT INTO schema_migrations(version) VALUES('logs_compress_after_1d_v1') ON CONFLICT DO ...)
-- DO block [raw logs: retention in days takes over from the rotated-file count]
--     left out 2 data statement(s), a repair of PostgreSQL install history (UPDATE public.system_settings SET raw_log_retention_days = GREATEST(raw_log_retention_days...)
-- DO block (applied: ALTER TABLE public.logs_partitioned ALTER COLUMN body_bytes_sent TYPE BIGINT) [001_init.sql]
--     left out 3 dynamic EXECUTE (TimescaleDB chunks/partitions/legacy view)
-- DO block (applied: ALTER TABLE public.logs_partitioned ALTER COLUMN rule_id TYPE BIGINT) [001_init.sql]
--     left out 3 dynamic EXECUTE (TimescaleDB chunks/partitions/legacy view)
-- DO block (applied: ALTER TABLE public.proxy_hosts ADD CONSTRAINT proxy_hosts_access_list_id_fkey FOREIGN KEY(...) [v1.3.31: proxy_hosts_access_list_id_fkey]
--     left out 1 data statement(s), a repair of PostgreSQL install history (UPDATE public.proxy_hosts SET access_list_id = NULL WHERE access_list_id IS NOT NULL AND a...)
-- extension pg_trgm [001_init.sql]
--     PostgreSQL extension: UUID() and the MySQL schema need none
-- extension uuid-ossp [001_init.sql]
--     PostgreSQL extension: UUID() and the MySQL schema need none
-- function cleanup_expired_challenge_tokens [001_init.sql]
--     PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch
-- function cleanup_old_logs [001_init.sql]
--     PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch
-- function cleanup_system_logs [001_init.sql]
--     PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch
-- function create_monthly_partitions [001_init.sql]
--     PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch
-- function drop_old_partitions [001_init.sql]
--     PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch
-- index idx_certificates_domain_names [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_geo_restrictions_blocked_cloud [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_logs_host_trgm [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_logs_part_block_reason_ts [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_host_trgm [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_logs_part_host_ts [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_log_type [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_status_code [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_status_ts [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_timestamp [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_type_timestamp [001_init.sql]
--     retired: NPG no longer builds it (retiredLogsPartitionedIndexes)
-- index idx_logs_part_ua_trgm [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_logs_part_uri_trgm [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_logs_request_uri_trgm [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_proxy_hosts_domain_names [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_proxy_hosts_tags [001_init.sql]
--     gin index; InnoDB has no equivalent
-- index idx_redirect_hosts_domains [001_init.sql]
--     gin index; InnoDB has no equivalent
-- table dashboard_stats_hourly_partitioned [replay]
--     transitional partitioned copy of dashboard_stats_hourly; the app reads and writes dashboard_stats_hourly
-- table logs [replay]
--     pre-partitioning log table: the app writes logs_partitioned only, and PostgreSQL keeps it solely for the logs_unified UNION, which on MySQL projects logs_partitioned alone

-- ---------------------------------------------------------------------------
-- Rendered differently from PostgreSQL
-- ---------------------------------------------------------------------------
-- access_list_items.idx_access_list_items_hostname: partial-index predicate dropped (the index covers every row)
-- api_tokens.idx_api_tokens_is_active: partial-index predicate dropped (the index covers every row)
-- banned_ips.idx_banned_ips_ip_global_unique: partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide
-- banned_ips.idx_banned_ips_ip_host_unique: partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide
-- certificates.idx_certificates_auto_renew: partial-index predicate dropped (the index covers every row)
-- dashboard_stats_hourly.idx_dashboard_stats_hourly_null_host_bucket: partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide
-- dns_providers.idx_dns_providers_default: partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide
-- filter_subscription_entries.value: text -> VARCHAR(732) (unique/primary key column)
-- filter_subscription_entry_exclusions.value: text -> VARCHAR(732) (unique/primary key column)
-- filter_subscriptions.url: text -> VARCHAR(768) (unique/primary key column)
-- global_exploit_rule_exclusions.uri_pattern: text -> VARCHAR(512) (unique/primary key column)
-- host_exploit_rule_exclusions.uri_pattern: text -> VARCHAR(512) (unique/primary key column)
-- logs_partitioned.idx_logs_partitioned_exploit_rule: partial-index predicate dropped (the index covers every row)
-- logs_partitioned.idx_logs_part_proxy_host_ts: partial-index predicate dropped (the index covers every row)
-- logs_partitioned.idx_logs_part_geo_ts: partial-index predicate dropped (the index covers every row)
-- logs_partitioned.idx_logs_part_block_reason: partial-index predicate dropped (the index covers every row)
-- logs_partitioned.idx_logs_part_block_reason_created: partial-index predicate dropped (the index covers every row)
-- proxy_hosts.idx_proxy_hosts_stream_listener_unique: partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide
-- raw_log_reclaim_chunks.chunk_name: text -> VARCHAR(768) (unique/primary key column)
-- system_logs.idx_system_logs_container: partial-index predicate dropped (the index covers every row)
-- user_identities.subject: text -> VARCHAR(732) (unique/primary key column)
-- users.idx_users_totp_enabled: partial-index predicate dropped (the index covers every row)
-- waf_rule_exclusions.scope_value: text -> VARCHAR(715) (unique/primary key column)

CREATE TABLE IF NOT EXISTS `access_list_items` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `access_list_id` CHAR(36) NOT NULL,
    `directive` VARCHAR(10) NOT NULL,
    `address` VARCHAR(255) NOT NULL,
    `description` TEXT,
    `sort_order` INT DEFAULT 0,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `hostname` VARCHAR(253),
    PRIMARY KEY (`id`),
    KEY `idx_access_list_items_hostname` (`hostname`),
    KEY `idx_access_list_items_list_id` (`access_list_id`),
    CONSTRAINT `access_list_items_directive_check` CHECK ((`directive`) IN ('allow', 'deny')),
    CONSTRAINT `access_list_items_hostname_check` CHECK (`hostname` IS NULL OR (`directive` = 'deny' AND `address` = 'all' AND REGEXP_LIKE(`hostname`, '^[a-z0-9.-]{1,253}$', 'c')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `access_lists` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(255) NOT NULL,
    `description` TEXT,
    `satisfy_any` BOOLEAN DEFAULT TRUE,
    `pass_auth` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `api_token_usage` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `token_id` CHAR(36) NOT NULL,
    `endpoint` VARCHAR(255) NOT NULL,
    `method` VARCHAR(10) NOT NULL,
    `status_code` INT,
    `client_ip` VARCHAR(45),
    `user_agent` TEXT,
    `request_body_size` BIGINT,
    `response_time_ms` INT,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_api_token_usage_created_at` (`created_at`),
    KEY `idx_api_token_usage_token_id` (`token_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `api_tokens` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `user_id` CHAR(36) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `token_hash` VARCHAR(64) NOT NULL,
    `token_prefix` VARCHAR(16) NOT NULL,
    `permissions` JSON NOT NULL DEFAULT ('["*"]'),
    `allowed_ips` TEXT,
    `rate_limit` INT DEFAULT 1000,
    `expires_at` DATETIME(6),
    `last_used_at` DATETIME(6),
    `last_used_ip` VARCHAR(45),
    `use_count` BIGINT DEFAULT 0,
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE,
    `revoked_at` DATETIME(6),
    `revoked_reason` VARCHAR(255),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `api_tokens_token_hash_key` (`token_hash`),
    KEY `idx_api_tokens_is_active` (`is_active`),
    KEY `idx_api_tokens_token_hash` (`token_hash`),
    KEY `idx_api_tokens_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `audit_logs` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `user_id` CHAR(36),
    `username` VARCHAR(255) NOT NULL,
    `action` VARCHAR(100) NOT NULL,
    `resource_type` VARCHAR(255),
    `resource_id` VARCHAR(255),
    `resource_name` VARCHAR(255),
    `details` JSON,
    `ip_address` VARCHAR(45),
    `user_agent` TEXT,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_audit_logs_action` (`action`),
    KEY `idx_audit_logs_created_at` (`created_at` DESC),
    KEY `idx_audit_logs_resource_type` (`resource_type`),
    KEY `idx_audit_logs_user_created` (`user_id`, `created_at` DESC),
    KEY `idx_audit_logs_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `auth_providers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(255) NOT NULL,
    `type` VARCHAR(20) NOT NULL DEFAULT 'custom',
    `provider_url` TEXT NOT NULL,
    `config` JSON NOT NULL DEFAULT ('{}'),
    `timeout_ms` INT NOT NULL DEFAULT 5000,
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `container_name` TEXT,
    `container_network` TEXT,
    `container_port` INT,
    `container_scheme` TEXT,
    `last_resolved_ip` TEXT,
    `last_reconcile_at` DATETIME(6),
    `last_reconcile_status` TEXT,
    `last_reconcile_error` TEXT,
    `reconcile_fail_count` INT NOT NULL DEFAULT 0,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `auth_providers_type_check` CHECK ((`type`) IN ('authelia', 'authentik', 'custom'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `auth_sessions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `user_id` CHAR(36) NOT NULL,
    `token_hash` VARCHAR(255) NOT NULL,
    `ip_address` VARCHAR(45),
    `user_agent` TEXT,
    `expires_at` DATETIME(6) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_auth_sessions_expires_at` (`expires_at`),
    KEY `idx_auth_sessions_token_hash` (`token_hash`),
    KEY `idx_auth_sessions_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `backups` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `filename` VARCHAR(255) NOT NULL,
    `file_size` BIGINT NOT NULL DEFAULT 0,
    `file_path` VARCHAR(500) NOT NULL,
    `includes_config` BOOLEAN NOT NULL DEFAULT TRUE,
    `includes_certificates` BOOLEAN NOT NULL DEFAULT TRUE,
    `includes_database` BOOLEAN NOT NULL DEFAULT TRUE,
    `backup_type` VARCHAR(20) NOT NULL DEFAULT 'manual',
    `description` TEXT,
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `error_message` TEXT,
    `checksum_sha256` VARCHAR(64),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `completed_at` DATETIME(6),
    PRIMARY KEY (`id`),
    KEY `idx_backups_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `banned_ips` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `ip_address` VARCHAR(45) NOT NULL,
    `reason` VARCHAR(255),
    `fail_count` INT DEFAULT 0,
    `banned_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `expires_at` DATETIME(6),
    `is_permanent` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `is_auto_banned` BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (`id`),
    KEY `idx_banned_ips_auto` (`is_auto_banned`, `expires_at`),
    KEY `idx_banned_ips_expires` (`expires_at`),
    UNIQUE KEY `idx_banned_ips_ip_global_unique` (((CASE WHEN (`proxy_host_id` IS NULL) THEN `ip_address` END))),
    UNIQUE KEY `idx_banned_ips_ip_host_unique` (((CASE WHEN (`proxy_host_id` IS NOT NULL) THEN `ip_address` END)), ((CASE WHEN (`proxy_host_id` IS NOT NULL) THEN `proxy_host_id` END))),
    KEY `idx_banned_ips_lookup` (`ip_address`, `expires_at`, `is_permanent`),
    KEY `idx_banned_ips_proxy_host` (`proxy_host_id`),
    KEY `idx_banned_ips_host_banned_at` (`proxy_host_id`, `banned_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `bot_filters` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `block_bad_bots` BOOLEAN DEFAULT TRUE,
    `block_ai_bots` BOOLEAN DEFAULT FALSE,
    `allow_search_engines` BOOLEAN DEFAULT TRUE,
    `custom_blocked_agents` TEXT,
    `custom_allowed_agents` TEXT,
    `challenge_suspicious` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `block_suspicious_clients` BOOLEAN DEFAULT FALSE,
    `disable_global` BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (`id`),
    UNIQUE KEY `bot_filters_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_bot_filters_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `certificate_history` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `certificate_id` CHAR(36) NOT NULL,
    `action` VARCHAR(50) NOT NULL,
    `status` VARCHAR(50) NOT NULL,
    `message` TEXT,
    `domain_names` TEXT NOT NULL,
    `provider` VARCHAR(50) NOT NULL,
    `expires_at` DATETIME(6),
    `logs` JSON,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_certificate_history_certificate_id` (`certificate_id`),
    KEY `idx_certificate_history_created_at` (`created_at` DESC),
    KEY `idx_certificate_history_action` (`action`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `certificates` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `domain_names` TEXT NOT NULL,
    `expires_at` DATETIME(6),
    `certificate_path` VARCHAR(512),
    `private_key_path` VARCHAR(512),
    `provider` VARCHAR(50) DEFAULT 'letsencrypt',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `dns_provider_id` CHAR(36),
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `acme_account` JSON DEFAULT ('{}'),
    `auto_renew` BOOLEAN NOT NULL DEFAULT TRUE,
    `renewal_attempted_at` DATETIME(6),
    `issued_at` DATETIME(6),
    `error_message` TEXT,
    `certificate_pem` TEXT,
    `private_key_pem` TEXT,
    `issuer_certificate_pem` TEXT,
    `key_type` VARCHAR(10) NOT NULL DEFAULT 'rsa2048',
    PRIMARY KEY (`id`),
    KEY `idx_certificates_auto_renew` (`auto_renew`),
    KEY `idx_certificates_expires_at` (`expires_at`),
    KEY `idx_certificates_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `challenge_configs` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `enabled` BOOLEAN DEFAULT TRUE,
    `challenge_type` VARCHAR(20) DEFAULT 'recaptcha_v2',
    `site_key` VARCHAR(255),
    `secret_key` VARCHAR(255),
    `token_validity` INT DEFAULT 86400,
    `min_score` DECIMAL(3,2) DEFAULT 0.5,
    `apply_to` VARCHAR(20) DEFAULT 'both',
    `page_title` VARCHAR(255) DEFAULT 'Security Check',
    `page_message` TEXT DEFAULT ('Please complete the security check to continue.'),
    `theme` VARCHAR(10) DEFAULT 'light',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `challenge_configs_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_challenge_configs_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `challenge_logs` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `client_ip` VARCHAR(45) NOT NULL,
    `user_agent` TEXT,
    `result` VARCHAR(20) NOT NULL,
    `trigger_reason` VARCHAR(255),
    `captcha_score` DECIMAL(3,2),
    `solve_time` INT,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_challenge_logs_created` (`created_at`),
    KEY `idx_challenge_logs_ip` (`client_ip`),
    KEY `idx_challenge_logs_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `challenge_tokens` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `token_hash` VARCHAR(64) NOT NULL,
    `client_ip` VARCHAR(45) NOT NULL,
    `user_agent` TEXT,
    `challenge_reason` VARCHAR(255),
    `issued_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `expires_at` DATETIME(6) NOT NULL,
    `use_count` INT DEFAULT 0,
    `last_used_at` DATETIME(6),
    `revoked` BOOLEAN DEFAULT FALSE,
    `revoked_at` DATETIME(6),
    `revoked_reason` VARCHAR(255),
    PRIMARY KEY (`id`),
    KEY `idx_challenge_tokens_expires` (`expires_at`),
    KEY `idx_challenge_tokens_hash` (`token_hash`),
    KEY `idx_challenge_tokens_ip` (`client_ip`),
    KEY `idx_challenge_tokens_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `cloud_providers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(100) NOT NULL,
    `slug` VARCHAR(50) NOT NULL,
    `region` VARCHAR(50) NOT NULL,
    `description` TEXT,
    `ip_ranges` TEXT NOT NULL DEFAULT ('{}'),
    `ip_ranges_url` VARCHAR(500),
    `last_updated` DATETIME(6),
    `enabled` BOOLEAN DEFAULT TRUE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `cloud_providers_slug_key` (`slug`),
    KEY `idx_cloud_providers_enabled` (`enabled`),
    KEY `idx_cloud_providers_region` (`region`),
    KEY `idx_cloud_providers_slug` (`slug`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `cloudflare_tunnel` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `token` TEXT NOT NULL DEFAULT (''),
    `mode` VARCHAR(20) NOT NULL DEFAULT 'token',
    `api_token` TEXT NOT NULL DEFAULT (''),
    `catchall_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `catchall_applied_service` TEXT NOT NULL DEFAULT (''),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_cloudflare_tunnel_singleton` (((1))),
    CONSTRAINT `chk_cloudflare_tunnel_mode` CHECK ((`mode`) IN ('token', 'managed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `dashboard_stats_daily` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `day_bucket` DATE NOT NULL,
    `total_requests` BIGINT NOT NULL DEFAULT 0,
    `status_2xx` BIGINT NOT NULL DEFAULT 0,
    `status_3xx` BIGINT NOT NULL DEFAULT 0,
    `status_4xx` BIGINT NOT NULL DEFAULT 0,
    `status_5xx` BIGINT NOT NULL DEFAULT 0,
    `avg_response_time` DOUBLE DEFAULT 0,
    `max_response_time` DOUBLE DEFAULT 0,
    `bytes_sent` BIGINT NOT NULL DEFAULT 0,
    `bytes_received` BIGINT NOT NULL DEFAULT 0,
    `waf_blocked` BIGINT NOT NULL DEFAULT 0,
    `rate_limited` BIGINT NOT NULL DEFAULT 0,
    `bot_blocked` BIGINT NOT NULL DEFAULT 0,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `dashboard_stats_daily_proxy_host_id_day_bucket_key` (`proxy_host_id`, `day_bucket`),
    KEY `idx_stats_daily_bucket` (`day_bucket`),
    KEY `idx_stats_daily_host_bucket` (`proxy_host_id`, `day_bucket`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `dashboard_stats_hourly` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `hour_bucket` DATETIME(6) NOT NULL,
    `total_requests` BIGINT NOT NULL DEFAULT 0,
    `status_2xx` BIGINT NOT NULL DEFAULT 0,
    `status_3xx` BIGINT NOT NULL DEFAULT 0,
    `status_4xx` BIGINT NOT NULL DEFAULT 0,
    `status_5xx` BIGINT NOT NULL DEFAULT 0,
    `avg_response_time` DOUBLE DEFAULT 0,
    `timed_requests` BIGINT NOT NULL DEFAULT 0,
    `max_response_time` DOUBLE DEFAULT 0,
    `min_response_time` DOUBLE DEFAULT 0,
    `p95_response_time` DOUBLE DEFAULT 0,
    `p99_response_time` DOUBLE DEFAULT 0,
    `bytes_sent` BIGINT NOT NULL DEFAULT 0,
    `bytes_received` BIGINT NOT NULL DEFAULT 0,
    `waf_blocked` BIGINT NOT NULL DEFAULT 0,
    `waf_detected` BIGINT NOT NULL DEFAULT 0,
    `rate_limited` BIGINT NOT NULL DEFAULT 0,
    `bot_blocked` BIGINT NOT NULL DEFAULT 0,
    `top_countries` JSON DEFAULT ('{}'),
    `top_paths` JSON DEFAULT ('[]'),
    `top_ips` JSON DEFAULT ('[]'),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `dashboard_stats_hourly_proxy_host_id_hour_bucket_key` (`proxy_host_id`, `hour_bucket`),
    KEY `idx_stats_hourly_bucket` (`hour_bucket`),
    KEY `idx_stats_hourly_host_bucket` (`proxy_host_id`, `hour_bucket`),
    UNIQUE KEY `idx_dashboard_stats_hourly_null_host_bucket` (((CASE WHEN `proxy_host_id` IS NULL THEN `hour_bucket` END)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `ddns_records` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `hostname` VARCHAR(253) NOT NULL,
    `dns_provider_id` CHAR(36) NOT NULL,
    `record_type` VARCHAR(8) NOT NULL DEFAULT 'A',
    `proxied` BOOLEAN NOT NULL DEFAULT FALSE,
    `ttl` INT NOT NULL DEFAULT 1,
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `last_ip` VARCHAR(45) NOT NULL DEFAULT '',
    `last_synced_at` DATETIME(6),
    `last_status` VARCHAR(16) NOT NULL DEFAULT '',
    `last_error` TEXT NOT NULL DEFAULT (''),
    `proxy_host_id` CHAR(36),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_ddns_records_hostname_provider` (`hostname`, `dns_provider_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `dns_entry_states` (
    `hostname` VARCHAR(253) NOT NULL,
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `review_reason` VARCHAR(20),
    `disabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `unused_since` DATETIME(6),
    `published` TEXT NOT NULL DEFAULT ('{}'),
    `last_applied` TEXT NOT NULL DEFAULT ('{}'),
    `empty_cause` VARCHAR(12),
    `generation` BIGINT NOT NULL DEFAULT 0,
    `applied_generation` BIGINT NOT NULL DEFAULT 0,
    `revoke_pending_since` DATETIME(6),
    `candidate` TEXT,
    `candidate_seen` SMALLINT NOT NULL DEFAULT 0,
    `replace_open` BOOLEAN NOT NULL DEFAULT FALSE,
    `v4_state` VARCHAR(16),
    `v6_state` VARCHAR(16),
    `v4_code` VARCHAR(32),
    `v6_code` VARCHAR(32),
    `v4_observed` TEXT,
    `v6_observed` TEXT,
    `v4_answer_at` DATETIME(6),
    `v6_answer_at` DATETIME(6),
    `v4_notfound` SMALLINT NOT NULL DEFAULT 0,
    `v6_notfound` SMALLINT NOT NULL DEFAULT 0,
    `canonical_name` VARCHAR(253),
    `canonical_seen` VARCHAR(253),
    `warnings` TEXT NOT NULL DEFAULT ('{}'),
    `change_times` TEXT NOT NULL DEFAULT ('{}'),
    `last_checked_at` DATETIME(6),
    `last_changed_at` DATETIME(6),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`hostname`),
    CONSTRAINT `chk_dns_entry_states_status` CHECK (`status` IN ('pending', 'ok', 'stale', 'expired', 'rejected', 'not_found', 'needs_review')),
    CONSTRAINT `chk_dns_entry_states_empty_cause` CHECK (`empty_cause` IS NULL OR `empty_cause` IN ('replace', 'lapse', 'disabled', 'restored', 'unused')),
    CONSTRAINT `chk_dns_entry_states_hostname` CHECK (REGEXP_LIKE(`hostname`, '^[a-z0-9.-]{1,253}$', 'c'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `dns_providers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(100) NOT NULL,
    `provider_type` VARCHAR(50) NOT NULL,
    `credentials` JSON NOT NULL DEFAULT ('{}'),
    `is_default` BOOLEAN NOT NULL DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_dns_providers_default` (((CASE WHEN (`is_default` = TRUE) THEN `is_default` END)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `exploit_block_rules` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `category` VARCHAR(50) NOT NULL,
    `name` VARCHAR(100) NOT NULL,
    `pattern` TEXT NOT NULL,
    `pattern_type` VARCHAR(20) NOT NULL DEFAULT 'query_string',
    `description` TEXT,
    `severity` VARCHAR(20) DEFAULT 'warning',
    `enabled` BOOLEAN DEFAULT TRUE,
    `is_system` BOOLEAN DEFAULT TRUE,
    `sort_order` INT DEFAULT 0,
    `auto_disabled_at` DATETIME(6),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_exploit_rules_category` (`category`),
    KEY `idx_exploit_rules_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `fail2ban_configs` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `max_retries` INT DEFAULT 5,
    `find_time` INT DEFAULT 600,
    `ban_time` INT DEFAULT 3600,
    `fail_codes` VARCHAR(100) DEFAULT '401,403,404',
    `action` VARCHAR(20) DEFAULT 'block',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `fail2ban_configs_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_fail2ban_configs_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `filter_subscription_entries` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `subscription_id` CHAR(36) NOT NULL,
    `value` VARCHAR(732) NOT NULL,
    `reason` TEXT,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `filter_subscription_entries_subscription_id_value_key` (`subscription_id`, `value`),
    KEY `idx_fse_subscription` (`subscription_id`),
    KEY `idx_fse_value` (`value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `filter_subscription_entry_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `subscription_id` CHAR(36) NOT NULL,
    `value` VARCHAR(732) NOT NULL,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `filter_subscription_entry_exclusions_subscription_id_value_key` (`subscription_id`, `value`),
    KEY `idx_fsee_subscription` (`subscription_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `filter_subscription_host_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `subscription_id` CHAR(36) NOT NULL,
    `proxy_host_id` CHAR(36) NOT NULL,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `filter_subscription_host_exclusions_subscription_id_proxy_host_` (`subscription_id`, `proxy_host_id`),
    KEY `idx_fshe_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `filter_subscriptions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` TEXT NOT NULL,
    `description` TEXT,
    `url` VARCHAR(768) NOT NULL,
    `format` VARCHAR(20) NOT NULL DEFAULT 'npg-json',
    `type` VARCHAR(20) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `refresh_type` VARCHAR(20) NOT NULL DEFAULT 'interval',
    `refresh_value` VARCHAR(50) NOT NULL DEFAULT '24h',
    `last_fetched_at` DATETIME(6),
    `last_success_at` DATETIME(6),
    `last_error` TEXT,
    `entry_count` INT DEFAULT 0,
    `exclude_private_ips` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `source_entry_count` INT NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `filter_subscriptions_url_key` (`url`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `geo_restrictions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `mode` VARCHAR(20) NOT NULL DEFAULT 'blacklist',
    `countries` TEXT NOT NULL DEFAULT ('{}'),
    `enabled` BOOLEAN DEFAULT TRUE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `allowed_ips` TEXT DEFAULT ('{}'),
    `challenge_mode` BOOLEAN DEFAULT FALSE,
    `blocked_cloud_providers` TEXT DEFAULT ('{}'),
    `challenge_cloud_providers` BOOLEAN DEFAULT FALSE,
    `allow_search_bots_cloud_providers` BOOLEAN DEFAULT FALSE,
    `allow_private_ips` BOOLEAN DEFAULT TRUE,
    `allow_search_bots` BOOLEAN DEFAULT FALSE,
    `disable_global` BOOLEAN NOT NULL DEFAULT FALSE,
    `cloud_disable_global` BOOLEAN NOT NULL DEFAULT FALSE,
    `allowed_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    PRIMARY KEY (`id`),
    UNIQUE KEY `geo_restrictions_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_geo_restrictions_proxy_host` (`proxy_host_id`),
    CONSTRAINT `geo_restrictions_mode_check` CHECK ((`mode`) IN ('whitelist', 'blacklist'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `geoip_update_history` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `trigger_type` VARCHAR(20) NOT NULL DEFAULT 'manual',
    `started_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `completed_at` DATETIME(6),
    `duration_ms` INT,
    `database_version` VARCHAR(50),
    `country_db_size` BIGINT,
    `asn_db_size` BIGINT,
    `error_message` TEXT,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_geoip_update_history_created_at` (`created_at` DESC),
    KEY `idx_geoip_update_history_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_bot_filters` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `block_bad_bots` BOOLEAN DEFAULT TRUE,
    `block_ai_bots` BOOLEAN DEFAULT FALSE,
    `allow_search_engines` BOOLEAN DEFAULT TRUE,
    `block_suspicious_clients` BOOLEAN DEFAULT FALSE,
    `custom_blocked_agents` TEXT,
    `custom_allowed_agents` TEXT,
    `challenge_suspicious` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_global_bot_filters_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_cloud_providers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `blocked_providers` TEXT NOT NULL DEFAULT ('{}'),
    `challenge_mode` BOOLEAN DEFAULT FALSE,
    `allow_search_bots` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_global_cloud_providers_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_exploit_rule_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `rule_id` CHAR(36) NOT NULL,
    `uri_pattern` VARCHAR(512),
    `reason` TEXT,
    `disabled_by` VARCHAR(100),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_global_exploit_exclusions_rule_uri_unique` (`rule_id`, ((COALESCE(`uri_pattern`, ''))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_fail2ban` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `max_retries` INT NOT NULL DEFAULT 5,
    `find_time` INT NOT NULL DEFAULT 600,
    `ban_time` INT NOT NULL DEFAULT 3600,
    `fail_codes` VARCHAR(100) NOT NULL DEFAULT '400,444',
    `action` VARCHAR(20) NOT NULL DEFAULT 'log',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_global_fail2ban_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_geo_restrictions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `mode` VARCHAR(20) NOT NULL DEFAULT 'blacklist',
    `countries` TEXT NOT NULL DEFAULT ('{}'),
    `allowed_ips` TEXT DEFAULT ('{}'),
    `allow_private_ips` BOOLEAN DEFAULT TRUE,
    `allow_search_bots` BOOLEAN DEFAULT FALSE,
    `challenge_mode` BOOLEAN DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `allowed_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    UNIQUE KEY `idx_global_geo_restrictions_singleton` (((1))),
    CONSTRAINT `global_geo_restrictions_mode_check` CHECK ((`mode`) IN ('whitelist', 'blacklist'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_rate_limits` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `requests_per_second` INT DEFAULT 50,
    `burst_size` INT DEFAULT 100,
    `zone_size` VARCHAR(10) DEFAULT '10m',
    `limit_by` VARCHAR(20) DEFAULT 'ip',
    `limit_response` INT DEFAULT 429,
    `whitelist_ips` TEXT,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `whitelist_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    UNIQUE KEY `idx_global_rate_limits_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_security_headers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `hsts_enabled` BOOLEAN DEFAULT TRUE,
    `hsts_max_age` INT DEFAULT 31536000,
    `hsts_include_subdomains` BOOLEAN DEFAULT TRUE,
    `hsts_preload` BOOLEAN DEFAULT FALSE,
    `x_frame_options` VARCHAR(50) DEFAULT 'SAMEORIGIN',
    `x_content_type_options` BOOLEAN DEFAULT TRUE,
    `x_xss_protection` BOOLEAN DEFAULT TRUE,
    `referrer_policy` VARCHAR(100) DEFAULT 'strict-origin-when-cross-origin',
    `content_security_policy` TEXT,
    `permissions_policy` TEXT,
    `custom_headers` JSON DEFAULT ('{}'),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_global_security_headers_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_settings` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `worker_processes` INT NOT NULL DEFAULT 0,
    `worker_connections` INT NOT NULL DEFAULT 8192,
    `worker_rlimit_nofile` INT,
    `worker_shutdown_timeout` INT,
    `multi_accept` BOOLEAN NOT NULL DEFAULT TRUE,
    `use_epoll` BOOLEAN NOT NULL DEFAULT TRUE,
    `sendfile` BOOLEAN NOT NULL DEFAULT TRUE,
    `tcp_nopush` BOOLEAN NOT NULL DEFAULT TRUE,
    `tcp_nodelay` BOOLEAN NOT NULL DEFAULT TRUE,
    `keepalive_timeout` INT NOT NULL DEFAULT 30,
    `keepalive_requests` INT NOT NULL DEFAULT 1000,
    `types_hash_max_size` INT NOT NULL DEFAULT 2048,
    `server_tokens` BOOLEAN NOT NULL DEFAULT FALSE,
    `client_body_buffer_size` VARCHAR(20) NOT NULL DEFAULT '16k',
    `client_header_buffer_size` VARCHAR(20) NOT NULL DEFAULT '4k',
    `client_max_body_size` VARCHAR(20) NOT NULL DEFAULT '100m',
    `large_client_header_buffers` VARCHAR(20) NOT NULL DEFAULT '4 16k',
    `client_body_timeout` INT NOT NULL DEFAULT 60,
    `client_header_timeout` INT NOT NULL DEFAULT 60,
    `send_timeout` INT NOT NULL DEFAULT 60,
    `proxy_connect_timeout` INT NOT NULL DEFAULT 60,
    `proxy_send_timeout` INT NOT NULL DEFAULT 60,
    `proxy_read_timeout` INT NOT NULL DEFAULT 60,
    `gzip_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `gzip_vary` BOOLEAN NOT NULL DEFAULT TRUE,
    `gzip_proxied` VARCHAR(50) NOT NULL DEFAULT 'any',
    `gzip_comp_level` INT NOT NULL DEFAULT 6,
    `gzip_buffers` VARCHAR(20) NOT NULL DEFAULT '16 8k',
    `gzip_http_version` VARCHAR(10) NOT NULL DEFAULT '1.1',
    `gzip_min_length` INT NOT NULL DEFAULT 256,
    `gzip_types` TEXT NOT NULL DEFAULT ('text/plain text/css text/xml text/javascript application/json application/javascript application/xml application/xml+rss application/x-javascript image/svg+xml'),
    `ssl_protocols` VARCHAR(100) NOT NULL DEFAULT 'TLSv1.2 TLSv1.3',
    `ssl_ciphers` TEXT NOT NULL DEFAULT ('ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384'),
    `ssl_prefer_server_ciphers` BOOLEAN NOT NULL DEFAULT TRUE,
    `ssl_session_cache` VARCHAR(50) NOT NULL DEFAULT 'shared:SSL:10m',
    `ssl_session_timeout` VARCHAR(20) NOT NULL DEFAULT '1d',
    `ssl_session_tickets` BOOLEAN NOT NULL DEFAULT FALSE,
    `ssl_stapling` BOOLEAN NOT NULL DEFAULT TRUE,
    `ssl_stapling_verify` BOOLEAN NOT NULL DEFAULT TRUE,
    `ssl_ecdh_curve` VARCHAR(255) NOT NULL DEFAULT 'X25519MLKEM768:X25519:secp256r1:secp384r1',
    `access_log_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `access_log_strip_query` BOOLEAN NOT NULL DEFAULT FALSE,
    `error_log_level` VARCHAR(20) NOT NULL DEFAULT 'warn',
    `resolver` VARCHAR(255) DEFAULT '1.1.1.1 8.8.8.8 valid=300s',
    `resolver_timeout` VARCHAR(20) DEFAULT '5s',
    `custom_http_config` TEXT DEFAULT (''),
    `custom_stream_config` TEXT DEFAULT (''),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `brotli_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `brotli_comp_level` INT NOT NULL DEFAULT 6,
    `brotli_types` TEXT NOT NULL DEFAULT ('text/plain text/css text/xml text/javascript application/json application/javascript application/xml application/xml+rss image/svg+xml'),
    `direct_ip_access_action` VARCHAR(20) DEFAULT 'allow',
    `enable_ipv6` BOOLEAN NOT NULL DEFAULT TRUE,
    `limit_conn_zone_size` VARCHAR(10) DEFAULT '10m',
    `limit_conn_per_ip` INT DEFAULT 100,
    `limit_conn_enabled` BOOLEAN DEFAULT FALSE,
    `limit_req_zone_size` VARCHAR(10) DEFAULT '10m',
    `limit_req_rate` INT DEFAULT 50,
    `limit_req_burst` INT DEFAULT 100,
    `limit_req_enabled` BOOLEAN DEFAULT FALSE,
    `reset_timedout_connection` BOOLEAN DEFAULT TRUE,
    `limit_rate` INT DEFAULT 0,
    `limit_rate_after` VARCHAR(10) DEFAULT '0',
    `proxy_buffer_size` VARCHAR(20) NOT NULL DEFAULT '8k',
    `proxy_buffers` VARCHAR(20) NOT NULL DEFAULT '8 32k',
    `proxy_busy_buffers_size` VARCHAR(20) NOT NULL DEFAULT '128k',
    `proxy_max_temp_file_size` VARCHAR(20) NOT NULL DEFAULT '1024m',
    `proxy_temp_file_write_size` VARCHAR(20) NOT NULL DEFAULT '64k',
    `proxy_buffering` VARCHAR(10) DEFAULT '',
    `proxy_request_buffering` VARCHAR(10) DEFAULT '',
    `open_file_cache_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `open_file_cache_max` INT NOT NULL DEFAULT 10000,
    `open_file_cache_inactive` VARCHAR(20) NOT NULL DEFAULT '60s',
    `open_file_cache_valid` VARCHAR(20) NOT NULL DEFAULT '30s',
    `open_file_cache_min_uses` INT NOT NULL DEFAULT 2,
    `open_file_cache_errors` BOOLEAN NOT NULL DEFAULT TRUE,
    `brotli_static` BOOLEAN NOT NULL DEFAULT TRUE,
    `connection_upgrade_empty` VARCHAR(10) NOT NULL DEFAULT '',
    `brotli_min_length` INT NOT NULL DEFAULT 1000,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_uri_blocks` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT TRUE,
    `rules` JSON DEFAULT ('[]'),
    `exception_ips` TEXT DEFAULT ('{}'),
    `allow_private_ips` BOOLEAN DEFAULT TRUE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `exception_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_global_uri_blocks_singleton` (((1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_waf` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `enabled` BOOLEAN DEFAULT FALSE,
    `mode` VARCHAR(20) DEFAULT 'detection',
    `paranoia_level` INT DEFAULT 1,
    `anomaly_threshold` INT DEFAULT 5,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY `idx_global_waf_singleton` (((1))),
    CONSTRAINT `chk_global_waf_anomaly_threshold` CHECK ((`anomaly_threshold` >= 1) AND (`anomaly_threshold` <= 100)),
    CONSTRAINT `chk_global_waf_paranoia_level` CHECK ((`paranoia_level` >= 1) AND (`paranoia_level` <= 4))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_waf_policy_history` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `rule_id` VARCHAR(20) NOT NULL,
    `rule_category` VARCHAR(255),
    `rule_description` TEXT,
    `action` VARCHAR(20) NOT NULL,
    `reason` TEXT,
    `changed_by` VARCHAR(255),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_global_waf_policy_history_created_at` (`created_at` DESC),
    KEY `idx_global_waf_policy_history_rule_id` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `global_waf_rule_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `rule_id` VARCHAR(20) NOT NULL,
    `rule_category` VARCHAR(255),
    `rule_description` TEXT,
    `reason` TEXT,
    `disabled_by` VARCHAR(255),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `global_waf_rule_exclusions_rule_id_key` (`rule_id`),
    KEY `idx_global_waf_rule_exclusions_rule_id` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `host_exploit_rule_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `rule_id` CHAR(36) NOT NULL,
    `uri_pattern` VARCHAR(512),
    `reason` TEXT,
    `disabled_by` VARCHAR(100),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_host_exploit_exclusions_host` (`proxy_host_id`),
    KEY `idx_host_exploit_exclusions_rule` (`rule_id`),
    UNIQUE KEY `idx_host_exploit_exclusions_host_rule_uri_unique` (`proxy_host_id`, `rule_id`, ((COALESCE(`uri_pattern`, ''))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `ip_ban_history` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `event_type` VARCHAR(10) NOT NULL,
    `ip_address` VARCHAR(45) NOT NULL,
    `proxy_host_id` CHAR(36),
    `domain_name` VARCHAR(255),
    `reason` TEXT,
    `source` VARCHAR(50) NOT NULL,
    `ban_duration` INT,
    `expires_at` DATETIME(6),
    `is_permanent` BOOLEAN DEFAULT FALSE,
    `is_auto` BOOLEAN DEFAULT FALSE,
    `fail_count` INT,
    `user_id` CHAR(36),
    `user_email` VARCHAR(255),
    `metadata` JSON,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_ip_ban_history_created_at` (`created_at` DESC),
    KEY `idx_ip_ban_history_event_type` (`event_type`),
    KEY `idx_ip_ban_history_ip` (`ip_address`),
    KEY `idx_ip_ban_history_ip_created` (`ip_address`, `created_at` DESC),
    KEY `idx_ip_ban_history_proxy_host` (`proxy_host_id`),
    KEY `idx_ip_ban_history_source` (`source`),
    KEY `idx_ip_ban_history_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `log_filter_presets` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` TEXT NOT NULL,
    `log_type` TEXT NOT NULL DEFAULT ('access'),
    `filter` JSON NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_log_filter_presets_log_type` (`log_type`(191))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `log_settings` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `retention_days` INT NOT NULL DEFAULT 30,
    `max_logs_per_type` BIGINT,
    `auto_cleanup_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `system_log_retention_days` INT NOT NULL DEFAULT 7,
    `enable_docker_logs` BOOLEAN NOT NULL DEFAULT TRUE,
    `filter_health_checks` BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `login_attempts` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `ip_address` VARCHAR(45) NOT NULL,
    `username` VARCHAR(255),
    `success` BOOLEAN NOT NULL DEFAULT FALSE,
    `attempted_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_login_attempts_ip` (`ip_address`, `attempted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `logs_partitioned` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `log_type` VARCHAR(64) NOT NULL,
    `timestamp` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `host` TEXT,
    `client_ip` VARCHAR(45),
    `request_method` TEXT,
    `request_uri` TEXT,
    `request_protocol` TEXT,
    `status_code` INT,
    `body_bytes_sent` BIGINT,
    `request_time` DOUBLE,
    `upstream_response_time` DOUBLE,
    `upstream_addr` TEXT,
    `upstream_status` TEXT,
    `http_referer` TEXT,
    `http_user_agent` TEXT,
    `http_x_forwarded_for` TEXT,
    `severity` VARCHAR(64),
    `error_message` TEXT,
    `rule_id` BIGINT,
    `rule_message` TEXT,
    `rule_severity` TEXT,
    `rule_data` TEXT,
    `attack_type` TEXT,
    `action_taken` TEXT,
    `proxy_host_id` CHAR(36),
    `raw_log` TEXT,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `geo_country` TEXT,
    `geo_country_code` VARCHAR(2),
    `geo_city` TEXT,
    `geo_asn` TEXT,
    `geo_org` TEXT,
    `block_reason` VARCHAR(64) DEFAULT 'none',
    `bot_category` TEXT,
    `exploit_rule` VARCHAR(50),
    PRIMARY KEY (`id`, `created_at`),
    KEY `idx_logs_part_host` (`host`(191)),
    KEY `idx_logs_partitioned_exploit_rule` (`exploit_rule`),
    KEY `idx_logs_part_client_ip` (`client_ip`),
    KEY `idx_logs_part_created_at` (`created_at` DESC),
    KEY `idx_logs_part_proxy_host_ts` (`proxy_host_id`, `timestamp` DESC),
    KEY `idx_logs_part_geo_ts` (`geo_country_code`, `timestamp` DESC),
    KEY `idx_logs_part_type_created` (`log_type`, `created_at` DESC),
    KEY `idx_logs_part_block_reason` (`block_reason`),
    KEY `idx_logs_part_status_created` (`status_code`, `created_at`),
    KEY `idx_logs_part_block_reason_created` (`created_at` DESC, `block_reason`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `notification_channels` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(64) NOT NULL,
    `type` VARCHAR(16) NOT NULL,
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `config` JSON NOT NULL DEFAULT ('{}'),
    `events` TEXT NOT NULL DEFAULT ('{}'),
    `digest_events` TEXT NOT NULL DEFAULT ('{}'),
    `rich_format` BOOLEAN NOT NULL DEFAULT TRUE,
    `language` VARCHAR(8) NOT NULL DEFAULT 'en',
    `dashboard_url` TEXT,
    `digest_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `digest_hour` SMALLINT NOT NULL DEFAULT 9,
    `allow_private_target` BOOLEAN NOT NULL DEFAULT FALSE,
    `template` TEXT,
    `last_success_at` DATETIME(6),
    `last_error_at` DATETIME(6),
    `last_error` TEXT,
    `consecutive_failures` INT NOT NULL DEFAULT 0,
    `last_digest_on` DATE,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `notification_channels_name_key` (((LOWER((`name`)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `notification_outbox` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `channel_id` CHAR(36) NOT NULL,
    `event_key` VARCHAR(64) NOT NULL,
    `payload` JSON NOT NULL,
    `status` VARCHAR(16) NOT NULL DEFAULT 'queued',
    `attempts` INT NOT NULL DEFAULT 0,
    `next_attempt_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `last_error` TEXT,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `sent_at` DATETIME(6),
    PRIMARY KEY (`id`),
    KEY `notification_outbox_due_idx` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `notification_state` (
    `event_key` VARCHAR(64) NOT NULL,
    `subject` VARCHAR(255) NOT NULL,
    `subject_label` VARCHAR(255),
    `state` VARCHAR(16) NOT NULL,
    `since` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `last_detail` TEXT,
    PRIMARY KEY (`event_key`, `subject`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `proxy_host_log_rules` (
    `proxy_host_id` CHAR(36) NOT NULL,
    `skip_all` BOOLEAN NOT NULL DEFAULT FALSE,
    `skip_statuses` TEXT NOT NULL DEFAULT ('{}'),
    `skip_methods` TEXT NOT NULL DEFAULT ('{}'),
    `skip_paths` TEXT NOT NULL DEFAULT ('{}'),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `proxy_hosts` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_type` VARCHAR(20) NOT NULL DEFAULT 'http',
    `domain_names` TEXT NOT NULL,
    `tags` TEXT NOT NULL DEFAULT ('{}'),
    `forward_scheme` VARCHAR(10) NOT NULL DEFAULT 'http',
    `forward_host` VARCHAR(255) NOT NULL,
    `forward_port` INT NOT NULL DEFAULT 80,
    `forward_container_name` TEXT,
    `forward_container_network` TEXT,
    `stream_listen_host` VARCHAR(255) DEFAULT '',
    `stream_listen_port` INT DEFAULT 0,
    `stream_protocol` VARCHAR(10) DEFAULT 'tcp',
    `stream_ssl_preread` BOOLEAN NOT NULL DEFAULT FALSE,
    `stream_accept_proxy_protocol` BOOLEAN NOT NULL DEFAULT FALSE,
    `stream_send_proxy_protocol` BOOLEAN NOT NULL DEFAULT FALSE,
    `stream_proxy_connect_timeout` INT DEFAULT 0,
    `stream_proxy_timeout` INT DEFAULT 0,
    `ssl_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `ssl_force_https` BOOLEAN NOT NULL DEFAULT FALSE,
    `ssl_http2` BOOLEAN NOT NULL DEFAULT TRUE,
    `certificate_id` CHAR(36),
    `allow_websocket_upgrade` BOOLEAN NOT NULL DEFAULT TRUE,
    `cache_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `cache_static_only` BOOLEAN NOT NULL DEFAULT TRUE,
    `cache_ttl` VARCHAR(20) NOT NULL DEFAULT '7d',
    `block_exploits` BOOLEAN NOT NULL DEFAULT TRUE,
    `custom_locations` JSON DEFAULT ('[]'),
    `advanced_config` TEXT DEFAULT (''),
    `waf_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `waf_mode` VARCHAR(20) DEFAULT 'detection',
    `access_list_id` CHAR(36),
    `auth_provider_id` CHAR(36),
    `auth_bypass_paths` TEXT DEFAULT ('{}'),
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `meta` JSON DEFAULT ('{}'),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `ssl_http3` BOOLEAN NOT NULL DEFAULT FALSE,
    `rate_limit_enabled` BOOLEAN DEFAULT FALSE,
    `fail2ban_enabled` BOOLEAN DEFAULT FALSE,
    `bot_filter_enabled` BOOLEAN DEFAULT FALSE,
    `security_headers_enabled` BOOLEAN DEFAULT FALSE,
    `waf_paranoia_level` INT DEFAULT 1,
    `waf_anomaly_threshold` INT DEFAULT 5,
    `block_exploits_exceptions` TEXT DEFAULT (''),
    `proxy_connect_timeout` INT DEFAULT 0,
    `proxy_send_timeout` INT DEFAULT 0,
    `proxy_read_timeout` INT DEFAULT 0,
    `proxy_buffering` VARCHAR(10) DEFAULT '',
    `proxy_request_buffering` VARCHAR(10) DEFAULT '',
    `client_max_body_size` VARCHAR(20) DEFAULT '',
    `proxy_max_temp_file_size` VARCHAR(20) DEFAULT '',
    `is_favorite` BOOLEAN NOT NULL DEFAULT FALSE,
    `config_status` VARCHAR(20) NOT NULL DEFAULT 'ok',
    `config_error` TEXT,
    `ddns_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `ddns_provider_id` CHAR(36),
    `ddns_proxied` BOOLEAN NOT NULL DEFAULT FALSE,
    `waf_use_global` BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (`id`),
    KEY `idx_proxy_hosts_created_at` (`created_at` DESC),
    KEY `idx_proxy_hosts_enabled` (`enabled`),
    UNIQUE KEY `idx_proxy_hosts_stream_listener_unique` (((CASE WHEN `proxy_type` = 'stream' AND `enabled` = TRUE AND `stream_listen_port` > 0 THEN `stream_listen_host` END)), ((CASE WHEN `proxy_type` = 'stream' AND `enabled` = TRUE AND `stream_listen_port` > 0 THEN `stream_listen_port` END)), ((CASE WHEN `proxy_type` = 'stream' AND `enabled` = TRUE AND `stream_listen_port` > 0 THEN `stream_protocol` END))),
    CONSTRAINT `chk_waf_anomaly_threshold` CHECK ((`waf_anomaly_threshold` >= 1) AND (`waf_anomaly_threshold` <= 100)),
    CONSTRAINT `chk_waf_paranoia_level` CHECK ((`waf_paranoia_level` >= 1) AND (`waf_paranoia_level` <= 4))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `rate_limits` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `requests_per_second` INT DEFAULT 10,
    `burst_size` INT DEFAULT 20,
    `zone_size` VARCHAR(10) DEFAULT '10m',
    `limit_by` VARCHAR(20) DEFAULT 'ip',
    `limit_response` INT DEFAULT 429,
    `whitelist_ips` TEXT,
    `disable_global` BOOLEAN NOT NULL DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `whitelist_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    PRIMARY KEY (`id`),
    UNIQUE KEY `rate_limits_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_rate_limits_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `raw_log_reclaim_chunks` (
    `chunk_name` VARCHAR(768) NOT NULL,
    `range_start` DATETIME(6) NOT NULL,
    `range_end` DATETIME(6) NOT NULL,
    `state` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `bytes_before` BIGINT,
    `bytes_after` BIGINT,
    `raw_bytes` BIGINT,
    `batches_nulled` INT,
    `update_xid` BIGINT,
    `attempts` INT NOT NULL DEFAULT 0,
    `conn_losses` INT NOT NULL DEFAULT 0,
    `last_error` TEXT,
    `worked_at` DATETIME(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`chunk_name`),
    CONSTRAINT `chk_raw_log_reclaim_chunks_state` CHECK (`state` IN ('pending', 'nulled', 'done', 'skipped', 'gone', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `raw_log_reclaim_job` (
    `id` BOOLEAN NOT NULL DEFAULT TRUE,
    `status` VARCHAR(20) NOT NULL DEFAULT 'idle',
    `max_chunks` INT,
    `requested_at` DATETIME(6),
    `requested_by` TEXT,
    `started_at` DATETIME(6),
    `finished_at` DATETIME(6),
    `last_error` TEXT,
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `chk_raw_log_reclaim_job_singleton` CHECK (`id` = TRUE),
    CONSTRAINT `chk_raw_log_reclaim_job_status` CHECK (`status` IN ('idle', 'running', 'paused', 'done', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `redirect_hosts` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `domain_names` TEXT NOT NULL DEFAULT ('{}'),
    `forward_scheme` VARCHAR(10) NOT NULL DEFAULT 'auto',
    `forward_domain_name` VARCHAR(255) NOT NULL,
    `forward_path` VARCHAR(1024) DEFAULT '',
    `preserve_path` BOOLEAN DEFAULT TRUE,
    `redirect_code` INT DEFAULT 301,
    `ssl_enabled` BOOLEAN DEFAULT FALSE,
    `certificate_id` CHAR(36),
    `ssl_force_https` BOOLEAN DEFAULT TRUE,
    `enabled` BOOLEAN DEFAULT TRUE,
    `block_exploits` BOOLEAN DEFAULT FALSE,
    `meta` JSON DEFAULT ('{}'),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `redirect_hosts_redirect_code_check` CHECK (`redirect_code` IN (301, 302, 307, 308))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `role_permissions` (
    `role_id` CHAR(36) NOT NULL,
    `permission` VARCHAR(64) NOT NULL,
    PRIMARY KEY (`role_id`, `permission`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `roles` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `name` VARCHAR(64) NOT NULL,
    `description` TEXT,
    `is_superuser` BOOLEAN NOT NULL DEFAULT FALSE,
    `is_builtin` BOOLEAN NOT NULL DEFAULT FALSE,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `roles_name_key` (((LOWER((`name`)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `security_headers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `hsts_enabled` BOOLEAN DEFAULT TRUE,
    `hsts_max_age` INT DEFAULT 31536000,
    `hsts_include_subdomains` BOOLEAN DEFAULT TRUE,
    `hsts_preload` BOOLEAN DEFAULT FALSE,
    `x_frame_options` VARCHAR(100) DEFAULT 'SAMEORIGIN',
    `x_content_type_options` BOOLEAN DEFAULT TRUE,
    `x_xss_protection` BOOLEAN DEFAULT TRUE,
    `referrer_policy` VARCHAR(50) DEFAULT 'strict-origin-when-cross-origin',
    `content_security_policy` TEXT,
    `permissions_policy` TEXT,
    `custom_headers` JSON,
    `disable_global` BOOLEAN NOT NULL DEFAULT FALSE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `security_headers_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_security_headers_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `settings` (
    `key` VARCHAR(255) NOT NULL,
    `value` JSON NOT NULL,
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `sso_login_states` (
    `state` VARCHAR(64) NOT NULL,
    `provider_id` CHAR(36) NOT NULL,
    `nonce` VARCHAR(64) NOT NULL,
    `code_verifier` VARCHAR(128) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `expires_at` DATETIME(6) NOT NULL,
    PRIMARY KEY (`state`),
    KEY `sso_login_states_expires_at_idx` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `sso_providers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `slug` VARCHAR(32) NOT NULL,
    `name` VARCHAR(64) NOT NULL,
    `issuer_url` TEXT NOT NULL,
    `client_id` TEXT NOT NULL,
    `client_secret` TEXT NOT NULL,
    `scopes` TEXT NOT NULL DEFAULT ('openid profile email'),
    `callback_base_url` TEXT,
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `allow_jit` BOOLEAN NOT NULL DEFAULT FALSE,
    `trust_provider_email` BOOLEAN NOT NULL DEFAULT FALSE,
    `allowed_email_domains` TEXT NOT NULL DEFAULT ('{}'),
    `allowed_emails` TEXT NOT NULL DEFAULT ('{}'),
    `group_claim` VARCHAR(64) NOT NULL DEFAULT 'groups',
    `required_group` TEXT,
    `default_role_id` CHAR(36),
    `group_role_mappings` JSON NOT NULL DEFAULT ('[]'),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `sso_providers_slug_key` (((LOWER((`slug`)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `system_health` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `recorded_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `nginx_status` VARCHAR(20) NOT NULL DEFAULT 'unknown',
    `nginx_workers` INT DEFAULT 0,
    `nginx_connections_active` INT DEFAULT 0,
    `nginx_connections_reading` INT DEFAULT 0,
    `nginx_connections_writing` INT DEFAULT 0,
    `nginx_connections_waiting` INT DEFAULT 0,
    `db_status` VARCHAR(20) NOT NULL DEFAULT 'unknown',
    `db_connections` INT DEFAULT 0,
    `cpu_usage` DOUBLE DEFAULT 0,
    `memory_usage` DOUBLE DEFAULT 0,
    `disk_usage` DOUBLE DEFAULT 0,
    `certs_total` INT DEFAULT 0,
    `certs_expiring_soon` INT DEFAULT 0,
    `certs_expired` INT DEFAULT 0,
    `upstreams_total` INT DEFAULT 0,
    `upstreams_healthy` INT DEFAULT 0,
    `upstreams_unhealthy` INT DEFAULT 0,
    `memory_total` BIGINT DEFAULT 0,
    `memory_used` BIGINT DEFAULT 0,
    `disk_total` BIGINT DEFAULT 0,
    `disk_used` BIGINT DEFAULT 0,
    `disk_path` VARCHAR(255) DEFAULT '/',
    `network_in` BIGINT DEFAULT 0,
    `network_out` BIGINT DEFAULT 0,
    `uptime_seconds` BIGINT DEFAULT 0,
    `hostname` VARCHAR(255) DEFAULT '',
    `os` VARCHAR(255) DEFAULT '',
    `platform` VARCHAR(100) DEFAULT '',
    `kernel_version` VARCHAR(255) DEFAULT '',
    PRIMARY KEY (`id`),
    KEY `idx_system_health_recorded` (`recorded_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `system_logs` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `source` VARCHAR(64) NOT NULL,
    `level` VARCHAR(64) NOT NULL DEFAULT 'info',
    `message` TEXT NOT NULL,
    `details` JSON,
    `container_name` VARCHAR(100),
    `component` VARCHAR(100),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_system_logs_container` (`container_name`, `created_at` DESC),
    KEY `idx_system_logs_created` (`created_at` DESC),
    KEY `idx_system_logs_level` (`level`),
    KEY `idx_system_logs_source` (`source`),
    KEY `idx_system_logs_source_created` (`source`, `created_at` DESC),
    KEY `idx_system_logs_source_level_created` (`source`, `level`, `created_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `system_settings` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `geoip_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `maxmind_license_key` VARCHAR(255) DEFAULT '',
    `maxmind_account_id` VARCHAR(100) DEFAULT '',
    `geoip_auto_update` BOOLEAN NOT NULL DEFAULT TRUE,
    `geoip_update_interval` VARCHAR(20) NOT NULL DEFAULT '7d',
    `geoip_last_updated` DATETIME(6),
    `geoip_database_version` VARCHAR(100) DEFAULT '',
    `acme_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `acme_email` VARCHAR(255) DEFAULT '',
    `acme_staging` BOOLEAN NOT NULL DEFAULT FALSE,
    `acme_auto_renew` BOOLEAN NOT NULL DEFAULT TRUE,
    `acme_renew_days_before` INT NOT NULL DEFAULT 30,
    `acme_dns_provider` VARCHAR(50) DEFAULT '',
    `acme_dns_credentials` JSON DEFAULT ('{}'),
    `notification_email` VARCHAR(255) DEFAULT '',
    `notify_cert_expiry` BOOLEAN NOT NULL DEFAULT TRUE,
    `notify_cert_expiry_days` INT NOT NULL DEFAULT 14,
    `notify_security_events` BOOLEAN NOT NULL DEFAULT TRUE,
    `notify_backup_complete` BOOLEAN NOT NULL DEFAULT FALSE,
    `log_retention_days` INT NOT NULL DEFAULT 30,
    `stats_retention_days` INT NOT NULL DEFAULT 90,
    `backup_retention_count` INT NOT NULL DEFAULT 10,
    `auto_backup_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `auto_backup_schedule` VARCHAR(50) DEFAULT '0 2 * * *',
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `access_log_retention_days` INT NOT NULL DEFAULT 1095,
    `waf_log_retention_days` INT NOT NULL DEFAULT 90,
    `error_log_retention_days` INT NOT NULL DEFAULT 30,
    `system_log_retention_days` INT NOT NULL DEFAULT 30,
    `audit_log_retention_days` INT NOT NULL DEFAULT 1095,
    `raw_log_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `raw_log_retention_days` INT NOT NULL DEFAULT 7,
    `raw_log_max_size_mb` INT NOT NULL DEFAULT 100,
    `raw_log_rotate_count` INT NOT NULL DEFAULT 5,
    `raw_log_compress_rotated` BOOLEAN NOT NULL DEFAULT TRUE,
    `raw_log_archive_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `raw_log_archive_retention_days` INT NOT NULL DEFAULT 365,
    `bot_filter_default_enabled` BOOLEAN DEFAULT FALSE,
    `bot_filter_default_block_bad_bots` BOOLEAN DEFAULT TRUE,
    `bot_filter_default_block_ai_bots` BOOLEAN DEFAULT FALSE,
    `bot_filter_default_allow_search_engines` BOOLEAN DEFAULT TRUE,
    `bot_filter_default_challenge_suspicious` BOOLEAN DEFAULT FALSE,
    `bot_filter_default_custom_blocked_agents` TEXT DEFAULT (''),
    `bot_list_bad_bots` TEXT DEFAULT ('AhrefsBot
SemrushBot
DotBot
MJ12bot
BLEXBot
DataForSeoBot
serpstatbot
AspiegelBot
BacklinkCrawler
Exabot
Screaming Frog
MegaIndex
LinkpadBot
Nimbostratus-Bot
TurnitinBot
PetalBot
Seekport Crawler
Bytespider
MauiBot
Sogou web spider
YandexBot
Baiduspider'),
    `bot_list_ai_bots` TEXT DEFAULT ('GPTBot
ChatGPT-User
Claude-Web
ClaudeBot
anthropic-ai
Amazonbot
CCBot
Google-Extended
FacebookBot
PerplexityBot
YouBot
Cohere-ai'),
    `bot_list_search_engines` TEXT DEFAULT ('Googlebot
Bingbot
DuckDuckBot
Slurp
facebot
Twitterbot
LinkedInBot
WhatsApp
TelegramBot
Discordbot
Slackbot
Applebot'),
    `bot_list_suspicious_clients` TEXT DEFAULT ('curl
Wget
libwww-perl
python-requests
Python-urllib
Python-httpx
httpx
aiohttp
Java
Go-http-client
Go-http
http_requester
HttpClient
Apache-HttpClient
okhttp
node-fetch
axios
got
request
fetch
urllib
http.client
requests
scrapy
mechanize
phantom
headless
puppeteer
playwright
selenium
chromedriver
geckodriver'),
    `bot_filter_default_block_suspicious_clients` BOOLEAN DEFAULT FALSE,
    `waf_auto_ban_enabled` BOOLEAN DEFAULT FALSE,
    `waf_auto_ban_threshold` INT DEFAULT 10,
    `waf_auto_ban_window` INT DEFAULT 300,
    `waf_auto_ban_duration` INT DEFAULT 3600,
    `direct_ip_access_action` VARCHAR(20) DEFAULT 'allow',
    `system_logs_enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `system_logs_levels` JSON DEFAULT ('{"npg-db": "warn", "npg-ui": "warn", "npg-api": "info", "npg-proxy": "info"}'),
    `system_logs_exclude_patterns` TEXT DEFAULT ('{"/health","/nginx_status","/.well-known/","HEAD /"}'),
    `system_logs_stdout_excluded` TEXT DEFAULT ('{"npg-proxy"}'),
    `ui_font_family` VARCHAR(50) DEFAULT 'system',
    `ui_error_page_language` VARCHAR(10) DEFAULT 'auto',
    `instance_name` VARCHAR(32) NOT NULL DEFAULT '',
    `global_block_exploits_exceptions` TEXT DEFAULT ('^/wp-json/
^/api/v1/challenge/
^/wp-admin/admin-ajax.php
^/webapi/'),
    `global_trusted_ips` TEXT DEFAULT (''),
    `global_trusted_ips_bypass_waf` BOOLEAN NOT NULL DEFAULT FALSE,
    `ddns_check_interval_minutes` INT NOT NULL DEFAULT 5,
    `trusted_proxy_cidrs` TEXT NOT NULL DEFAULT (''),
    `trusted_proxy_preset` TEXT NOT NULL DEFAULT ('none'),
    `real_ip_header` TEXT NOT NULL DEFAULT ('X-Forwarded-For'),
    `dns_entry_settings` JSON NOT NULL DEFAULT ('{}'),
    `global_trusted_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    PRIMARY KEY (`id`),
    KEY `idx_system_settings_updated` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `traffic_hourly_backfill` (
    `id` SMALLINT NOT NULL,
    `next_hour` DATETIME(6) NOT NULL,
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `traffic_hourly_backfill_id_check` CHECK (`id` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `upstream_servers` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `upstream_id` CHAR(36) NOT NULL,
    `address` VARCHAR(255) NOT NULL,
    `port` INT DEFAULT 80,
    `weight` INT DEFAULT 1,
    `max_fails` INT DEFAULT 3,
    `fail_timeout` INT DEFAULT 30,
    `is_backup` BOOLEAN DEFAULT FALSE,
    `is_down` BOOLEAN DEFAULT FALSE,
    `is_healthy` BOOLEAN DEFAULT TRUE,
    `last_check_at` DATETIME(6),
    `last_error` TEXT,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_upstream_servers_upstream` (`upstream_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `upstreams` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `scheme` VARCHAR(10) NOT NULL DEFAULT 'http',
    `servers` JSON NOT NULL DEFAULT ('[]'),
    `load_balance` VARCHAR(20) DEFAULT 'round_robin',
    `health_check_enabled` BOOLEAN DEFAULT FALSE,
    `health_check_interval` INT DEFAULT 30,
    `health_check_timeout` INT DEFAULT 5,
    `health_check_path` VARCHAR(255) DEFAULT '/',
    `health_check_expected_status` INT DEFAULT 200,
    `keepalive` INT DEFAULT 32,
    `is_healthy` BOOLEAN DEFAULT TRUE,
    `last_check_at` DATETIME(6),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `upstreams_proxy_host_id_key` (`proxy_host_id`),
    KEY `idx_upstreams_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `uri_blocks` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `enabled` BOOLEAN DEFAULT TRUE,
    `rules` JSON DEFAULT ('[]'),
    `exception_ips` TEXT DEFAULT ('{}'),
    `allow_private_ips` BOOLEAN DEFAULT TRUE,
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `exception_hostnames` TEXT NOT NULL DEFAULT ('{}'),
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_uri_blocks_proxy_host` (`proxy_host_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `user_identities` (
    `provider_id` CHAR(36) NOT NULL,
    `subject` VARCHAR(732) NOT NULL,
    `user_id` CHAR(36) NOT NULL,
    `email` TEXT,
    `last_login_at` DATETIME(6),
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`provider_id`, `subject`),
    UNIQUE KEY `user_identities_user_provider_key` (`user_id`, `provider_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `users` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `email` VARCHAR(255) NOT NULL,
    `password_hash` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255),
    `role` VARCHAR(50) DEFAULT 'user',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    `username` VARCHAR(255) NOT NULL,
    `is_initial_setup` BOOLEAN NOT NULL DEFAULT TRUE,
    `last_login_at` DATETIME(6),
    `last_login_ip` VARCHAR(45),
    `login_count` INT NOT NULL DEFAULT 0,
    `totp_secret` VARCHAR(255),
    `totp_enabled` BOOLEAN NOT NULL DEFAULT FALSE,
    `totp_verified_at` DATETIME(6),
    `backup_codes` TEXT,
    `language` VARCHAR(10) DEFAULT 'ko',
    `font_family` VARCHAR(100) DEFAULT 'system',
    `role_id` CHAR(36),
    `must_change_password` BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (`id`),
    UNIQUE KEY `users_email_key` (`email`),
    KEY `idx_users_totp_enabled` (`totp_enabled`),
    UNIQUE KEY `idx_users_email_lower` (((LOWER((`email`))))),
    UNIQUE KEY `users_username_key` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `waf_policy_history` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `rule_id` INT NOT NULL,
    `rule_category` VARCHAR(100),
    `rule_description` TEXT,
    `action` VARCHAR(20) NOT NULL,
    `reason` TEXT,
    `changed_by` VARCHAR(255),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_waf_policy_history_proxy_host` (`proxy_host_id`, `created_at` DESC),
    KEY `idx_waf_policy_history_rule` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `waf_rule_change_events` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `rule_id` VARCHAR(20) NOT NULL,
    `action` VARCHAR(20) NOT NULL,
    `rule_category` VARCHAR(255),
    `rule_description` TEXT,
    `reason` TEXT,
    `changed_by` VARCHAR(255),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_waf_rule_changes_proxy_host` (`proxy_host_id`, `created_at` DESC),
    KEY `idx_waf_rule_changes_rule` (`rule_id`, `created_at` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `waf_rule_exclusions` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36) NOT NULL,
    `rule_id` INT NOT NULL,
    `rule_category` VARCHAR(100),
    `rule_description` TEXT,
    `reason` TEXT,
    `disabled_by` VARCHAR(255),
    `scope_type` VARCHAR(16) NOT NULL DEFAULT 'host',
    `scope_value` VARCHAR(715) NOT NULL DEFAULT '',
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `waf_rule_exclusions_scope_key` (`proxy_host_id`, `rule_id`, `scope_type`, `scope_value`),
    KEY `idx_waf_rule_exclusions_proxy_host` (`proxy_host_id`),
    KEY `idx_waf_rule_exclusions_rule_id` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `waf_rule_snapshot_details` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `snapshot_id` CHAR(36) NOT NULL,
    `rule_id` VARCHAR(20) NOT NULL,
    `rule_category` VARCHAR(255),
    `rule_description` TEXT,
    `is_disabled` BOOLEAN DEFAULT FALSE,
    `reason` TEXT,
    PRIMARY KEY (`id`),
    KEY `idx_waf_snapshot_details_snapshot` (`snapshot_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `waf_rule_snapshots` (
    `id` CHAR(36) NOT NULL DEFAULT (UUID()),
    `proxy_host_id` CHAR(36),
    `version_number` INT NOT NULL,
    `snapshot_name` VARCHAR(255),
    `rule_engine` VARCHAR(20),
    `paranoia_level` INT,
    `anomaly_threshold` INT,
    `total_rules` INT DEFAULT 0,
    `disabled_rules` INT DEFAULT 0,
    `change_description` TEXT,
    `created_by` VARCHAR(255),
    `created_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_waf_snapshots_created_at` (`created_at` DESC),
    KEY `idx_waf_snapshots_proxy_host_version` (`proxy_host_id`, `version_number` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE `access_list_items` ADD CONSTRAINT `access_list_items_access_list_id_fkey` FOREIGN KEY (`access_list_id`) REFERENCES `access_lists` (`id`) ON DELETE CASCADE;

ALTER TABLE `api_token_usage` ADD CONSTRAINT `api_token_usage_token_id_fkey` FOREIGN KEY (`token_id`) REFERENCES `api_tokens` (`id`) ON DELETE CASCADE;

ALTER TABLE `api_tokens` ADD CONSTRAINT `api_tokens_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE;

ALTER TABLE `audit_logs` ADD CONSTRAINT `audit_logs_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL;

ALTER TABLE `auth_sessions` ADD CONSTRAINT `auth_sessions_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE;

ALTER TABLE `banned_ips` ADD CONSTRAINT `banned_ips_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `bot_filters` ADD CONSTRAINT `bot_filters_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `certificate_history` ADD CONSTRAINT `certificate_history_certificate_id_fkey` FOREIGN KEY (`certificate_id`) REFERENCES `certificates` (`id`) ON DELETE CASCADE;

ALTER TABLE `certificates` ADD CONSTRAINT `certificates_dns_provider_id_fkey` FOREIGN KEY (`dns_provider_id`) REFERENCES `dns_providers` (`id`) ON DELETE SET NULL;

ALTER TABLE `challenge_configs` ADD CONSTRAINT `challenge_configs_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `challenge_logs` ADD CONSTRAINT `challenge_logs_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `challenge_tokens` ADD CONSTRAINT `challenge_tokens_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE SET NULL;

ALTER TABLE `dashboard_stats_daily` ADD CONSTRAINT `dashboard_stats_daily_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `dashboard_stats_hourly` ADD CONSTRAINT `dashboard_stats_hourly_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `ddns_records` ADD CONSTRAINT `ddns_records_dns_provider_id_fkey` FOREIGN KEY (`dns_provider_id`) REFERENCES `dns_providers` (`id`) ON DELETE CASCADE;

ALTER TABLE `ddns_records` ADD CONSTRAINT `ddns_records_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `fail2ban_configs` ADD CONSTRAINT `fail2ban_configs_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `filter_subscription_entries` ADD CONSTRAINT `filter_subscription_entries_subscription_id_fkey` FOREIGN KEY (`subscription_id`) REFERENCES `filter_subscriptions` (`id`) ON DELETE CASCADE;

ALTER TABLE `filter_subscription_entry_exclusions` ADD CONSTRAINT `filter_subscription_entry_exclusions_subscription_id_fkey` FOREIGN KEY (`subscription_id`) REFERENCES `filter_subscriptions` (`id`) ON DELETE CASCADE;

ALTER TABLE `filter_subscription_host_exclusions` ADD CONSTRAINT `filter_subscription_host_exclusions_subscription_id_fkey` FOREIGN KEY (`subscription_id`) REFERENCES `filter_subscriptions` (`id`) ON DELETE CASCADE;

ALTER TABLE `filter_subscription_host_exclusions` ADD CONSTRAINT `filter_subscription_host_exclusions_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `geo_restrictions` ADD CONSTRAINT `geo_restrictions_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `global_exploit_rule_exclusions` ADD CONSTRAINT `global_exploit_rule_exclusions_rule_id_fkey` FOREIGN KEY (`rule_id`) REFERENCES `exploit_block_rules` (`id`) ON DELETE CASCADE;

ALTER TABLE `host_exploit_rule_exclusions` ADD CONSTRAINT `host_exploit_rule_exclusions_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `host_exploit_rule_exclusions` ADD CONSTRAINT `host_exploit_rule_exclusions_rule_id_fkey` FOREIGN KEY (`rule_id`) REFERENCES `exploit_block_rules` (`id`) ON DELETE CASCADE;

ALTER TABLE `ip_ban_history` ADD CONSTRAINT `ip_ban_history_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE SET NULL;

ALTER TABLE `ip_ban_history` ADD CONSTRAINT `ip_ban_history_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL;

ALTER TABLE `notification_outbox` ADD CONSTRAINT `notification_outbox_channel_id_fkey` FOREIGN KEY (`channel_id`) REFERENCES `notification_channels` (`id`) ON DELETE CASCADE;

ALTER TABLE `proxy_host_log_rules` ADD CONSTRAINT `proxy_host_log_rules_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `proxy_hosts` ADD CONSTRAINT `proxy_hosts_certificate_id_fkey` FOREIGN KEY (`certificate_id`) REFERENCES `certificates` (`id`) ON DELETE SET NULL;

ALTER TABLE `proxy_hosts` ADD CONSTRAINT `proxy_hosts_ddns_provider_id_fkey` FOREIGN KEY (`ddns_provider_id`) REFERENCES `dns_providers` (`id`) ON DELETE SET NULL;

ALTER TABLE `proxy_hosts` ADD CONSTRAINT `proxy_hosts_auth_provider_id_fkey` FOREIGN KEY (`auth_provider_id`) REFERENCES `auth_providers` (`id`) ON DELETE SET NULL;

ALTER TABLE `proxy_hosts` ADD CONSTRAINT `proxy_hosts_access_list_id_fkey` FOREIGN KEY (`access_list_id`) REFERENCES `access_lists` (`id`) ON DELETE SET NULL;

ALTER TABLE `rate_limits` ADD CONSTRAINT `rate_limits_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `redirect_hosts` ADD CONSTRAINT `redirect_hosts_certificate_id_fkey` FOREIGN KEY (`certificate_id`) REFERENCES `certificates` (`id`) ON DELETE SET NULL;

ALTER TABLE `role_permissions` ADD CONSTRAINT `role_permissions_role_id_fkey` FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE CASCADE;

ALTER TABLE `security_headers` ADD CONSTRAINT `security_headers_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `sso_login_states` ADD CONSTRAINT `sso_login_states_provider_id_fkey` FOREIGN KEY (`provider_id`) REFERENCES `sso_providers` (`id`) ON DELETE CASCADE;

ALTER TABLE `sso_providers` ADD CONSTRAINT `sso_providers_default_role_id_fkey` FOREIGN KEY (`default_role_id`) REFERENCES `roles` (`id`) ON DELETE RESTRICT;

ALTER TABLE `upstream_servers` ADD CONSTRAINT `upstream_servers_upstream_id_fkey` FOREIGN KEY (`upstream_id`) REFERENCES `upstreams` (`id`) ON DELETE CASCADE;

ALTER TABLE `upstreams` ADD CONSTRAINT `upstreams_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `uri_blocks` ADD CONSTRAINT `uri_blocks_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `user_identities` ADD CONSTRAINT `user_identities_provider_id_fkey` FOREIGN KEY (`provider_id`) REFERENCES `sso_providers` (`id`) ON DELETE CASCADE;

ALTER TABLE `user_identities` ADD CONSTRAINT `user_identities_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE;

ALTER TABLE `users` ADD CONSTRAINT `users_role_id_fkey` FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE RESTRICT;

ALTER TABLE `waf_policy_history` ADD CONSTRAINT `waf_policy_history_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `waf_rule_change_events` ADD CONSTRAINT `waf_rule_change_events_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `waf_rule_exclusions` ADD CONSTRAINT `waf_rule_exclusions_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

ALTER TABLE `waf_rule_snapshot_details` ADD CONSTRAINT `waf_rule_snapshot_details_snapshot_id_fkey` FOREIGN KEY (`snapshot_id`) REFERENCES `waf_rule_snapshots` (`id`) ON DELETE CASCADE;

ALTER TABLE `waf_rule_snapshots` ADD CONSTRAINT `waf_rule_snapshots_proxy_host_id_fkey` FOREIGN KEY (`proxy_host_id`) REFERENCES `proxy_hosts` (`id`) ON DELETE CASCADE;

CREATE OR REPLACE VIEW `logs_unified` AS SELECT `id`, `log_type`, `timestamp`, `host`, `client_ip`, `geo_country`, `geo_country_code`, `geo_city`, `geo_asn`, `geo_org`, `request_method`, `request_uri`, `request_protocol`, `status_code`, `body_bytes_sent`, `request_time`, `upstream_response_time`, `http_referer`, `http_user_agent`, `http_x_forwarded_for`, `severity`, `error_message`, `rule_id`, `rule_message`, `rule_severity`, `rule_data`, `attack_type`, `action_taken`, `block_reason`, `bot_category`, `proxy_host_id`, `raw_log`, `created_at`, `upstream_addr`, `upstream_status` FROM `logs_partitioned`;

-- 7 data statement(s) from 001_init.sql and 14 from the upgrade list run
-- through the translating driver after the DDL (see database/migration_mysql.go).
