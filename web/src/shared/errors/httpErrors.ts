export enum HttpErrorMessage {
  BadRequest = '请求参数有误',
  Unauthorized = '未授权，请重新登录',
  Forbidden = '没有权限执行此操作',
  NotFound = '资源不存在或已删除',
  Conflict = '请求冲突，HA 防护拦截',
  Locked = '当前为备用节点，无法执行写操作',
  Timeout = '请求超时，请稍后重试',
  Server = '服务异常，请稍后重试',
  Network = '网络异常，请检查连接',
  Default = '请求失败，请稍后重试'
}

const isNetworkError = (message?: string) => !!message && message.toLowerCase().includes('network');

export const mapHttpError = (status?: number, message?: string) => {
  if (status === 400) return HttpErrorMessage.BadRequest;
  if (status === 401) return HttpErrorMessage.Unauthorized;
  if (status === 403) return HttpErrorMessage.Forbidden;
  if (status === 404) return HttpErrorMessage.NotFound;
  if (status === 409) return HttpErrorMessage.Conflict;
  if (status === 423) return HttpErrorMessage.Locked;
  if (status === 408) return HttpErrorMessage.Timeout;
  if (status && status >= 500) return HttpErrorMessage.Server;
  if (isNetworkError(message)) return HttpErrorMessage.Network;
  return HttpErrorMessage.Default;
};
