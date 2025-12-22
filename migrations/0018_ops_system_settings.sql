-- +goose Up
CREATE TABLE IF NOT EXISTS ops_system_settings (
    id                  TINYINT UNSIGNED NOT NULL PRIMARY KEY DEFAULT 1,
    theme               VARCHAR(64) NOT NULL DEFAULT '',
    locale              VARCHAR(32) NOT NULL DEFAULT '',
    maintenance_mode    TINYINT(1) NOT NULL DEFAULT 0,
    maintenance_window  VARCHAR(128) NULL,
    announcement        TEXT NULL,
    updated_at          DATETIME NOT NULL,
    updated_by          VARCHAR(128) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO ops_system_settings (id, theme, locale, maintenance_mode, maintenance_window, announcement, updated_at, updated_by)
VALUES (1, '', '', 0, '', '', NOW(), 'system')
ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at);

-- +goose Down
DROP TABLE IF EXISTS ops_system_settings;
