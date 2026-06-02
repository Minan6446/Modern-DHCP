import type { ApiResponse, GlobalConfig, NotificationConfig } from '@/types/system';

const ok = <T>(data: T): ApiResponse<T> => ({ code: 0, message: '', data });

// Backend lacks /config endpoints; provide stubs to avoid 404s.
export const getGlobalConfig = async () => ok<GlobalConfig>({} as GlobalConfig);

export const updateGlobalConfig = async (_payload: Partial<GlobalConfig>) =>
  ok<GlobalConfig>({} as GlobalConfig);

export const getNotificationConfig = async () => ok<NotificationConfig>({} as NotificationConfig);

export const updateNotificationConfig = async (_payload: NotificationConfig) =>
  ok<NotificationConfig>({} as NotificationConfig);
