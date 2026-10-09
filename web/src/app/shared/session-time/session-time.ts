const pad = (n: number) => String(n).padStart(2, '0');

/**
 * "30/09 às 20:05", in local time: when a live session started (the
 * campaign's "Sessão" panel, the master's status line). No year: an open
 * session is from today or yesterday. The spaces around "às" don't break.
 */
export function formatDayAt(date: Date): string {
  return `${pad(date.getDate())}/${pad(date.getMonth() + 1)} às ${formatClock(date)}`;
}

/** "21:14", in local time ("Última atualização às 21:14"). */
export function formatClock(date: Date): string {
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** "Em andamento desde 20:05" for a session that started today, with the day
 * ("… desde 30/09 às 20:05") when it started on another one (timeline.md,
 * shared decision 8): the line under "Sessão 5" for the master and the
 * players alike. */
export function sessionSince(startedAt: Date, now: Date = new Date()): string {
  const today =
    startedAt.getFullYear() === now.getFullYear() &&
    startedAt.getMonth() === now.getMonth() &&
    startedAt.getDate() === now.getDate();
  return `Em andamento desde ${today ? formatClock(startedAt) : formatDayAt(startedAt)}`;
}

const NBSP = ' ';
const WEEKDAYS = ['dom.', 'seg.', 'ter.', 'qua.', 'qui.', 'sex.', 'sáb.'];
const MONTHS = [
  'jan.',
  'fev.',
  'mar.',
  'abr.',
  'mai.',
  'jun.',
  'jul.',
  'ago.',
  'set.',
  'out.',
  'nov.',
  'dez.',
];
const MS_PER_MINUTE = 60_000;
const HOURS_PER_DAY = 24;
const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = HOURS_PER_DAY * MINUTES_PER_HOUR;
const DAY_MS = MINUTES_PER_DAY * MS_PER_MINUTE;

/** "19h05", "1h12": the hour as people say it, without a leading zero. */
function spokenClock(date: Date): string {
  return `${date.getHours()}h${pad(date.getMinutes())}`;
}

/** "qui., 8 de out." in local time, with " de 2025" when the year is not `now`'s. */
function spokenDay(date: Date, now: Date): string {
  const base = `${WEEKDAYS[date.getDay()]}, ${date.getDate()}${NBSP}de${NBSP}${MONTHS[date.getMonth()]}`;
  return date.getFullYear() === now.getFullYear()
    ? base
    : `${base}${NBSP}de${NBSP}${date.getFullYear()}`;
}

/** The calendar day of a date, as a number that compares and subtracts by days. */
function dayNumber(date: Date): number {
  return Math.round(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / DAY_MS);
}

/**
 * "qui., 8 de out., 19h05": the day (with the year when it is not this one)
 * and the hour a session started, in local time. The sentence of an open
 * session ("desde qui., 8 de out., 19h05") and the first half of
 * `formatSessionSpan`. The day's own words stay on one line.
 */
export function formatSessionStart(date: Date, now: Date = new Date()): string {
  return `${spokenDay(date, now)}, ${spokenClock(date)}`;
}

/**
 * "qui., 1 de out., 19h05 às 22h47": when an ended session ran, in local time.
 * One that crossed midnight says the day of its end after the end hour ("sex.,
 * 2 de out., 22h30 às 1h12 (sáb.)"; with the date too when it ended more than a
 * day later), and the year shows when it is not the current one.
 */
export function formatSessionSpan(start: Date, end: Date, now: Date = new Date()): string {
  const apart = dayNumber(end) - dayNumber(start);
  let tail = '';
  if (apart === 1) {
    tail = ` (${WEEKDAYS[end.getDay()]})`;
  } else if (apart > 1) {
    tail = ` (${spokenDay(end, start)})`;
  }
  return `${spokenDay(start, now)}, ${spokenClock(start)} às ${spokenClock(end)}${tail}`;
}
