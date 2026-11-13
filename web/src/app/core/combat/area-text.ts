import { type AreaTarget, AreaPlacement, CoverDegree } from '../../../gen/meurpg/play/v1/combat_pb';
import { SpellAreaShape } from '../../../gen/meurpg/rules/v1/rules_pb';
import { tieNumbers, tight } from '../format/text';
import { SQUARE_FT, metersFixed, metersText, reachSquares } from '../units';
import { article } from './combat-log';
import { stateWord } from './combat-view';
import { listNames } from './joint-turn';
import { coverMark, degreeWord, sourceWord } from './cover';
import type { AreaShape, Direction } from './spell-area';
import { directionWord } from './spell-area';

/**
 * What the area picker says (PM-02a, PM-02b, PM-02d): the shape in words, the hint of each step, the out-of-range refusal,
 * the announcements, the ally warning and each row of "Quem está na área". Only what the caller was sent is named: a
 * player's preview never lists a hidden creature (RN-10), so nothing here can name one, and no sentence counts or hints at
 * what is not on the list. Pure functions, tested without a DOM.
 */

/** "esfera de 6 m de raio", "cone de 4,5 m", "linha de 30 m × 1,5 m", "cubo de 4,5 m". */
export function shapeText(area: AreaShape): string {
  const size = metersText(area.sizeFt);
  switch (area.shape) {
    case SpellAreaShape.SPHERE:
      return tight(`esfera de ${size} de raio`);
    case SpellAreaShape.CYLINDER:
      return tight(`cilindro de ${size} de raio`);
    case SpellAreaShape.CONE:
      return tight(`cone de ${size}`);
    case SpellAreaShape.LINE:
      return tight(`linha de ${size} × ${metersText(Math.max(area.widthFt, SQUARE_FT))}`);
    case SpellAreaShape.CUBE:
      return tight(`cubo de ${size}`);
    default:
      return '';
  }
}

/** "A Bola de Fogo", "O Relâmpago". */
export function theSpell(name: string): string {
  return `${article(name) === 'a' ? 'A' : 'O'} ${name}`;
}

/** The step's title: "Onde ela explode" for a point, "Para onde?" for a direction. */
export function stepOneTitle(placement: AreaPlacement): string {
  return placement === AreaPlacement.DIRECTION ? 'Para onde?' : 'Onde ela explode';
}

/** "Confirmar local" or "Confirmar direção". */
export function confirmLabel(placement: AreaPlacement): string {
  return placement === AreaPlacement.DIRECTION ? 'Confirmar direção' : 'Confirmar local';
}

/** "Centrar em…" for a point, "Apontar para…" for a direction. */
export function centerLabel(placement: AreaPlacement): string {
  return placement === AreaPlacement.DIRECTION ? 'Apontar para…' : 'Centrar em…';
}

/** The hint under the step's title before anything is placed. */
export function beforeHint(placement: AreaPlacement, area: AreaShape): string {
  if (placement === AreaPlacement.DIRECTION) {
    const out =
      area.shape === SpellAreaShape.CUBE ? 'O cubo encosta em você' : 'A área sai de você';
    return `Toque no mapa para dar a direção. ${out} e não inclui o seu quadrado.`;
  }
  const n = reachSquares(area.sizeFt);
  const word = area.shape === SpellAreaShape.CYLINDER ? 'O cilindro' : 'A esfera';
  return tight(
    `Toque no mapa ou arraste para escolher o ponto. ${word} tem ${metersText(area.sizeFt)} de raio (${n} ${n === 1 ? 'quadrado' : 'quadrados'}).`,
  );
}

/** The hint once a point or a direction is placed. */
export function placedHint(
  placement: AreaPlacement,
  distanceFt: number,
  direction: Direction | null,
): string {
  if (placement === AreaPlacement.DIRECTION) {
    return `Direção: ${direction ? directionWord(direction) : ''}. Toque de novo na mesma direção ou use “Confirmar direção”.`;
  }
  return tight(
    `Ponto a ${metersFixed(distanceFt)} de você. Toque de novo no mesmo ponto ou use “Confirmar local”.`,
  );
}

/** Why "Confirmar local" is not ready: nothing placed yet. */
export function notPlacedReason(placement: AreaPlacement): string {
  return placement === AreaPlacement.DIRECTION
    ? 'Toque no mapa para dar a direção.'
    : 'Toque no mapa para escolher o ponto.';
}

/** "A Bola de Fogo vai até 45 m; esse ponto está a 58,5 m.": the out-of-range refusal (the bold "Fora do alcance." goes before it). */
export function outOfRangeText(spell: string, rangeFt: number, distanceFt: number): string {
  return tight(
    `${theSpell(spell)} vai até ${metersText(rangeFt)}; esse ponto está a ${metersFixed(distanceFt)}.`,
  );
}

/** "4 criaturas", "1 criatura". */
export function creaturesText(n: number): string {
  return tight(`${n} ${n === 1 ? 'criatura' : 'criaturas'}`);
}

/**
 * What the live region says after a move: "Ponto a 7,5 m de você" (or "Direção: leste"), and ", 4 criaturas na área" only
 * once a preview for this very place answered, with the count of what the caller sees.
 */
export function announcement(
  placement: AreaPlacement,
  distanceFt: number,
  direction: Direction | null,
  seen: number | null,
): string {
  const where =
    placement === AreaPlacement.DIRECTION
      ? `Direção: ${direction ? directionWord(direction) : ''}`
      : `Ponto a ${metersFixed(distanceFt)} de você`;
  return tight(seen === null ? where : `${where}, ${creaturesText(seen)} na área`);
}

/** "Toren está na área.", "Você e 3 aliados estão na área": the warning a caster reads when the area holds their own side. */
export interface AllyWarning {
  readonly strong: string;
  readonly rest: string;
}

/** The player's warning: who of their side is in the area (the caster too), from the preview's `ally` and `self`. */
export function allyWarning(
  targets: readonly AreaTarget[],
  spell: string,
  casterLabel: string,
): AllyWarning | null {
  const allies = targets.filter((t) => t.ally && !t.self).map((t) => t.label);
  const self = targets.some((t) => t.self);
  if (!self && allies.length === 0) {
    return null;
  }
  const spellThe = theSpell(spell);
  if (self && allies.length === 0) {
    return {
      strong: `${casterLabel} (você) está na área.`,
      rest: `${spellThe} atinge você também. Mude o local se não quiser se atingir.`,
    };
  }
  if (self) {
    const n = allies.length;
    return {
      strong: `Você e ${n} ${n === 1 ? 'aliado' : 'aliados'} estão na área`,
      rest: tieNumbers(`(${listNames(allies)}).`),
    };
  }
  const one = allies.length === 1;
  const them = one ? (article(allies[0]) === 'a' ? 'atingi-la' : 'atingi-lo') : 'atingi-los';
  return {
    strong: tieNumbers(`${listNames(allies)} ${one ? 'está' : 'estão'} na área.`),
    rest: `${spellThe} atinge aliados também. Mude o local se não quiser ${them}.`,
  };
}

/** The master's warning: the NPC caster's allies in its own area ("Goblin 1 e Goblin 2 são aliados do Zuk."). */
export function masterAllyWarning(
  targets: readonly AreaTarget[],
  casterLabel: string,
): AllyWarning | null {
  const allies = targets.filter((t) => t.ally && !t.self).map((t) => t.label);
  const self = targets.some((t) => t.self);
  if (allies.length === 0 && !self) {
    return null;
  }
  const of = `${article(casterLabel) === 'a' ? 'da' : 'do'} ${casterLabel}`;
  const parts: string[] = [];
  if (allies.length > 0) {
    parts.push(`${listNames(allies)} ${allies.length === 1 ? 'é aliado' : 'são aliados'} ${of}.`);
  }
  if (self) {
    parts.push(`${casterLabel} está na área.`);
  }
  return { strong: tieNumbers(parts.join(' ')), rest: 'A magia atinge todos na área.' };
}

/** One row of "Quem está na área". */
export interface AreaRow {
  readonly id: string;
  readonly label: string;
  /** "Ileso", "Ferido": never hit points (RN-20). */
  readonly state: string;
  readonly ally: boolean;
  readonly self: boolean;
  /** The master's only: the creature is hidden from the players. */
  readonly hidden: boolean;
  /** "Três quartos: +5 no teste de Destreza", "Sem cobertura", "Sem bônus de cobertura: o teste é de Constituição". */
  readonly cover: string;
  readonly mark: 'half' | 'three' | null;
  /** "a 3,0 m do ponto"; empty for the caster itself. */
  readonly distance: string;
}

/** The rows of the step-2 list, in the server's order. `ability` is the save's ability ("Constituição"), for a spell
 * whose save cover does not help (`coverCounts` false). */
export function areaRows(
  targets: readonly AreaTarget[],
  coverCounts: boolean,
  ability: string,
  placement: AreaPlacement,
): AreaRow[] {
  return targets.map((t) => {
    const c = coverOf(t.cover, t.coverSource, coverCounts, ability);
    const from = placement === AreaPlacement.POINT ? 'do ponto' : 'de você';
    return {
      id: t.combatantId,
      label: t.self ? `${t.label} (você)` : t.label,
      state: stateWord(t.state, t.label),
      ally: t.ally && !t.self,
      self: t.self,
      hidden: t.hidden,
      cover: c.text,
      mark: c.mark,
      distance: t.self ? '' : tight(`a ${metersFixed(t.distanceFt)} ${from}`),
    };
  });
}

/** "4 criaturas · 1 escondida" (the master's count); a player's is only the creatures. */
export function countText(targets: readonly AreaTarget[]): string {
  const hidden = targets.filter((t) => t.hidden).length;
  const base = creaturesText(targets.length);
  return hidden > 0 ? `${base} · ${hidden} ${hidden === 1 ? 'escondida' : 'escondidas'}` : base;
}

/** "Ninguém que você vê está na área. A magia gasta o espaço de 3º nível e a sua ação mesmo assim." The "que você vê" stays
 * whatever the area holds: a hidden creature may be in it, and this sentence must not tell. */
export function nobodyText(
  grid: boolean,
  slotLevel: number,
  bonusAction: boolean,
): { strong: string; rest: string } {
  const spent = bonusAction ? 'a sua ação bônus' : 'a sua ação';
  const slot = slotLevel > 0 ? `o espaço de ${slotLevel}º nível e ` : '';
  return {
    strong: grid ? 'Ninguém que você vê está na área.' : 'Ninguém está marcado.',
    rest: `A magia gasta ${slot}${spent} mesmo assim.`,
  };
}

/**
 * A target's cover against the point of origin, in words: "Três quartos (do mapa): +5 no teste de Destreza", "Sem cobertura";
 * and, for a spell whose save is not Dexterity, "Sem bônus de cobertura: o teste é de Constituição" (SRD 5.1, "Cover": the
 * bonus is to Dexterity saving throws only).
 */
export function coverOf(
  degree: CoverDegree,
  source: Parameters<typeof sourceWord>[0],
  coverCounts: boolean,
  ability: string,
): { text: string; mark: 'half' | 'three' | null } {
  if (!coverCounts) {
    return {
      text: ability ? `Sem bônus de cobertura: o teste é de ${ability}` : 'Sem bônus de cobertura',
      mark: null,
    };
  }
  const word = degreeWord(degree);
  if (!word || degree === CoverDegree.TOTAL) {
    return { text: 'Sem cobertura', mark: null };
  }
  const from = sourceWord(source);
  const bonus = degree === CoverDegree.HALF ? '+2' : '+5';
  return {
    text: `${word}${from ? ` (${from})` : ''}: ${bonus} no teste de Destreza`,
    mark: coverMark(degree),
  };
}

/** The map's name for a reader of the screen: "Mapa: escolha o ponto da Bola de Fogo", the direction, or a wall's start or center. */
export function mapLabelOf(
  spell: string,
  placement: AreaPlacement,
  wall: { readonly ring: boolean } | null,
): string {
  if (wall) {
    return `Mapa: escolha o ${wall.ring ? 'centro' : 'começo'} da ${spell}`;
  }
  return placement === AreaPlacement.DIRECTION
    ? `Mapa: escolha a direção da ${spell}`
    : `Mapa: escolha o ponto da ${spell}`;
}
