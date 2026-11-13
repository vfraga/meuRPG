/** A zone as the announcer needs it: who it is and how it reads. */
export interface ZoneNote {
  readonly id: string;
  readonly label: string;
  /** The zone's letter in the list: it changes when a zone before it ends. */
  readonly letter: string;
}

function list(one: string, many: string, zones: readonly ZoneNote[]): string {
  if (zones.length === 0) {
    return '';
  }
  const names = zones.map((z) => `${z.letter}, ${z.label}`).join('; ');
  return zones.length === 1 ? `${one}: ${names}.` : `${many}: ${names}.`;
}

/**
 * What the polite live region says when the zones list changes: "Zona criada: B, Teia · Bartolo.", "Zona encerrada: ..." and, for a zone
 * that took another letter because one before it ended, "Letras novas: Teia · Bartolo agora é a zona A.".
 * `before` is `null` on the first read, which says nothing (the list is not news when the screen opens).
 */
export function zoneNotice(before: readonly ZoneNote[] | null, after: readonly ZoneNote[]): string {
  if (before === null) {
    return '';
  }
  const had = new Set(before.map((z) => z.id));
  const has = new Set(after.map((z) => z.id));
  const created = list(
    'Zona criada',
    'Zonas criadas',
    after.filter((z) => !had.has(z.id)),
  );
  const ended = list(
    'Zona encerrada',
    'Zonas encerradas',
    before.filter((z) => !has.has(z.id)),
  );
  const was = new Map(before.map((z) => [z.id, z.letter]));
  const moved = after
    .filter((z) => was.has(z.id) && was.get(z.id) !== z.letter)
    .map((z) => `${z.label} agora é a zona ${z.letter}`);
  const letters = moved.length > 0 ? `Letras novas: ${moved.join('; ')}.` : '';
  return [created, ended, letters].filter((t) => t !== '').join(' ');
}
