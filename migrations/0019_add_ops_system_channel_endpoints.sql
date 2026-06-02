ALTER TABLE ops_system_settings
    ADD COLUMN log_push_dingtalk_endpoint VARCHAR(512) NOT NULL DEFAULT '' AFTER log_push_phones,
    ADD COLUMN log_push_feishu_endpoint VARCHAR(512) NOT NULL DEFAULT '' AFTER log_push_dingtalk_endpoint,
    ADD COLUMN log_push_wecom_endpoint VARCHAR(512) NOT NULL DEFAULT '' AFTER log_push_feishu_endpoint,
    ADD COLUMN log_push_slack_endpoint VARCHAR(512) NOT NULL DEFAULT '' AFTER log_push_wecom_endpoint;

UPDATE ops_system_settings
SET
    log_push_dingtalk_endpoint = CASE WHEN log_push_dingtalk_endpoint IS NULL THEN '' ELSE TRIM(log_push_dingtalk_endpoint) END,
    log_push_feishu_endpoint = CASE WHEN log_push_feishu_endpoint IS NULL THEN '' ELSE TRIM(log_push_feishu_endpoint) END,
    log_push_wecom_endpoint = CASE WHEN log_push_wecom_endpoint IS NULL THEN '' ELSE TRIM(log_push_wecom_endpoint) END,
    log_push_slack_endpoint = CASE WHEN log_push_slack_endpoint IS NULL THEN '' ELSE TRIM(log_push_slack_endpoint) END
WHERE id = 1;
