import { feetToMeters, formatMeters } from '../units';
import type {
  DurationKindKey,
  SpellDetailsVm,
} from '../../shared/spell-details/spell-details.types';
import {
  formatCastingTime,
  formatComponents,
  formatDuration,
  formatRange,
} from '../../shared/spell-details/spell-details-format';
import { rangeFeet } from './effect-draft';
import { SHAPE_OPTIONS } from './spell-options';
import { SUMMON_FIELD_PATHS } from './spell-summon';
import type { SpellDraft } from './spell-types';

/** One row of "Como os jogadores veem". */
export interface PreviewRow {
  readonly label: string;
  readonly value: string;
}

export type NameOf = (key: string) => string;

/** The damage type as the server names it ("fogo"); the key's last word when the catalog does not know it. */
const keyName: NameOf = (key) => key.replace(/^[a-z-]+:/, '');

/** The target as the preview says it: "Uma criatura", "3 criaturas", "Cone de 4,5 m", "Só quem conjura". */
export function targetText(d: SpellDraft): string {
  switch (d.target) {
    case 'creature':
      return d.perSlot > 0 ? `Uma criatura, mais ${d.perSlot} por nível de espaço` : 'Uma criatura';
    case 'creatures':
      return d.perSlot > 0
        ? `${d.count} criaturas, mais ${d.perSlot} por nível de espaço`
        : `${d.count} criaturas`;
    case 'self':
      return 'Só quem conjura';
    case 'area': {
      const ft = rangeFeet(d.sizeM);
      const shape = SHAPE_OPTIONS.find((s) => s.value === d.shape)?.label ?? '';
      return ft > 0 ? `${shape} de ${formatMeters(feetToMeters(ft))}` : shape;
    }
  }
}

/** "+1d8" typed with or without its plus, said once. */
function withoutPlus(text: string): string {
  return text.trim().replace(/^\+/, '');
}

/** What the extra damage per level (or per tier of a truque) says after the dice: ", +1d8 por nível acima do 1º". */
function moreDamageText(d: SpellDraft): string {
  if (!d.more.trim()) {
    return '';
  }
  return d.level === 0
    ? `, +${withoutPlus(d.more)} por degrau do truque`
    : `, +${withoutPlus(d.more)} por nível acima do ${d.level}º`;
}

/** "2d8 necrótico, +1d8 por nível acima do 1º". */
export function damageText(d: SpellDraft, nameOf: NameOf = keyName): string {
  if (!d.dice.trim()) {
    return '';
  }
  const type = nameOf(d.damageType);
  return `${d.dice.trim()}${type ? ` ${type}` : ''}${moreDamageText(d)}`;
}

function durationKind(d: SpellDraft): DurationKindKey {
  switch (d.duration) {
    case 'instant':
      return 'instantaneous';
    case 'timed':
      return 'timed';
    default:
      return 'until_dispelled';
  }
}

/** The rows of the spell's cast: time, range, target, components and duration. */
function castRows(d: SpellDraft): PreviewRow[] {
  const time = formatCastingTime({
    amount: d.time === 'minute' || d.time === 'hour' ? d.timeAmount : 1,
    unit: d.time,
    trigger: '',
    raw: '',
  });
  const range = formatRange({
    kind: d.range,
    distanceFt: d.range === 'ranged' ? rangeFeet(d.rangeM) : 0,
    raw: '',
  } satisfies SpellDetailsVm['range']);
  const comps = formatComponents({
    verbal: d.verbal,
    somatic: d.somatic,
    material: d.material,
    materialText: '',
  });
  const duration = formatDuration({
    kind: durationKind(d),
    amount: d.durationAmount,
    unit: d.durationUnit,
    upTo: d.upTo,
    concentration: d.concentration,
    raw: '',
  });
  return [
    { label: 'Tempo', value: time.text || '—' },
    { label: 'Alcance', value: range.text || '—' },
    { label: 'Alvo', value: targetText(d) },
    {
      label: 'Componentes',
      value:
        d.material && d.materialPt.trim() ? `${comps.text} (${d.materialPt.trim()})` : comps.text,
    },
    { label: 'Duração', value: duration.text || '—' },
  ];
}

/** The attack or the saving throw, then the damage they roll. */
function attackRows(d: SpellDraft, nameOf: NameOf): PreviewRow[] {
  const rows: PreviewRow[] = [];
  if (d.mechanic === 'attack') {
    rows.push({
      label: 'Ataque',
      value: d.attack === 'melee' ? 'Ataque de magia corpo a corpo' : 'Ataque de magia à distância',
    });
  } else {
    const ab = nameOf(`ability:${d.saveAbility}`);
    rows.push({
      label: 'Teste de resistência',
      value: `${ab}, ${d.saveOnSuccess === 'half' ? 'metade do dano ao passar' : 'nada ao passar'}`,
    });
  }
  const dmg = damageText(d, nameOf);
  if (dmg) {
    rows.push({ label: 'Dano', value: dmg });
  }
  return rows;
}

function healRows(d: SpellDraft): PreviewRow[] {
  if (!d.healDice.trim()) {
    return [];
  }
  const more = d.healMore.trim() ? `, +${withoutPlus(d.healMore)} por nível de espaço` : '';
  return [
    {
      label: 'Cura',
      value: `${d.healDice.trim()}${d.healModifier ? ' + modificador' : ''}${more}`,
    },
  ];
}

/** What a player reads of the spell, written as the app writes an SRD spell (the shared formatters of the "?"). */
export function previewRows(d: SpellDraft, nameOf: NameOf = keyName): PreviewRow[] {
  const rows = castRows(d);
  if (d.mechanic === 'attack' || d.mechanic === 'save') {
    rows.push(...attackRows(d, nameOf));
  }
  if (d.mechanic === 'heal') {
    rows.push(...healRows(d));
  }
  return rows;
}

/** The inputs every spell form draws, whatever it is. */
const ALWAYS_FIELDS = [
  'name_pt',
  'level',
  'school_key',
  'casting_time.unit',
  'range.kind',
  'duration.kind',
  'components.verbal',
  'components.somatic',
  'components.material',
  'concentration',
  'ritual',
  'class_keys',
  'desc_pt',
  'higher_level_pt',
  'target.kind',
];

/** The fields the casting (time, range, duration, components) draws beyond the always-there ones. */
function castFieldPaths(d: SpellDraft): string[] {
  const paths: string[] = [];
  if (d.time === 'minute' || d.time === 'hour') paths.push('casting_time.amount');
  if (d.time === 'reaction') paths.push('casting_time.trigger_pt');
  if (d.range === 'ranged') paths.push('range.distance_ft');
  if (d.duration === 'timed') paths.push('duration.amount', 'duration.unit', 'duration.up_to');
  if (d.material) paths.push('components.material_pt');
  return paths;
}

function targetFieldPaths(d: SpellDraft): string[] {
  switch (d.target) {
    case 'creatures':
      return ['target.count', 'target.per_slot_level'];
    case 'creature':
      return ['target.per_slot_level'];
    case 'area':
      return ['target.shape', 'target.size_ft'];
    default:
      return [];
  }
}

function mechanicFieldPaths(d: SpellDraft): string[] {
  const paths: string[] = [];
  if (d.mechanic === 'attack') paths.push('attack');
  if (d.mechanic === 'save') paths.push('save.ability', 'save.on_success');
  if (d.mechanic === 'attack' || d.mechanic === 'save') {
    paths.push(
      'damage[0].dice',
      'damage[0].damage_type_key',
      d.level === 0 ? 'damage[0].per_tier' : 'damage[0].per_slot_level',
    );
  }
  if (d.mechanic === 'heal') paths.push('heal.dice', 'heal.per_slot_level', 'heal.adds_modifier');
  if (d.mechanic === 'summon') paths.push(...SUMMON_FIELD_PATHS);
  return paths;
}

/** The paths of the inputs the spell form draws right now: where a refusal can land. Anything else goes to its nearest input
 * above, or to the top. The mechanic, the target and the range show their own fields only. */
export function spellFieldPaths(d: SpellDraft): Set<string> {
  const paths = [
    ...ALWAYS_FIELDS,
    ...castFieldPaths(d),
    ...targetFieldPaths(d),
    ...mechanicFieldPaths(d),
  ];
  return new Set(paths.map((x) => `table_spell.${x}`));
}
