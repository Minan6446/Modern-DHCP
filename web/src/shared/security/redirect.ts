const defaultWhitelist = ['/', '/dashboard', '/overview', '/pools', '/leases', '/monitoring'];

const isRelativePath = (path?: string) => !!path && path.startsWith('/') && !path.startsWith('//');
const hasProtocol = (path?: string) => !!path && /^https?:\/\//i.test(path);

export const sanitizeRedirect = (target?: string, extraWhitelist: string[] = []): string => {
  if (!target) return '/';
  if (hasProtocol(target)) return '/';
  if (!isRelativePath(target)) return '/';
  const whitelist = new Set([...defaultWhitelist, ...extraWhitelist]);
  if ([...whitelist].some((prefix) => target === prefix || target.startsWith(`${prefix}/`)))
    return target;
  console.warn('Blocked unsafe redirect', target);
  return '/';
};
