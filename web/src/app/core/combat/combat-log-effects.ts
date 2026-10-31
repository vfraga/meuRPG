import {
  CombatEffect,
  type CombatLogEffect,
  CombatLogEffectChange,
  type CombatLogEntry,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { listNames } from './conditions';
import { article } from '../format/article';
import { dcLabel } from '../format/save-dc';
import { naturalMark } from './natural-mark';
import { luckyText } from './lucky';
import { LogContext, LogLine } from './combat-log-types';
import { ofSubject, subject } from './combat-log-words';
import { upFirst } from '../format/text';
import { abilityName } from '../effects/ability-choice';

/** Who an effect line is about, after the preposition ("em"/"a") or alone: "em Brisa e no Goblin 2", "Brisa e o Goblin 2". */
function effectTargets(fx: CombatLogEffect, ctx: LogContext, prep: '' | 'em' | 'a' = ''): string {
  const names = fx.targetLabels.map((label) => {
    if (prep === '') {
      return subject(label, ctx);
    }
    if (ctx.players.has(label)) {
      return `${prep} ${label}`;
    }
    const feminine = article(label) === 'a';
    const joined = prep === 'em' ? (feminine ? 'na' : 'no') : feminine ? 'à' : 'ao';
    return `${joined} ${label}`;
  });
  if (names.length === 0) {
    return prep === '' ? 'alguém' : `${prep} alguém`;
  }
  return listNames(names);
}

type EffectLine = (fx: CombatLogEffect, ctx: LogContext) => string;

/** "(d20 12, total 15 contra CD 14)": the numbers of a saving throw against an effect, for the master. The rolling
 * player reads "(CD 14)" when the table shows the DC and the server sent it. */
function effectSaveNumbers(fx: CombatLogEffect, ctx: LogContext): string {
  if (!ctx.master) {
    const dc = dcLabel(fx.dc);
    return dc ? ` (${dc})` : '';
  }
  if (fx.total === undefined || fx.dc === undefined) {
    return '';
  }
  // "20 natural" is information only: a saving throw has no automatic success or failure (SRD 5.1).
  const natural = fx.d20 !== undefined ? naturalMark(fx.d20) : '';
  return ` (${fx.d20 !== undefined ? `d20 ${fx.d20}, ` : ''}${natural ? `${natural}, ` : ''}total ${fx.total} contra CD ${fx.dc})`;
}

function effectSaveWords(fx: CombatLogEffect): string {
  const name = abilityName(fx.ability, fx.abilityNamePt);
  return name ? ` de ${name}` : '';
}

/** "entrou no Raio Lunar" or "começou o turno na Teia": how a creature met a zone (`reason` zone_enter, zone_start, zone_cast, zone_end), or `''`. */
function zoneMeeting(fx: CombatLogEffect): string {
  const inSpell = `${article(fx.sourceNamePt) === 'a' ? 'na' : 'no'} ${fx.sourceNamePt}`;
  switch (fx.reason) {
    case 'zone_enter':
      return `entrou ${inSpell}`;
    case 'zone_start':
      return `começou o turno ${inSpell}`;
    case 'zone_cast':
      return `estava ${inSpell} quando a magia foi conjurada`;
    case 'zone_end':
      return `terminou o turno ${inSpell}`;
    default:
      return '';
  }
}

/** "Tor começou o turno na Teia e passou no teste de Destreza (…)", or `null` when the line is no zone catch. */
function zoneCatch(fx: CombatLogEffect, ctx: LogContext, then: string): string | null {
  const met = zoneMeeting(fx);
  return met ? `${upFirst(effectTargets(fx, ctx))} ${met} e ${then}` : null;
}

/** Why a zone (or an effect) ended, as the end of the line: "a concentração acabou". */
const ENDED_WHY: Readonly<Record<string, string>> = {
  concentration: 'a concentração acabou',
  duration: 'o tempo da magia passou',
  ended_by_master: 'o mestre a encerrou',
};

function endedLine(fx: CombatLogEffect, ctx: LogContext): string {
  if (fx.reason === 'concentration_replaced') {
    // A zone's caster is its only target (the server writes the caster as the label).
    const caster = fx.targetLabels[0] ?? 'alguém';
    return `${fx.sourceNamePt} ${ofSubject(caster, ctx)} acabou: ${caster} começou outra magia de concentração`;
  }
  const base = `${fx.sourceNamePt} acabou ${effectTargets(fx, ctx, 'em')}`;
  const why = ENDED_WHY[fx.reason];
  return why ? `${base}: ${why}` : base;
}

const EFFECT_LINES: Partial<Record<CombatLogEffectChange, EffectLine>> = {
  [CombatLogEffectChange.ADDED]: (fx, ctx) =>
    `${fx.sourceNamePt} foi aplicado ${effectTargets(fx, ctx, 'em')}`,
  [CombatLogEffectChange.ENDED]: endedLine,
  [CombatLogEffectChange.ZONE_MOVED]: (fx, ctx) =>
    `${fx.sourceNamePt} ${ofSubject(fx.targetLabels[0] ?? 'alguém', ctx)} mudou de lugar`,
  [CombatLogEffectChange.DURATION_CHANGED]: (fx, ctx) =>
    `A duração de ${fx.sourceNamePt} ${effectTargets(fx, ctx, 'em')} mudou`,
  [CombatLogEffectChange.SAVED]: (fx, ctx) =>
    zoneCatch(fx, ctx, `passou no teste${effectSaveWords(fx)}${effectSaveNumbers(fx, ctx)}`) ??
    `${upFirst(effectTargets(fx, ctx))} passou no teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}${effectSaveNumbers(fx, ctx)}`,
  [CombatLogEffectChange.SAVE_FAILED]: (fx, ctx) =>
    zoneCatch(fx, ctx, `falhou no teste${effectSaveWords(fx)}${effectSaveNumbers(fx, ctx)}`) ??
    `${upFirst(effectTargets(fx, ctx))} falhou no teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}${effectSaveNumbers(fx, ctx)}`,
  [CombatLogEffectChange.SAVE_SKIPPED]: (fx, ctx) =>
    `${upFirst(effectTargets(fx, ctx))} não fez o teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}`,
  [CombatLogEffectChange.DAMAGE]: (fx, ctx) =>
    zoneCatch(
      fx,
      ctx,
      `sofreu ${fx.amount} de dano${fx.damageTypePt ? ` ${fx.damageTypePt}` : ''}`,
    ) ??
    `${fx.sourceNamePt} causou ${fx.amount} de dano${fx.damageTypePt ? ` ${fx.damageTypePt}` : ''} ${effectTargets(fx, ctx, 'a')}`,
  [CombatLogEffectChange.TEMP_HP]: (fx, ctx) =>
    `${fx.sourceNamePt} deu ${fx.amount} PV temporários ${effectTargets(fx, ctx, 'a')}`,
};

/** The line of a lasting effect, with the d20 the halfling's Lucky rolled again on its saving throw as a note
 * ("Sortudo: 1 → 14", sent only to whoever may read the roll). */
export function effectLine(
  base: Pick<LogLine, 'id' | 'actor' | 'hidden' | 'undoable'>,
  fx: CombatLogEffect,
  ctx: LogContext,
): LogLine {
  const lucky = luckyText(fx);
  return {
    ...base,
    icon: 'auto_awesome',
    actor: '',
    text: effectLogText(fx, ctx),
    ...(lucky ? { notes: [lucky] } : {}),
  };
}

/** The line of a lasting effect: "Heroísmo deu 5 PV temporários a Brisa", "Brisa passou no teste de Destreza contra Teia". */
function effectLogText(fx: CombatLogEffect, ctx: LogContext): string {
  const named = fx.sourceNamePt ? fx : { ...fx, sourceNamePt: 'Um efeito' };
  const line = EFFECT_LINES[fx.change];
  return line ? line(named, ctx) : `${named.sourceNamePt}: ${effectTargets(fx, ctx)}`;
}

/** "Brisa agora tem exaustão de nível 2". */
export function exhaustionLogText(fx: CombatLogEffect, ctx: LogContext): string {
  const who = upFirst(effectTargets(fx, ctx));
  if (fx.reason === 'frenzy') {
    // The end of a frenzied rage (SRD 5.1, Berserker): "Frenesi: +1 nível de exaustão · Ragna agora tem exaustão de nível 1".
    return `Frenesi: +1 nível de exaustão · ${who} agora tem exaustão de nível ${fx.exhaustionLevel}`;
  }
  return fx.exhaustionLevel > 0
    ? `${who} agora tem exaustão de nível ${fx.exhaustionLevel}`
    : `${who} ficou sem exaustão`;
}

/** "O Escudo Arcano de Pensantus acabou", "A Ajuda de Sálvia acabou" and, for the master, who has the numbers
 * (the server sends them to nobody else: zero), ": PV 43 → 38". */
export function effectEndedText(e: CombatLogEntry, ctx: LogContext): string {
  const end = e.effectEnd;
  const label = e.actorLabel || e.targetLabel || 'alguém';
  const of = ofSubject(label, ctx);
  if (end?.effect === CombatEffect.SHIELD) {
    return `O Escudo Arcano ${of} acabou`;
  }
  if (end?.effect !== CombatEffect.AID) {
    return `Um efeito ${of} acabou`;
  }
  const base = `A Ajuda ${of} acabou`;
  if (!ctx.master || end.hitPointsMaxBefore <= 0) {
    return base;
  }
  return end.hitPointsBefore === end.hitPointsAfter
    ? `${base}: PV ${end.hitPointsAfter}, máximo ${end.hitPointsMaxBefore} → ${end.hitPointsMaxAfter}`
    : `${base}: PV ${end.hitPointsBefore} → ${end.hitPointsAfter}`;
}
