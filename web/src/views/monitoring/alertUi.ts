import { i18n } from '@/i18n';

export type AlertLevel = 'emergency' | 'critical' | 'warning' | 'info';

const t = (key: string) => (i18n.global as any).t(key);

export const ALERT_LEVEL_LABEL = new Proxy({} as Record<AlertLevel, string>, {
  get(_target, prop: string) {
    const key = `monitoring.alertLevel.${prop}`;
    return t(key);
  }
});

export const ALERT_LEVEL_COLOR: Record<AlertLevel, string> = {
  emergency: '#f56c6c',
  critical: '#e6a23c',
  warning: '#409eff',
  info: '#909399'
};

export const ALERT_LEVEL_TAG_TYPE: Record<AlertLevel, 'danger' | 'warning' | 'primary' | 'info'> = {
  emergency: 'danger',
  critical: 'warning',
  warning: 'primary',
  info: 'info'
};

export const levelLabel = (level: AlertLevel) => ALERT_LEVEL_LABEL[level] || t('monitoring.alertLevel.info');
export const levelColor = (level: AlertLevel) => ALERT_LEVEL_COLOR[level] || ALERT_LEVEL_COLOR.info;
export const levelTagType = (level: AlertLevel) => ALERT_LEVEL_TAG_TYPE[level] || ALERT_LEVEL_TAG_TYPE.info;
