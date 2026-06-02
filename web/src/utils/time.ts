import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import isLeapYear from 'dayjs/plugin/isLeapYear';

dayjs.extend(relativeTime);
dayjs.extend(isLeapYear);

export const formatTs = (ts?: string) => (ts ? dayjs(ts).format('YYYY-MM-DD HH:mm:ss') : '');

export const calcT1T2 = (leaseTime: number) => {
  const t1 = Math.floor(leaseTime * 0.5);
  const t2 = Math.floor(leaseTime * 0.875);
  return { t1, t2 };
};

export const timeLeft = (ts?: string) => {
  if (!ts) return '';
  const diff = dayjs(ts).diff(dayjs(), 'second');
  if (diff < 0) return 'expired';
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m`;
  return `${Math.floor(diff / 3600)}h`;
};

export const formatRelative = (ts: string) => dayjs(ts).fromNow();

export const formatRange = (from: string, to: string) => `${formatTs(from)} ~ ${formatTs(to)}`;

export const isHoliday = (ts: string) => {
  const d = dayjs(ts);
  const day = d.day();
  return day === 0 || day === 6;
};

export const nextBusinessDay = (ts: string) => {
  let d = dayjs(ts).add(1, 'day');
  while (isHoliday(d.toISOString())) d = d.add(1, 'day');
  return d.toISOString();
};
