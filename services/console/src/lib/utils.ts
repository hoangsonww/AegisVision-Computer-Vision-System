import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { formatDistanceToNow, format } from 'date-fns';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function fmtBytes(b: number) {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = b;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 ? 2 : 1)} ${units[i]}`;
}

export function fmtNumber(n: number, digits = 1) {
  if (Math.abs(n) < 1000) return n.toFixed(digits);
  const units = ['k', 'M', 'B', 'T'];
  let i = -1;
  let v = n;
  while (Math.abs(v) >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v.toFixed(digits)}${units[i]}`;
}

export function fmtDuration(ms: number) {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`;
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)} min`;
  return `${(ms / 3_600_000).toFixed(1)} h`;
}

export function fmtRelative(t: Date | string | number) {
  try {
    return formatDistanceToNow(new Date(t), { addSuffix: true });
  } catch {
    return String(t);
  }
}

export function fmtAbsolute(t: Date | string | number) {
  try {
    return format(new Date(t), 'yyyy-MM-dd HH:mm:ss');
  } catch {
    return String(t);
  }
}

export function pretty(obj: unknown) {
  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
}

export function severityClass(sev: string | undefined) {
  switch (sev?.replace(/^SEVERITY_/, '').toUpperCase()) {
    case 'CRITICAL':
    case 'HIGH':
      return 'pill-danger';
    case 'MEDIUM':
      return 'pill-warn';
    case 'LOW':
    case 'INFO':
      return 'pill';
    default:
      return 'pill';
  }
}
