import type { PreviewSpellAreaResponse } from '../../../gen/meurpg/play/v1/combat_pb';
import { tight } from '../format/text';
import { metersText } from '../units';
import { listNames } from './joint-turn';
import type { WallChoice } from './spell-wall';

/** The card under a ring's controls: what the server laid and who it names in it. */
export interface WallSummary {
  /** "Anel de 6 m · 16 quadrados". */
  readonly title: string;
  /** "Na muralha agora: Kobold 2 (teste de Destreza)." */
  readonly who: string;
}

/**
 * What the server's preview says of a ring (board 640-04): its diameter and the squares it laid, and the creatures it names
 * in them (only the ones the caller sees, as the server sent them: nothing here counts or hints at who is not named). `ability`
 * is the spell's save ("Destreza"). `null` for a straight wall (its note already counts the squares) and before an answer.
 */
export function wallSummary(
  wall: WallChoice | null,
  preview: PreviewSpellAreaResponse | null,
  ability: string,
): WallSummary | null {
  if (!wall?.ring || !preview) {
    return null;
  }
  const n = preview.squares.length;
  const title = tight(
    `Anel de ${metersText(preview.lengthFt || wall.lengthFt)} · ${n} ${n === 1 ? 'quadrado' : 'quadrados'}`,
  );
  const save = ability ? `teste de ${ability}` : '';
  const names = preview.targets.map((t) => {
    const notes = [t.self ? 'você' : '', save].filter(Boolean).join(', ');
    return notes ? `${t.label} (${notes})` : t.label;
  });
  return {
    title,
    who: names.length
      ? `Na muralha agora: ${listNames(names)}.`
      : 'Ninguém que você vê está na muralha agora.',
  };
}
