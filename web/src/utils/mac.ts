const macRegex = /^([0-9A-Fa-f]{2}([-:])){5}([0-9A-Fa-f]{2})$/;

export const isMac = (mac: string) => macRegex.test(mac.trim());

export const formatMac = (mac: string, separator = ':') => {
  const clean = mac.replace(/[^0-9A-Fa-f]/g, '').toUpperCase();
  if (clean.length !== 12) return mac;
  return clean.match(/.{1,2}/g)?.join(separator) || mac;
};

export const normalizeMac = (mac: string) => formatMac(mac, ':');

export const conflictMac = (macs: string[]) => {
  const set = new Set<string>();
  for (const m of macs.map((x) => normalizeMac(x))) {
    if (set.has(m)) return true;
    set.add(m);
  }
  return false;
};
