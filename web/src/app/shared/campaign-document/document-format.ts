import { formatClock, formatDayAt } from '../session-time/session-time';

/** "hoje às 22:10", "ontem às 22:10" or "28/09 às 22:10" (with the year
 * when it is not this one), in local time. */
export function whenText(date: Date, now: Date = new Date()): string {
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((day(now) - day(date)) / 86_400_000);
  if (days === 0) {
    return `hoje às ${formatClock(date)}`;
  }
  if (days === 1) {
    return `ontem às ${formatClock(date)}`;
  }
  const text = formatDayAt(date).replace(/\u00a0/g, ' ');
  return date.getFullYear() === now.getFullYear()
    ? text
    : `${text.replace(' às', `/${date.getFullYear()} às`)}`;
}

/** "Editado por Samuel ontem às 22:10." / "Editado ontem às 22:10." / "Ainda
 * não foi salvo." */
export function editedLine(updatedAt: Date | null, by: string, now: Date = new Date()): string {
  if (updatedAt === null) {
    return 'Ainda não foi salvo.';
  }
  const who = by ? ` por ${by}` : '';
  return `Editado${who} ${whenText(updatedAt, now)}.`;
}
