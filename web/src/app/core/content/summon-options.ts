import {
  CastingTimeUnit,
  SpellDurationKind,
  SpellDurationUnit,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { SelectOption } from '../../shared/form-fields/select-field';
import { DURATION_OPTIONS } from './spell-options';
import type { SummonDuration, SummonTime } from './summon-draft';

/**
 * The words of the summon block's selects (E10-01): a closed list of the times a feature's summon takes and lasts,
 * each one a value `unit:amount` (`kind:amount:unit` for the duration). A stored value the lists do not have (written by
 * the API) is shown as one more option, so the select never claims it is something else.
 */

/** The spell editor's own word for it. */
const UNTIL_DISPELLED =
  DURATION_OPTIONS.find((o) => o.value === 'until_dispelled')?.label ?? 'Até ser dissipada';
const LONG_REST_HOURS = 8;
const DAY_HOURS = 24;

const TIMES: readonly { time: SummonTime; label: string }[] = [
  { time: time(CastingTimeUnit.ACTION, 1), label: '1 ação' },
  { time: time(CastingTimeUnit.BONUS_ACTION, 1), label: '1 ação bônus' },
  { time: time(CastingTimeUnit.MINUTE, 1), label: '1 minuto' },
  { time: time(CastingTimeUnit.MINUTE, 10), label: '10 minutos' },
  { time: time(CastingTimeUnit.HOUR, 1), label: '1 hora' },
  { time: time(CastingTimeUnit.HOUR, LONG_REST_HOURS), label: '8 horas' },
];

const DURATIONS: readonly { duration: SummonDuration; label: string }[] = [
  { duration: instant(), label: 'Instantânea' },
  { duration: timed(1, SpellDurationUnit.ROUND), label: '1 rodada' },
  { duration: timed(1, SpellDurationUnit.MINUTE), label: '1 minuto' },
  { duration: timed(10, SpellDurationUnit.MINUTE), label: '10 minutos' },
  { duration: timed(1, SpellDurationUnit.HOUR), label: '1 hora' },
  { duration: timed(LONG_REST_HOURS, SpellDurationUnit.HOUR), label: '8 horas' },
  { duration: timed(DAY_HOURS, SpellDurationUnit.HOUR), label: '24 horas' },
  {
    duration: { kind: SpellDurationKind.UNTIL_DISPELLED, amount: 0, unit: 0, upTo: false },
    label: UNTIL_DISPELLED,
  },
];

const TIME_UNITS: Readonly<Record<number, readonly [string, string]>> = {
  [CastingTimeUnit.ACTION]: ['ação', 'ações'],
  [CastingTimeUnit.BONUS_ACTION]: ['ação bônus', 'ações bônus'],
  [CastingTimeUnit.REACTION]: ['reação', 'reações'],
  [CastingTimeUnit.MINUTE]: ['minuto', 'minutos'],
  [CastingTimeUnit.HOUR]: ['hora', 'horas'],
};

const DURATION_UNITS: Readonly<Record<number, readonly [string, string]>> = {
  [SpellDurationUnit.ROUND]: ['rodada', 'rodadas'],
  [SpellDurationUnit.MINUTE]: ['minuto', 'minutos'],
  [SpellDurationUnit.HOUR]: ['hora', 'horas'],
  [SpellDurationUnit.DAY]: ['dia', 'dias'],
};

function time(unit: CastingTimeUnit, amount: number): SummonTime {
  return { unit, amount, triggerPt: '' };
}

function instant(): SummonDuration {
  return { kind: SpellDurationKind.INSTANTANEOUS, amount: 0, unit: 0, upTo: false };
}

function timed(amount: number, unit: SpellDurationUnit): SummonDuration {
  return { kind: SpellDurationKind.TIMED, amount, unit, upTo: false };
}

/** An empty time is 1 action, an empty duration is instantaneous (the server's own reading); any other stored value keeps its own. */
export function timeValue(t: SummonTime): string {
  if (t.unit === CastingTimeUnit.UNSPECIFIED && t.amount === 0) {
    return `${CastingTimeUnit.ACTION}:1`;
  }
  return `${t.unit}:${oneShot(t.unit) && t.amount === 0 ? 1 : t.amount}`;
}

/** An action, a bonus action and a reaction are one: a stored amount of 0 reads as 1 (as an unset one does). */
function oneShot(unit: CastingTimeUnit): boolean {
  return (
    unit === CastingTimeUnit.ACTION ||
    unit === CastingTimeUnit.BONUS_ACTION ||
    unit === CastingTimeUnit.REACTION
  );
}

export function durationValue(d: SummonDuration): string {
  const plain = d.amount === 0 && d.unit === 0 && !d.upTo;
  if (
    plain &&
    (d.kind === SpellDurationKind.UNSPECIFIED || d.kind === SpellDurationKind.INSTANTANEOUS)
  ) {
    return String(SpellDurationKind.INSTANTANEOUS);
  }
  if (plain && d.kind !== SpellDurationKind.TIMED) {
    return String(d.kind);
  }
  return `${d.kind}:${d.amount}:${d.unit}${d.upTo ? ':up' : ''}`;
}

function timeLabel(t: SummonTime): string {
  const [one, many] = TIME_UNITS[t.unit] ?? ['tempo', 'tempos'];
  const amount = oneShot(t.unit) && t.amount === 0 ? 1 : t.amount;
  return `${amount} ${amount === 1 ? one : many}`;
}

function durationLabel(d: SummonDuration): string {
  if (d.kind === SpellDurationKind.TIMED) {
    const [one, many] = DURATION_UNITS[d.unit] ?? ['unidade', 'unidades'];
    return `${d.upTo ? 'Até ' : ''}${d.amount} ${d.amount === 1 ? one : many}`;
  }
  return d.kind === SpellDurationKind.UNTIL_DISPELLED ? UNTIL_DISPELLED : 'Instantânea';
}

/** The times to pick from, with the stored one added when it is none of them. */
export function timeOptions(current: SummonTime): readonly SelectOption[] {
  const rows = TIMES.map((t) => ({ value: timeValue(t.time), label: t.label }));
  return rows.some((r) => r.value === timeValue(current))
    ? rows
    : [...rows, { value: timeValue(current), label: timeLabel(current) }];
}

export function durationOptions(current: SummonDuration): readonly SelectOption[] {
  const rows = DURATIONS.map((d) => ({ value: durationValue(d.duration), label: d.label }));
  return rows.some((r) => r.value === durationValue(current))
    ? rows
    : [...rows, { value: durationValue(current), label: durationLabel(current) }];
}

/** The time a select value names; a stored time with a trigger keeps it while the unit stays the same. */
export function timeOf(value: string, current: SummonTime): SummonTime {
  const [unit, amount] = value.split(':').map(Number);
  const same = unit === current.unit;
  return { unit, amount, triggerPt: same ? current.triggerPt : '' };
}

export function durationOf(value: string, current: SummonDuration): SummonDuration {
  const preset = DURATIONS.find((d) => durationValue(d.duration) === value);
  if (preset) {
    return preset.duration;
  }
  if (durationValue(current) === value) {
    return current;
  }
  const [kind, amount, unit] = value.split(':').map(Number);
  return { kind, amount: amount ?? 0, unit: unit ?? 0, upTo: false };
}
