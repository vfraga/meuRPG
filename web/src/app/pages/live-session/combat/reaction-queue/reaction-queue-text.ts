import type {
  ConcentrationSaveResult,
  ReactionWindow,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { EffectPhase } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type {
  ConcentrationAnswer,
  ReactionAnswer,
} from '../../../../core/combat/combat-client-types';
import { rollText } from '../../../../core/combat/combat-dice';
import { ofThe } from '../../../../core/combat/move-plan';
import {
  type ConcentrationMode,
  concentrationMode as modeOfSave,
  concentrationModeLine as modeLineOfSave,
} from '../../../../core/combat/reactions';
import type { EffectSaveAnswer } from '../../../../core/effects/effects-client';

// What the master's cards say about a window and about the answer just given. Pure: the component only picks the window.

/** "+3" or "−2" (the true minus sign the table uses). */
export function signed(n: number): string {
  return n < 0 ? `−${Math.abs(n)}` : `+${n}`;
}

// ---- an effect's saving throw ----

function effectSavePrompt(w: ReactionWindow) {
  return w.prompt.case === 'effectSave' ? w.prompt.value : null;
}

export function effectSaveMode(w: ReactionWindow): 'normal' | 'advantage' | 'disadvantage' {
  const mode = effectSavePrompt(w)?.mode;
  return mode === 'advantage' || mode === 'disadvantage' ? mode : 'normal';
}

export function effectSavePairHint(w: ReactionWindow): string {
  const counts =
    effectSaveMode(w) === 'advantage'
      ? 'Com vantagem conta o maior'
      : 'Com desvantagem conta o menor';
  return `${counts}. Role os dois d20 e digite os números que saíram nos dados (1 a 20 cada), sem somar o bônus.`;
}

export function effectSaveAbility(w: ReactionWindow): string {
  return effectSavePrompt(w)?.abilityNamePt ?? '';
}

/** "Teste de resistência de Sabedoria do Goblin 1 · Riso Histérico · CD 13". */
export function effectSaveTitle(w: ReactionWindow): string {
  const p = effectSavePrompt(w);
  const whose = w.reactorIsPlayer ? `de ${w.reactorLabel}` : ofThe([w.reactorLabel]);
  const parts = [
    `Teste de resistência de ${p?.abilityNamePt ?? ''} ${whose}`,
    p?.sourceNamePt ?? '',
    p?.dc !== undefined ? `CD ${p.dc}` : '',
  ];
  return parts.filter((x) => x !== '').join(' · ');
}

export function effectSaveText(w: ReactionWindow): string {
  const p = effectSavePrompt(w);
  const when = p?.phase === EffectPhase.START ? 'no começo' : 'no fim';
  return `${p?.sourceNamePt ?? 'Um efeito'} pede este teste ${when} da vez ${ofThe([w.reactorLabel])}. ${p?.textPt ?? ''}`.trim();
}

export function effectSaveRule(w: ReactionWindow): string {
  const p = effectSavePrompt(w);
  if (!p) {
    return '';
  }
  const bonus = p.bonusKnown ? `${p.abilityNamePt}: ${signed(p.modifier)}` : 'Bônus desconhecido';
  return [bonus, p.mode].filter((x) => x !== '').join(' · ');
}

export function effectSaveWho(w: ReactionWindow): string {
  return w.reactorIsPlayer ? `O jogador de ${w.reactorLabel} deixou a rolagem com você.` : '';
}

export function effectSaveBonus(w: ReactionWindow): number {
  const p = effectSavePrompt(w);
  return p?.bonusKnown ? p.modifier : 0;
}

export function effectSaveSkippable(w: ReactionWindow): boolean {
  return effectSavePrompt(w)?.skippable ?? false;
}

/** What the live region says once the master answered an effect's saving throw. */
export function effectSaveSaid(w: ReactionWindow, how: EffectSaveAnswer): string {
  return how.kind === 'skip'
    ? `Teste pulado ${ofThe([w.reactorLabel])}.`
    : `Teste de resistência ${ofThe([w.reactorLabel])} resolvido.`;
}

// ---- the second steps ----

function rebukePrompt(w: ReactionWindow) {
  return w.prompt.case === 'hellishRebukeSave' ? w.prompt.value : null;
}

export function rebukeTitle(w: ReactionWindow): string {
  return `Repreensão Infernal ${w.reactorIsPlayer ? `de ${w.reactorLabel}` : ofThe([w.reactorLabel])}`;
}

export function rebukeAggressor(w: ReactionWindow): string {
  return rebukePrompt(w)?.aggressorLabel ?? '';
}

export function rebukeText(w: ReactionWindow): string {
  const p = rebukePrompt(w);
  return p
    ? `${p.aggressorLabel} sofre ${p.diceCount}d10 de fogo, ou metade se passar no teste de Destreza contra CD ${p.saveDc}.`
    : '';
}

export function rebukeBonus(w: ReactionWindow): string {
  const p = rebukePrompt(w);
  return p
    ? `Destreza ${ofThe([p.aggressorLabel])}: ${p.bonusKnown ? signed(p.saveBonus) : 'desconhecida'} · o teste é seu`
    : '';
}

export function rebukeModifier(w: ReactionWindow): number {
  const p = rebukePrompt(w);
  return p?.bonusKnown ? p.saveBonus : 0;
}

// ---- the concentration save ----

function concentrationPrompt(w: ReactionWindow) {
  return w.prompt.case === 'concentrationSave' ? w.prompt.value : null;
}

export function concentrationTitle(w: ReactionWindow): string {
  const damage = w.trigger?.damageTaken ?? 0;
  return `${w.reactorLabel} sofreu ${damage} de dano`;
}

function concentrationSpell(w: ReactionWindow): string {
  return concentrationPrompt(w)?.spellNamePt ?? w.trigger?.concentrationSpellKey ?? '';
}

export function concentrationDc(w: ReactionWindow): number {
  return concentrationPrompt(w)?.dc ?? 10;
}

export function concentrationLead(w: ReactionWindow): string {
  return `${w.reactorLabel} concentra em ${concentrationSpell(w)}.`;
}

export function concentrationRule(w: ReactionWindow): string {
  const damage = w.trigger?.damageTaken ?? 0;
  const p = concentrationPrompt(w);
  const bonus = p?.bonusKnown
    ? ` · Constituição ${ofThe([w.reactorLabel])}: ${signed(p.saveBonus)}`
    : '';
  return `Teste de resistência de Constituição contra CD ${concentrationDc(w)} (metade de ${damage} seria ${Math.floor(damage / 2)}; o mínimo é 10)${bonus}`;
}

export function concentrationMode(w: ReactionWindow): ConcentrationMode {
  const p = concentrationPrompt(w);
  return p ? modeOfSave(p) : 'normal';
}

/** How many d20 a typed save takes: two with advantage or disadvantage. */
export function concentrationDice(w: ReactionWindow): number {
  return concentrationMode(w) === 'normal' ? 1 : 2;
}

export function concentrationModeLine(w: ReactionWindow): string {
  const p = concentrationPrompt(w);
  return p ? modeLineOfSave(p, w.reactorLabel) : '';
}

export function concentrationBonus(w: ReactionWindow): number {
  const p = concentrationPrompt(w);
  return p?.bonusKnown ? p.saveBonus : 0;
}

export function bonusWord(w: ReactionWindow): string {
  const bonus = concentrationBonus(w);
  return `${bonus < 0 ? '−' : '+'} ${Math.abs(bonus)}`;
}

/** What the live region says once the master answered a concentration save. */
export function concentrationSaid(
  w: ReactionWindow,
  how: ConcentrationAnswer,
  r: ConcentrationSaveResult | undefined,
): string {
  if (r) {
    const roll = r.save ? `${rollText(r.save)} ` : '';
    const spell = r.spellNamePt ? ` em ${r.spellNamePt}` : '';
    return `${roll}contra CD ${r.dc}: ${w.reactorLabel} ${r.kept ? 'manteve' : 'perdeu'} a concentração${spell}.`;
  }
  return how.kind === 'keep' ? `Concentração mantida ${ofThe([w.reactorLabel])}.` : 'Respondido.';
}

// ---- a reaction answered ----

/** What the live region says once the master answered a reaction window. */
export function sayAnswer(
  w: ReactionWindow,
  answer: ReactionAnswer,
  result: { readonly case?: string; readonly value?: unknown } | undefined,
): string {
  if (!answer.use) {
    return `Deixou passar ${ofThe([w.reactorLabel])}.`;
  }
  if (result?.case === 'shield') {
    const stopped = (result.value as { stopped: boolean }).stopped;
    return stopped
      ? 'Escudo Arcano usado: o ataque errou.'
      : 'Escudo Arcano usado: o ataque ainda acerta.';
  }
  return 'Reação usada.';
}
