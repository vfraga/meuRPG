import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  type MapPoint,
  TrapState,
  type TrapSpecSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  Ability,
  type TrapEffect,
  type TrapPreset,
  TrapPassOutcome,
  TrapSaveApplies,
  TrapTargets,
  TrapTrigger,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { DAMAGE_TYPE_OPTIONS } from '../../../core/characters/character-labels';
import { metersText } from '../../../core/units';
import type { PointChanges } from '../../../core/maps/maps-client';
import { POINT_DESCRIPTION_MAX, POINT_NAME_MAX } from '../point-panel/point-draft';

/**
 * What the trap form edits (E9-02, MR-035): the form's own text and choices, kept apart from the saved point
 * until "Salvar ponto". The effect is a list of parts the master adds and removes; each field is the text he
 * typed, so a half-typed number is not lost. The server's rules (maps.proto) are checked first so each field
 * can say what is wrong, and the server stays the authority. Pure functions: tested without a DOM.
 */
export interface DamageDraft {
  readonly dice: string;
  readonly typeKey: string;
}

export interface ConditionDraft {
  readonly key: string;
  /** "1 hora": how long it lasts, for the master's table (0 to 60 characters). */
  readonly duration: string;
}

export interface AttackDraft {
  readonly bonus: string;
  readonly count: string;
  readonly damage: DamageDraft;
}

export interface SaveDraft {
  readonly ability: Ability;
  readonly dc: string;
  readonly appliesTo: TrapSaveApplies;
  readonly failDamage: readonly DamageDraft[];
  readonly failCondition: ConditionDraft | null;
  readonly onPass: TrapPassOutcome;
}

export interface TrapDraft {
  readonly presetKey: string;
  readonly name: string;
  readonly description: string;
  /** Empty: nobody notices it alone (the SRD's poison needle). */
  readonly noticeDc: string;
  readonly findDc: string;
  readonly areaSize: 1 | 2 | 3 | 4;
  readonly trigger: TrapTrigger;
  readonly state: TrapState;
  readonly targets: TrapTargets;
  readonly attack: AttackDraft | null;
  /** "Dano que sempre acontece": damage that lands on every creature caught. */
  readonly damage: readonly DamageDraft[];
  /** "Condição que sempre acontece". */
  readonly conditions: readonly ConditionDraft[];
  readonly save: SaveDraft | null;
}

export const MAX_DAMAGE_PARTS = 4;
export const MAX_CONDITION_PARTS = 4;

/** The damage types the SRD names, by the server's key (`damage-type:poison`), with the words the rest of the app uses (`character-labels`). */
export const DAMAGE_TYPES: readonly { readonly key: string; readonly name: string }[] =
  DAMAGE_TYPE_OPTIONS.map((o) => ({
    key: `damage-type:${o.key}`,
    name: o.label,
  }));

export const ABILITIES: readonly { readonly value: Ability; readonly name: string }[] = [
  { value: Ability.STRENGTH, name: 'Força' },
  { value: Ability.DEXTERITY, name: 'Destreza' },
  { value: Ability.CONSTITUTION, name: 'Constituição' },
  { value: Ability.INTELLIGENCE, name: 'Inteligência' },
  { value: Ability.WISDOM, name: 'Sabedoria' },
  { value: Ability.CHARISMA, name: 'Carisma' },
];

const blankDamage = (): DamageDraft => ({ dice: '', typeKey: 'damage-type:bludgeoning' });

/** The parts the "+" actions add. */
export function newDamage(): DamageDraft {
  return blankDamage();
}

export function newCondition(): ConditionDraft {
  return { key: 'condition:prone', duration: '' };
}

export function newAttack(): AttackDraft {
  return { bonus: '', count: '1', damage: { dice: '', typeKey: 'damage-type:piercing' } };
}

export function newSave(): SaveDraft {
  return {
    ability: Ability.DEXTERITY,
    dc: '',
    appliesTo: TrapSaveApplies.CAUGHT,
    failDamage: [],
    failCondition: null,
    onPass: TrapPassOutcome.NONE,
  };
}

/** A trap from nothing ("Começar do zero"): the square alone, fired by entering it, with no effect yet. */
export function blankTrapDraft(name = ''): TrapDraft {
  return {
    presetKey: '',
    name,
    description: '',
    noticeDc: '',
    findDc: '',
    areaSize: 1,
    trigger: TrapTrigger.ENTER,
    state: TrapState.ARMED,
    targets: TrapTargets.AREA,
    attack: null,
    damage: [],
    conditions: [],
    save: null,
  };
}

function damageDraft(d: { dice: string; damageTypeKey: string }): DamageDraft {
  return { dice: d.dice, typeKey: d.damageTypeKey };
}

function effectDraft(
  effect: TrapEffect | undefined,
): Pick<TrapDraft, 'targets' | 'attack' | 'damage' | 'conditions' | 'save'> {
  return {
    targets: effect?.targets === TrapTargets.MANUAL ? TrapTargets.MANUAL : TrapTargets.AREA,
    attack: effect?.attack
      ? {
          bonus: String(effect.attack.bonus),
          count: String(effect.attack.count),
          damage: effect.attack.damage ? damageDraft(effect.attack.damage) : blankDamage(),
        }
      : null,
    damage: (effect?.damage ?? []).map(damageDraft),
    conditions: (effect?.conditions ?? []).map((c) => ({
      key: c.conditionKey,
      duration: c.durationPt,
    })),
    save: effect?.save
      ? {
          ability: effect.save.ability,
          dc: String(effect.save.dc),
          appliesTo:
            effect.save.appliesTo === TrapSaveApplies.HIT
              ? TrapSaveApplies.HIT
              : TrapSaveApplies.CAUGHT,
          failDamage: (effect.save.onFail?.damage ?? []).map(damageDraft),
          failCondition: effect.save.onFail?.condition
            ? {
                key: effect.save.onFail.condition.conditionKey,
                duration: effect.save.onFail.condition.durationPt,
              }
            : null,
          onPass:
            effect.save.onPass === TrapPassOutcome.HALF
              ? TrapPassOutcome.HALF
              : TrapPassOutcome.NONE,
        }
      : null,
  };
}

/** The form filled from a preset of the SRD: it only fills, and everything stays editable. */
export function trapDraftFromPreset(preset: TrapPreset): TrapDraft {
  return {
    presetKey: preset.key,
    name: preset.namePt,
    description: preset.descriptionPt,
    noticeDc: preset.noticeDc > 0 ? String(preset.noticeDc) : '',
    findDc: String(preset.findDc),
    areaSize: clampArea(preset.areaSize),
    trigger: preset.trigger === TrapTrigger.MANUAL ? TrapTrigger.MANUAL : TrapTrigger.ENTER,
    state: TrapState.ARMED,
    ...effectDraft(preset.effect),
  };
}

function clampArea(n: number): 1 | 2 | 3 | 4 {
  return n >= 4 ? 4 : n === 3 ? 3 : n === 2 ? 2 : 1;
}

/** The form as the saved point has it. */
export function trapDraftOf(point: Pick<MapPoint, 'name' | 'description' | 'trap'>): TrapDraft {
  const t = point.trap;
  return {
    presetKey: t?.presetKey ?? '',
    name: point.name,
    description: point.description,
    noticeDc: t && t.noticeDc > 0 ? String(t.noticeDc) : '',
    findDc: t ? String(t.findDc) : '',
    areaSize: clampArea(t?.areaSize ?? 1),
    trigger: t?.trigger === TrapTrigger.MANUAL ? TrapTrigger.MANUAL : TrapTrigger.ENTER,
    state:
      t?.state === TrapState.TRIGGERED || t?.state === TrapState.DISARMED
        ? t.state
        : TrapState.ARMED,
    ...effectDraft(t?.effect),
  };
}

export interface TrapErrors {
  readonly name?: string;
  readonly description?: string;
  readonly noticeDc?: string;
  readonly findDc?: string;
  readonly attackBonus?: string;
  readonly attackCount?: string;
  readonly attackDice?: string;
  /** By part: `damage:0`, `fail:1`, `condition:0`, `saveDc`, `save` (the save as a whole). */
  readonly parts: Readonly<Record<string, string>>;
}

const DICE = /^(\d{1,2})d(4|6|8|10|12)$/;
const DICE_MESSAGE = 'Use dados como 2d6 (de d4 a d12) ou um número de 1 a 100.';

/** Whether `text` is a die the server takes: 1 to 20 dice of d4 to d12, or a flat 1 to 100. */
export function validDice(text: string): boolean {
  const t = text.trim();
  const m = DICE.exec(t);
  if (m) {
    const n = Number(m[1]);
    return n >= 1 && n <= 20;
  }
  return /^\d{1,3}$/.test(t) && Number(t) >= 1 && Number(t) <= 100;
}

function wholeIn(text: string, min: number, max: number): boolean {
  return /^\d{1,3}$/.test(text.trim()) && Number(text) >= min && Number(text) <= max;
}

/** The server's rules (maps.proto: the numbers are checked against the rules content), so a field can say what is wrong. */
export function trapErrors(d: TrapDraft): TrapErrors {
  const errors: { -readonly [K in keyof TrapErrors]: TrapErrors[K] } = { parts: {} };
  const parts: Record<string, string> = {};
  const name = d.name.trim();
  if (name === '') {
    errors.name = 'Dê um nome ao ponto.';
  } else if ([...name].length > POINT_NAME_MAX) {
    errors.name = `Use até ${POINT_NAME_MAX} caracteres.`;
  } else if (/\p{Cc}/u.test(name)) {
    errors.name = 'Use um nome numa linha só.';
  }
  if ([...d.description].length > POINT_DESCRIPTION_MAX) {
    errors.description = 'Use até 2.000 caracteres.';
  }
  if (d.noticeDc.trim() !== '' && !wholeIn(d.noticeDc, 1, 30)) {
    errors.noticeDc = 'Use uma CD de 1 a 30, ou deixe vazio.';
  }
  if (!wholeIn(d.findDc, 1, 30)) {
    errors.findDc = 'Use uma CD de 1 a 30.';
  }
  if (d.attack) {
    if (!wholeIn(d.attack.bonus, 0, 20)) {
      errors.attackBonus = 'Use de 0 a 20.';
    }
    if (!wholeIn(d.attack.count, 1, 10)) {
      errors.attackCount = 'Use de 1 a 10.';
    }
    if (!validDice(d.attack.damage.dice)) {
      errors.attackDice = DICE_MESSAGE;
    }
  }
  d.damage.forEach((x, i) => {
    if (!validDice(x.dice)) {
      parts[`damage:${i}`] = DICE_MESSAGE;
    }
  });
  d.conditions.forEach((c, i) => {
    if ([...c.duration].length > 60) {
      parts[`condition:${i}`] = 'Use até 60 caracteres.';
    }
  });
  if (d.save) {
    if (!wholeIn(d.save.dc, 1, 30)) {
      parts['saveDc'] = 'Use uma CD de 1 a 30.';
    }
    d.save.failDamage.forEach((x, i) => {
      if (!validDice(x.dice)) {
        parts[`fail:${i}`] = DICE_MESSAGE;
      }
    });
    if (d.save.failCondition && [...d.save.failCondition.duration].length > 60) {
      parts['failCondition'] = 'Use até 60 caracteres.';
    }
    if (d.save.failDamage.length === 0 && !d.save.failCondition) {
      parts['save'] = 'Escolha o que acontece a quem falha: dano ou uma condição.';
    } else if (d.save.onPass === TrapPassOutcome.HALF && d.save.failDamage.length === 0) {
      parts['save'] = 'Metade do dano precisa de dano para quem falha.';
    }
    if (d.save.appliesTo === TrapSaveApplies.HIT && !d.attack) {
      parts['save'] =
        'Só quem foi atingido faz a resistência: acrescente um ataque ou escolha quem foi pego.';
    }
  }
  return { ...errors, parts };
}

export function hasTrapErrors(e: TrapErrors): boolean {
  return Object.keys(e).some((k) => k !== 'parts') || Object.keys(e.parts).length > 0;
}

function damageSpec(d: DamageDraft) {
  return { dice: d.dice.trim(), damageTypeKey: d.typeKey };
}

function conditionSpec(c: ConditionDraft) {
  return { conditionKey: c.key, durationPt: c.duration.trim() };
}

/** What `CreateMapPoint` and `UpdateMapPoint` take as the trap (the whole spec). */
export function trapSpecOf(d: TrapDraft): MessageInitShape<typeof TrapSpecSchema> {
  return {
    presetKey: d.presetKey,
    noticeDc: d.noticeDc.trim() === '' ? 0 : Number(d.noticeDc),
    findDc: Number(d.findDc),
    areaSize: d.areaSize,
    trigger: d.trigger,
    state: d.state,
    effect: {
      targets: d.targets,
      ...(d.attack
        ? {
            attack: {
              bonus: Number(d.attack.bonus),
              count: Number(d.attack.count),
              damage: damageSpec(d.attack.damage),
            },
          }
        : {}),
      damage: d.damage.map(damageSpec),
      conditions: d.conditions.map(conditionSpec),
      ...(d.save
        ? {
            save: {
              ability: d.save.ability,
              dc: Number(d.save.dc),
              appliesTo: d.save.appliesTo,
              onPass: d.save.onPass,
              onFail: {
                damage: d.save.failDamage.map(damageSpec),
                ...(d.save.failCondition ? { condition: conditionSpec(d.save.failCondition) } : {}),
              },
            },
          }
        : {}),
    },
  };
}

/**
 * Whether the form differs from what the point has (the page asks before leaving it). `openedState` is the state the form opened with: a
 * trap that fired while the form was open is not an edit of the master's.
 */
export function isTrapDirty(
  d: TrapDraft,
  point: Pick<MapPoint, 'name' | 'description' | 'trap'>,
  openedState?: TrapState,
): boolean {
  const saved = trapDraftOf(point);
  return (
    JSON.stringify(d) !==
    JSON.stringify(openedState === undefined ? saved : { ...saved, state: openedState })
  );
}

/**
 * What "Salvar ponto" sends; `null` when nothing changed. The spec goes whole, as `UpdateMapPoint` replaces it, except the state: the
 * server keeps the current one unless the master chose another than the form opened with (the live game changes it too).
 */
export function trapChangesOf(
  d: TrapDraft,
  point: Pick<MapPoint, 'name' | 'description' | 'trap'>,
  openedState?: TrapState,
): PointChanges | null {
  if (!isTrapDirty(d, point, openedState)) {
    return null;
  }
  const spec = trapSpecOf(d);
  const changes: { -readonly [K in keyof PointChanges]: PointChanges[K] } = {
    trap:
      openedState !== undefined && d.state === openedState
        ? { ...spec, state: TrapState.UNSPECIFIED }
        : spec,
  };
  if (d.name.trim() !== point.name) {
    changes.name = d.name.trim();
  }
  if (d.description !== point.description) {
    changes.description = d.description;
  }
  return changes;
}

const ABILITY_NAME = new Map(ABILITIES.map((a) => [a.value, a.name]));

/** One line about a preset for its radio: "Queda de 6 m, 2d6", "1 perfurante, 2d10 veneno · resistência de Constituição". The server's
 * words and numbers, only put in a line. */
export function presetSummary(p: TrapPreset): string {
  const e = p.effect;
  const dmg = (d: { dice: string; damageTypePt: string }) => `${d.dice} ${d.damageTypePt}`.trim();
  if (p.fallFt > 0 && e && e.damage.length > 0) {
    return `Queda de ${metersText(p.fallFt)}, ${e.damage[0].dice}`;
  }
  const parts: string[] = [];
  if (e?.attack) {
    parts.push(`ataque +${e.attack.bonus}${e.attack.damage ? `, ${dmg(e.attack.damage)}` : ''}`);
  }
  if (e && e.damage.length > 0) {
    parts.push(e.damage.map(dmg).join(', '));
  }
  if (e?.save) {
    parts.push(`resistência de ${ABILITY_NAME.get(e.save.ability) ?? 'habilidade'}`);
  }
  const text = parts.join(' · ');
  return text === '' ? 'Só descrição' : text.charAt(0).toUpperCase() + text.slice(1);
}
