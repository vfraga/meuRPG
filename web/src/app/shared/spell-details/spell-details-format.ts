import { spellLevelLabel } from '../../core/characters/character-labels';
import { tight } from '../../core/format/text';
import { formatMeters, feetToMeters } from '../../core/units';
import { SpellDetailsVm } from './spell-details.types';

/**
 * The "?" dialog's text (E6-22, E6-23): the SRD's structured values written
 * in Portuguese, in the table's units (5 ft = 1,5 m). Pure, so every unit,
 * range kind and duration form the API can return is covered by a test.
 *
 * What does not map (an unknown unit, a "Special" range, a duration that
 * is only text) comes back as the SRD's own `raw` string with
 * `fallback: true`: the dialog shows it as English text with a note,
 * rather than inventing a translation.
 */

export interface SpellFieldValue {
  /** The Portuguese text; empty for a fallback. */
  readonly text: string;
  /** English that belongs to the value (the material, a reaction's trigger, or the whole raw string for a fallback). */
  readonly english?: string;
  /** The value did not map: `english` is the SRD's raw text. */
  readonly fallback?: boolean;
  /** `english` is really Portuguese (the translated material component): no `lang="en"`. */
  readonly inPortuguese?: boolean;
}

export interface SpellFields {
  readonly castingTime: SpellFieldValue;
  readonly range: SpellFieldValue;
  readonly components: SpellFieldValue;
  readonly duration: SpellFieldValue;
}

function fallback(raw: string): SpellFieldValue {
  return { text: '', english: raw.trim() || '—', fallback: true };
}

/** "1 ação", "2 ações": the singular for 1, the plural otherwise. */
function counted(amount: number, one: string, many: string): string {
  return `${amount} ${amount === 1 ? one : many}`;
}

/** 1.5 -> "1,5"; 18 -> "18". */
function decimal(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1).replace('.', ',');
}

export function formatCastingTime(t: SpellDetailsVm['castingTime']): SpellFieldValue {
  if (!Number.isInteger(t.amount) || t.amount < 1) {
    return fallback(t.raw);
  }
  const trigger = t.trigger.trim() ? { english: t.trigger.trim() } : {};
  switch (t.unit) {
    case 'action':
      return { text: counted(t.amount, 'ação', 'ações') };
    case 'bonus_action':
      return { text: counted(t.amount, 'ação bônus', 'ações bônus') };
    case 'reaction':
      return { text: counted(t.amount, 'reação', 'reações'), ...trigger };
    case 'minute':
      return { text: counted(t.amount, 'minuto', 'minutos') };
    case 'hour':
      return { text: counted(t.amount, 'hora', 'horas') };
    default:
      return fallback(t.raw);
  }
}

/** Metres for a distance in feet (`core/units.ts`: 5 ft = 1,5 m), and
 * kilometres from 1.000 m. */
export function formatRangeFt(feet: number): string {
  const meters = feetToMeters(feet);
  return meters >= 1000 ? `${decimal(Math.round(meters / 100) / 10)} km` : formatMeters(meters);
}

export function formatRange(r: SpellDetailsVm['range']): SpellFieldValue {
  switch (r.kind) {
    case 'self':
      return { text: 'Pessoal' };
    case 'touch':
      return { text: 'Toque' };
    case 'sight':
      return { text: 'À vista' };
    case 'unlimited':
      return { text: 'Ilimitado' };
    case 'ranged':
      return r.distanceFt > 0 ? { text: formatRangeFt(r.distanceFt) } : fallback(r.raw);
    default:
      return fallback(r.raw);
  }
}

/**
 * "V, S, M" with the material after it: "(uma pitada de sal)" in Portuguese, or, when the reader
 * asked for English (or the spell has no translation yet), the SRD's "(a pinch of salt)".
 */
export function formatComponents(
  c: SpellDetailsVm['components'],
  english = false,
): SpellFieldValue {
  const letters = [c.verbal && 'V', c.somatic && 'S', c.material && 'M'].filter(Boolean);
  const why = c.materialWaivedBy ? ` · sem material (${c.materialWaivedBy})` : '';
  if (letters.length === 0) {
    return { text: `Nenhum${why}` };
  }
  const pt = english ? '' : (c.materialTextPt ?? '').trim();
  const material = c.material && (pt || c.materialText.trim());
  if (!material) {
    return { text: letters.join(', ') + why };
  }
  return pt
    ? { text: letters.join(', '), english: `(${material})`, inPortuguese: true }
    : { text: letters.join(', '), english: `(${material})` };
}

export function formatDuration(d: SpellDetailsVm['duration']): SpellFieldValue {
  switch (d.kind) {
    case 'instantaneous':
      return { text: 'Instantânea' };
    case 'until_dispelled':
      return { text: 'Até ser dissipada' };
    case 'timed': {
      if (!Number.isInteger(d.amount) || d.amount < 1) {
        return fallback(d.raw);
      }
      const length = durationLength(d.amount, d.unit);
      if (length === null) {
        return fallback(d.raw);
      }
      const prefix = d.concentration ? 'Concentração, ' : '';
      // "Concentração, até 1 minuto": a concentration spell always ends
      // early if the caster loses it, and the SRD writes "up to".
      const upTo = d.upTo || d.concentration ? 'até ' : '';
      return { text: `${prefix}${upTo}${length}` };
    }
    default:
      return fallback(d.raw);
  }
}

function durationLength(amount: number, unit: SpellDetailsVm['duration']['unit']): string | null {
  switch (unit) {
    case 'round':
      return counted(amount, 'rodada', 'rodadas');
    case 'minute':
      return counted(amount, 'minuto', 'minutos');
    case 'hour':
      return counted(amount, 'hora', 'horas');
    case 'day':
      return counted(amount, 'dia', 'dias');
    default:
      return null;
  }
}

export function spellFields(d: SpellDetailsVm, english = false): SpellFields {
  return {
    castingTime: formatCastingTime(d.castingTime),
    range: formatRange(d.range),
    components: formatComponents(d.components, english),
    duration: formatDuration(d.duration),
  };
}

export interface SpellRow {
  readonly label: string;
  readonly value: SpellFieldValue;
}

/**
 * The rows of a spell's facts, in the order the sheet prints them: casting time, range, "Alvo" (when the
 * server says whom it reaches), components, duration, and, for a spell of the table, "Ataque" and "Dano"
 * (an SRD spell's text says them). Only the structured values; nothing is worked out here.
 */
export function spellRows(d: SpellDetailsVm, english = false): SpellRow[] {
  const f = spellFields(d, english);
  const rows: SpellRow[] = [
    { label: 'Tempo de conjuração', value: f.castingTime },
    { label: 'Alcance', value: f.range },
  ];
  const target = d.targetLabel?.trim();
  if (target) {
    rows.push({ label: 'Alvo', value: { text: tight(target) } });
  }
  rows.push({ label: 'Componentes', value: f.components }, { label: 'Duração', value: f.duration });
  if (d.table) {
    if (d.attack === 'melee' || d.attack === 'ranged') {
      rows.push({
        label: 'Ataque',
        value: {
          text:
            d.attack === 'melee' ? 'Ataque de magia corpo a corpo' : 'Ataque de magia à distância',
        },
      });
    }
    if (d.damage && d.damage.length > 0) {
      rows.push({
        label: 'Dano',
        value: { text: d.damage.map((x) => `${x.dice} ${x.typePt}`.trim()).join(' e ') },
      });
    }
  }
  return rows;
}

/** "2º nível · Transmutação", or "Truque · Evocação". */
export function spellSubtitle(d: Pick<SpellDetailsVm, 'level' | 'schoolNamePt'>): string {
  return d.schoolNamePt
    ? `${spellLevelLabel(d.level)} · ${d.schoolNamePt}`
    : spellLevelLabel(d.level);
}
