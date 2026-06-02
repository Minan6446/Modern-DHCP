const CSRF_COOKIE = 'csrf_token';

const hasCookie = () => {
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some((part) => part.trim().startsWith(`${CSRF_COOKIE}=`));
};

export const ensureCsrfCookie = async () => {
  // Backend sets csrf_token on login/refresh responses; no prefetch endpoint needed.
  if (hasCookie()) return;
  return;
};

export const getCsrfCookieName = () => CSRF_COOKIE;
