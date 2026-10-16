import type { SlotUsageVm } from '../play/resource-usage.types';
import { circleLabel } from '../combat/combat-grid';
import { type Pool, pointsText } from './pools';

/** The highest slot Flexible Casting creates (SRD 5.1, Sorcerer). */
export const MAX_CREATED_SLOT_LEVEL = 5;

/**
 * The SRD's "Creating Spell Slots" table: the sorcery points a slot of each level costs. The same table the server
 * spends from (the protos do not carry it before the call); the answer of `CreateSpellSlot` carries the cost the
 * server took, which is what the sheet says afterwards.
 */
export const SLOT_CREATION_COSTS: Readonly<Record<number, number>> = {
  1: 2,
  2: 3,
  3: 5,
  4: 6,
  5: 7,
};

/** A row of a Flexible Casting list. */
export interface FlexRow {
  readonly level: number;
  readonly title: string;
  /** "custa 3 pontos", "3 livres de 4 · +1 ponto". */
  readonly sub: string;
  /** Why it cannot be chosen: "custa 6 pontos (você tem 5)"; empty when it can. */
  readonly blocked: string;
}

/** The five levels of "Pontos → espaço", each with its cost; the ones the points do not pay for are disabled with the
 * reason written ("custa 6 pontos (você tem 5)"). */
export function createRows(points: Pool): FlexRow[] {
  return Array.from({ length: MAX_CREATED_SLOT_LEVEL }, (_, i) => i + 1).map((level) => {
    const cost = SLOT_CREATION_COSTS[level];
    const short = cost > points.left;
    return {
      level,
      title: `Criar um espaço de ${circleLabel(level)}`,
      sub: short ? '' : `custa ${pointsText(cost)}`,
      blocked: short ? `custa ${pointsText(cost)} (você tem ${points.left})` : '',
    };
  });
}

/** The free slots of "Espaço → pontos": each gives its level in points. */
export function convertRows(slots: readonly SlotUsageVm[]): FlexRow[] {
  return slots
    .filter((s) => s.total - s.used > 0)
    .map((s) => {
      const free = s.total - s.used;
      return {
        level: s.level,
        title: circleLabel(s.level),
        sub: `${free} ${free === 1 ? 'livre' : 'livres'} de ${s.total} · +${pointsText(s.level)}`,
        blocked: '',
      };
    });
}

/** "Gasta 3 pontos (restam 2) e cria um espaço de 2º nível. O espaço criado some no descanso longo. Ação bônus." */
export function createPreview(level: number, points: Pool): string {
  const cost = SLOT_CREATION_COSTS[level];
  return `Gasta ${cost} pontos (restam ${points.left - cost}) e cria um espaço de ${circleLabel(level)}. O espaço criado some no descanso longo. Ação bônus.`;
}

/** "Gasta um espaço de 2º nível e ganha 2 pontos de feitiçaria: ficam em 4 de 5. Ação bônus." */
export function convertPreview(level: number, points: Pool): string {
  return `Gasta um espaço de ${circleLabel(level)} e ganha ${pointsText(level)} de feitiçaria: ficam em ${Math.min(points.total, points.left + level)} de ${points.total}. Ação bônus.`;
}

/** The refusal drawn before asking: the sorcerer already holds the most points (the sorcerer's level), so a
 * conversion would only lose them. The server refuses the same way, with the same words. */
export function fullText(points: Pool): string {
  return `Você já tem o máximo de pontos de feitiçaria. O máximo é o seu nível (${points.total}): converter um espaço agora perderia os pontos.`;
}
