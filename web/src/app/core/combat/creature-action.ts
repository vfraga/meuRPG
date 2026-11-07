import {
  Ability,
  type CreatureAction,
  CreatureActionClass,
  CreatureActionKind,
  type CreatureArea,
  CreatureAreaShape,
  type CreatureDamagePart,
  SpellAreaShape,
  SpellSaveSuccess,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  AreaPlacement,
  type CreatureActionResult,
  type LimitedAction,
  SaveOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { abilityName } from '../content/feat-text';
import { dcLabel } from '../format/save-dc';
import { joinDots, tight } from '../format/text';
import { SQUARE_FT, metersText } from '../units';
import { d20Formula } from './roll-mode';
import type { AreaShape } from './spell-area';

/**
 * An NPC's actions that are no attack roll (SRD 5.1, Monsters): a breath weapon or anything else that asks a saving throw,
 * and the actions with no roll at all. The stat block reaches the master only (RN-20). Pure functions, tested without a DOM.
 */

/** The actions the master can use with `UseCreatureAction`: a saving throw or no roll. Attacks and the Multiattack are made as before. */
export function usableActions(actions: readonly CreatureAction[] | undefined): CreatureAction[] {
  return (actions ?? []).filter(
    (a) =>
      a.key !== '' && (a.kind === CreatureActionKind.SAVE || a.kind === CreatureActionKind.OTHER),
  );
}

/** The action's name, in Portuguese when the content has it. */
export function actionName(a: CreatureAction): string {
  return a.namePt || a.name;
}

/** Whether the action asks a saving throw (otherwise it is a reminder that spends the action). */
export function asksSave(a: CreatureAction): boolean {
  return a.kind === CreatureActionKind.SAVE && !!a.save;
}

/** "2d6 de fogo + 1d4 de ácido": the damage parts, in the stat block's order. */
export function damagePartsText(parts: readonly CreatureDamagePart[]): string {
  return parts.map((p) => `${p.dice} de ${p.damageTypePt || 'dano'}`).join(' + ');
}

/** How the SRD words a duration ("1 minute", "10 minutes", "1 hour", "1 round"), in Portuguese; any other wording stays as the SRD has it. */
export function durationPt(text: string): string {
  const m = /^(\d+)\s+(round|minute|hour|day)s?$/i.exec(text.trim());
  if (!m) {
    return text.trim();
  }
  const n = Number(m[1]);
  const unit = { round: 'rodada', minute: 'minuto', hour: 'hora', day: 'dia' }[
    m[2].toLowerCase() as 'round' | 'minute' | 'hour' | 'day'
  ];
  return `${n} ${unit}${n === 1 ? '' : 's'}`;
}

/**
 * How the SRD words the limit of an action ("Recharge 5-6", "3/Day", "Recharges after a Short or Long Rest"), in Portuguese:
 * "Recarga 5–6" (an en dash between the numbers), "Recarga 6", "3/dia", "Recarrega após um descanso curto ou longo". Any other wording
 * stays as the SRD has it.
 */
export function usageText(text: string): string {
  const t = text.trim();
  const recharge = /^recharge\s+(\d)\s*[-–]\s*(\d)$/i.exec(t);
  if (recharge) {
    return `Recarga ${recharge[1]}–${recharge[2]}`;
  }
  const single = /^recharge\s+(\d)$/i.exec(t);
  if (single) {
    return `Recarga ${single[1]}`;
  }
  const perDay = /^(\d+)\s*\/\s*day$/i.exec(t);
  if (perDay) {
    return `${perDay[1]}/dia`;
  }
  const rest = /^recharges after an? (short|long)(?: or (short|long))? rest$/i.exec(t);
  if (rest) {
    const kinds = new Set([rest[1], rest[2]].filter(Boolean).map((k) => k.toLowerCase()));
    if (kinds.size === 2) {
      return 'Recarrega após um descanso curto ou longo';
    }
    return `Recarrega após um descanso ${kinds.has('short') ? 'curto' : 'longo'}`;
  }
  return t;
}

/** The shape of the area in words: "cone de 4,5 m", "linha de 18 m × 1,5 m", "raio de 3 m ao redor dele". */
export function creatureAreaText(area: CreatureArea | undefined): string {
  if (!area) {
    return '';
  }
  const size = metersText(area.sizeFt);
  switch (area.shape) {
    case CreatureAreaShape.CONE:
      return `cone de ${size}`;
    case CreatureAreaShape.LINE:
      return `linha de ${size} × ${metersText(Math.max(area.widthFt, SQUARE_FT))}`;
    case CreatureAreaShape.SPHERE:
      return `esfera de ${size} de raio`;
    case CreatureAreaShape.CUBE:
      return `cubo de ${size}`;
    case CreatureAreaShape.CYLINDER:
      return `cilindro de ${size} de raio`;
    case CreatureAreaShape.RADIUS:
      return `raio de ${size} ao redor dele`;
    default:
      return '';
  }
}

/** Where the master places the area, as `CastSpell` does: a point, a direction, or nothing (centered on the creature). `null`: no area. */
export function areaPlacementOf(a: CreatureAction): AreaPlacement | null {
  switch (a.area?.shape) {
    case CreatureAreaShape.CONE:
    case CreatureAreaShape.LINE:
    case CreatureAreaShape.CUBE:
      return AreaPlacement.DIRECTION;
    case CreatureAreaShape.SPHERE:
    case CreatureAreaShape.CYLINDER:
      return AreaPlacement.POINT;
    case CreatureAreaShape.RADIUS:
      return AreaPlacement.CASTER;
    default:
      return null;
  }
}

/** The area as the picker draws it (the same shape the spells have). */
export function areaShapeOf(a: CreatureAction): AreaShape {
  const area = a.area;
  const shape = {
    [CreatureAreaShape.CONE]: SpellAreaShape.CONE,
    [CreatureAreaShape.LINE]: SpellAreaShape.LINE,
    [CreatureAreaShape.CUBE]: SpellAreaShape.CUBE,
    [CreatureAreaShape.SPHERE]: SpellAreaShape.SPHERE,
    [CreatureAreaShape.CYLINDER]: SpellAreaShape.CYLINDER,
    [CreatureAreaShape.RADIUS]: SpellAreaShape.SPHERE,
  } as Partial<Record<CreatureAreaShape, SpellAreaShape>>;
  return {
    shape: shape[area?.shape ?? CreatureAreaShape.UNSPECIFIED] ?? SpellAreaShape.UNSPECIFIED,
    sizeFt: area?.sizeFt ?? 0,
    widthFt: area?.widthFt ?? 0,
  };
}

/**
 * What the action does, in Portuguese lines, from the plan the server reads: the saving throw and its DC, the damage and what a
 * success does, the area, the condition with its duration. An action with no roll says so. Never a number the plan does not hold.
 */
export function actionLines(a: CreatureAction): string[] {
  if (!asksSave(a)) {
    return ['Sem teste: gasta a ação e lembra o texto.'];
  }
  const s = a.save!;
  const lines: string[] = [];
  const ability = s.ability === Ability.UNSPECIFIED ? '' : abilityName(s.ability);
  const test = ability ? `Teste de resistência de ${ability}` : 'Teste de resistência';
  lines.push(
    joinDots([s.dc > 0 ? `${test}, CD ${s.dc}` : test, creatureAreaText(a.area)].filter(Boolean)),
  );
  if (s.damage.length > 0) {
    const half =
      s.onSuccess === SpellSaveSuccess.HALF
        ? ', metade se passar'
        : s.onSuccess === SpellSaveSuccess.NONE
          ? ', nada se passar'
          : '';
    lines.push(`Dano: ${damagePartsText(s.damage)}${half}`);
  }
  if (s.conditionPt || s.conditionKey) {
    const duration = s.duration ? ` por ${durationPt(s.duration)}` : '';
    const repeat = s.repeatSave ? '; repete o teste no fim de cada turno dele' : '';
    lines.push(`Se falhar: ${s.conditionPt || s.conditionKey}${duration}${repeat}`);
  }
  return lines;
}

/** The note under the lines: how far the app reads the text. `''` for an action the app reads whole. */
export function classNote(a: CreatureAction): string {
  switch (a.planClass) {
    case CreatureActionClass.PARTIAL:
      return 'O texto diz algo que o app não calcula: leia o lembrete.';
    case CreatureActionClass.TEXT:
      return 'O app não calcula esta ação: o texto é o lembrete.';
    default:
      return '';
  }
}

/** Whether the SRD's text is shown as a reminder (the master reads it): anything the plan does not hold whole. */
export function showsText(a: CreatureAction): boolean {
  return (
    a.kind === CreatureActionKind.OTHER ||
    a.planClass === CreatureActionClass.PARTIAL ||
    a.planClass === CreatureActionClass.TEXT
  );
}

/** One creature the action reached: who, how its saving throw went and what the action gave it. */
export interface TargetLine {
  readonly id: string;
  readonly label: string;
  /** "Falhou", "Passou: metade do dano", "Falha automática"; `''` when the target rolled nothing. */
  readonly word: string;
  readonly saved: boolean;
  /** "Teste de resistência: 1d20 (12) + 2 = 14 contra CD 13", or `''`. */
  readonly roll: string;
  /** "Ficou Amedrontado", or `''`. */
  readonly condition: string;
}

/**
 * What a used action did to each creature, from the server's answer: the d20 the target made against the DC (the master reads
 * both), whether it saved (and half the damage when the action says so), and the condition a failure gave.
 */
export function targetLines(
  result: CreatureActionResult,
  labels: ReadonlyMap<string, string>,
  half: boolean,
): TargetLine[] {
  return result.targets.map((t) => {
    const save = t.save;
    const saved = save?.outcome === SaveOutcome.SAVED;
    let word = '';
    if (save) {
      word = saved ? (half ? 'Passou: metade do dano' : 'Passou') : 'Falhou';
      if (save.autoFailed || save.autoFail) {
        word = 'Falha automática';
      }
    }
    const dc = save?.dc ? ` contra ${dcLabel(save.dc)}` : '';
    return {
      id: t.combatantId,
      label: labels.get(t.combatantId) ?? 'Alvo',
      word,
      saved,
      roll: save?.roll ? tight(`Teste de resistência: ${d20Formula(save.roll)}${dc}`) : '',
      condition:
        t.conditionApplied && result.conditionNamePt ? `Ficou ${result.conditionNamePt}` : '',
    };
  });
}

/** The notes of a used action: the duration and the repeated save the stat block words, for the master to keep. */
export function resultNotes(result: CreatureActionResult): string[] {
  const notes: string[] = [];
  if (result.conditionNamePt && result.durationText) {
    notes.push(`${result.conditionNamePt} por ${durationPt(result.durationText)}.`);
  }
  if (result.repeatSave) {
    notes.push('O alvo repete o teste no fim de cada turno dele; o app não conta o tempo.');
  }
  return notes;
}

/** What the card shows of a limited action's state (RN-29): a tag, or the dots of the uses left, and why "Usar" is off. */
export interface LimitView {
  /** "Pronta" or "Gasta" for a Recharge action and an action that waits for a rest; `''` for one with uses a day. */
  readonly tag: '' | 'Pronta' | 'Gasta';
  /** The uses a day: how many it has and how many are left (0 allowed: no dots). */
  readonly uses: { readonly allowed: number; readonly left: number } | null;
  /** "2 de 3", for the uses a day. */
  readonly usesText: string;
  /** Why the limit blocks "Usar" now, or `''`. */
  readonly why: string;
}

const NO_LIMIT: LimitView = { tag: '', uses: null, usesText: '', why: '' };

/** The faces a Recharge action comes back on, in words: "Recharge 5-6" is "5 ou 6", "Recharge 6" is "6". */
export function rechargeFaces(usage: string): string {
  const m = /(\d)\s*[-–]\s*(\d)/.exec(usage) ?? /(\d)/.exec(usage);
  if (!m) {
    return '';
  }
  return m[2] ? `${m[1]} ou ${m[2]}` : m[1];
}

/**
 * The state of a limited action from the server's copy for the master and the creature's controller (`LimitedAction`),
 * or no limit at all when the copy has none for the key. `creature` is the creature's label, for the reason.
 */
export function limitView(limited: LimitedAction | undefined, creature: string): LimitView {
  if (!limited) {
    return NO_LIMIT;
  }
  if (limited.kind === 'recharge') {
    const faces = rechargeFaces(limited.usage);
    return {
      tag: limited.available ? 'Pronta' : 'Gasta',
      uses: null,
      usesText: '',
      why: limited.available
        ? ''
        : `Volta com ${faces} no d6 que o app rola no começo de cada turno do ${creature}.`,
    };
  }
  if (limited.kind === 'per_day') {
    const left = limited.usesLeft;
    return {
      tag: '',
      uses: { allowed: limited.usesAllowed, left },
      usesText: `${left} de ${limited.usesAllowed}`,
      why: limited.available
        ? ''
        : limited.kept
          ? 'Sem usos: voltam no descanso longo da mesa.'
          : `Sem usos hoje. O ${creature} tem de novo no próximo combate.`,
    };
  }
  if (limited.kind === 'rest') {
    return {
      tag: limited.available ? 'Pronta' : 'Gasta',
      uses: null,
      usesText: '',
      why: limited.available
        ? ''
        : limited.kept
          ? 'Volta no próximo descanso curto ou longo.'
          : 'Volta no próximo combate.',
    };
  }
  return NO_LIMIT;
}

/** One line of the opened order row for a limited action: "Esporos · Recarga 6 · gasta", "Guincho atordoante · 1/dia · 1 de 1". */
export interface LimitLine {
  readonly key: string;
  readonly text: string;
  /** Material icon beside the words: a Recharge or rest action, or uses a day. */
  readonly icon: 'autorenew' | 'looks_one';
}

export function limitLines(limited: readonly LimitedAction[]): LimitLine[] {
  return limited.map((l) => {
    const state = limitView(l, '');
    const status = state.uses ? state.usesText : state.tag === 'Pronta' ? 'pronta' : 'gasta';
    return {
      key: l.actionKey,
      text: `${l.namePt || l.name} · ${usageText(l.usage)} · ${status}`,
      icon: l.kind === 'per_day' ? 'looks_one' : 'autorenew',
    };
  });
}
