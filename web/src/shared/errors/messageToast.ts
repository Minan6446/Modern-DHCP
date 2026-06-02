import { ElMessage } from 'element-plus';

type MessageParam = Parameters<typeof ElMessage.success>[0];

export const showSuccess = (message: MessageParam) => {
  ElMessage.success(message as any);
};

export const showWarning = (message: MessageParam) => {
  ElMessage.warning(message as any);
};

export const showInfo = (message: MessageParam) => {
  ElMessage.info(message as any);
};

export const showError = (message: MessageParam) => {
  ElMessage.error(message as any);
};
