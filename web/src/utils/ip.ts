const ipv4Regex = /^(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)){3}$/;
const ipv6Regex =
  /^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(::)|(([0-9a-fA-F]{1,4}:){1,7}:)|(:{2}([0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}))$/;

export const isIPv4 = (ip: string) => ipv4Regex.test(ip.trim());
export const isIPv6 = (ip: string) => ipv6Regex.test(ip.trim());

export const formatIPv4 = (ip: string) => (isIPv4(ip) ? ip.trim() : ip);

export const formatIPv6 = (ip: string) => (isIPv6(ip) ? ip.trim().toLowerCase() : ip);

export const ipv4ToInt = (ip: string): number =>
  ip
    .split('.')
    .map((x) => parseInt(x, 10))
    .reduce((acc, oct) => (acc << 8) + oct, 0) >>> 0;

export const intToIpv4 = (num: number): string =>
  [24, 16, 8, 0].map((shift) => (num >>> shift) & 255).join('.');

export const cidrMask = (prefix: number) => (prefix === 0 ? 0 : 0xffffffff << (32 - prefix)) >>> 0;

export const calcIPv4Range = (cidr: string) => {
  const [ip, prefixStr] = cidr.split('/');
  const prefix = Number(prefixStr);
  if (!isIPv4(ip) || isNaN(prefix) || prefix < 0 || prefix > 32) return null;
  const ipInt = ipv4ToInt(ip);
  const mask = cidrMask(prefix);
  const network = ipInt & mask;
  const broadcast = network + (0xffffffff >>> prefix);
  const firstHost = prefix === 32 ? network : network + 1;
  const lastHost = prefix >= 31 ? broadcast : broadcast - 1;
  const hostCount =
    prefix >= 31 ? Math.max(1, broadcast - network + 1) : Math.max(0, lastHost - firstHost + 1);
  return {
    network: intToIpv4(network),
    broadcast: intToIpv4(broadcast),
    firstHost: intToIpv4(firstHost),
    lastHost: intToIpv4(lastHost),
    hostCount
  };
};

export const validateRangeWithin = (cidr: string, ip: string) => {
  const range = calcIPv4Range(cidr);
  if (!range) return false;
  const val = ipv4ToInt(ip);
  const start = ipv4ToInt(range.network);
  const end = ipv4ToInt(range.broadcast);
  return val >= start && val <= end;
};

export const cidrOverlap = (a: string, b: string) => {
  const ra = calcIPv4Range(a);
  const rb = calcIPv4Range(b);
  if (!ra || !rb) return false;
  const aStart = ipv4ToInt(ra.network);
  const aEnd = ipv4ToInt(ra.broadcast);
  const bStart = ipv4ToInt(rb.network);
  const bEnd = ipv4ToInt(rb.broadcast);
  return Math.max(aStart, bStart) <= Math.min(aEnd, bEnd);
};

export const dedupeIps = (ips: string[]) =>
  Array.from(new Set(ips.map((i) => i.trim()).filter(Boolean)));

export const validateIPv6Prefix = (prefix: string) => {
  const parts = prefix.split('/');
  if (parts.length !== 2) return false;
  const [addr, lenStr] = parts;
  const len = Number(lenStr);
  return isIPv6(addr) && !isNaN(len) && len >= 0 && len <= 128;
};

export const hasConflict = (ips: string[]) => {
  const seen = new Set<string>();
  for (const ip of ips.map((i) => i.trim()).filter(Boolean)) {
    if (seen.has(ip)) return true;
    seen.add(ip);
  }
  return false;
};

export const summarizeSubnets = (cidrs: string[]) => {
  const ranges = cidrs.map((c) => calcIPv4Range(c)).filter(Boolean) as Array<
    ReturnType<typeof calcIPv4Range>
  >;
  const totalHosts = ranges.reduce((acc, r) => acc + (r?.hostCount || 0), 0);
  return { count: ranges.length, totalHosts };
};
