import { article } from '../../../../core/combat/combat-log';

/** The four groups of a surprised character's turn, each unavailable for the same reason (SRD 5.1, Surprise). */
export const SURPRISE_ROWS: readonly { readonly name: string; readonly why: string }[] = [
  { name: 'Atacar', why: 'Surpresa.' },
  { name: 'Movimento', why: 'Surpresa.' },
  { name: 'Ação bônus', why: 'Surpresa.' },
  { name: 'Reação', why: 'Surpresa até o fim do turno.' },
];

/** "Você volta a ser a Sálvia, com os seus PV. Os PV que sobraram do Lobo se perdem.": the character and the beast, by name. */
export function leaveDetailOf(name: string, beast: string): string {
  const a = (n: string) => (article(n) === 'a' ? 'a' : 'o');
  return `Você volta a ser ${a(name)} ${name}, com os seus PV. Os PV que sobraram d${a(beast)} ${beast} se perdem.`;
}
