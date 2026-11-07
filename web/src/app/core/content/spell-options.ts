import type { SpellDraft } from './spell-types';
import type {
  DurationChoice,
  MechanicChoice,
  RangeChoice,
  TargetChoice,
  TimeChoice,
} from './spell-types';

/** The choices of the spell form, as the selects list them (Portuguese labels). */

export const TIME_OPTIONS: readonly { value: TimeChoice; label: string }[] = [
  { value: 'action', label: 'Ação' },
  { value: 'bonus_action', label: 'Ação bônus' },
  { value: 'reaction', label: 'Reação' },
  { value: 'minute', label: 'Minutos' },
  { value: 'hour', label: 'Horas' },
];

/** The range offers "Pessoal" and "Toque" beside a distance (E10-01 state 4b). */
export const RANGE_OPTIONS: readonly { value: RangeChoice; label: string }[] = [
  { value: 'ranged', label: 'Distância' },
  { value: 'self', label: 'Pessoal' },
  { value: 'touch', label: 'Toque' },
  { value: 'sight', label: 'À vista' },
  { value: 'unlimited', label: 'Ilimitado' },
];

export const DURATION_OPTIONS: readonly { value: DurationChoice; label: string }[] = [
  { value: 'instant', label: 'Instantânea' },
  { value: 'timed', label: 'Por um tempo' },
  { value: 'until_dispelled', label: 'Até ser dissipada' },
];

export const DURATION_UNITS: readonly { value: SpellDraft['durationUnit']; label: string }[] = [
  { value: 'round', label: 'Rodadas' },
  { value: 'minute', label: 'Minutos' },
  { value: 'hour', label: 'Horas' },
  { value: 'day', label: 'Dias' },
];

export const TARGET_OPTIONS: readonly { value: TargetChoice; label: string }[] = [
  { value: 'creature', label: 'Uma criatura' },
  { value: 'creatures', label: 'Várias criaturas' },
  { value: 'area', label: 'Área' },
  { value: 'self', label: 'Só quem conjura' },
];

export const MECHANIC_OPTIONS: readonly { value: MechanicChoice; label: string }[] = [
  { value: 'text', label: 'Só texto' },
  { value: 'attack', label: 'Ataque' },
  { value: 'save', label: 'Teste de resistência' },
  { value: 'heal', label: 'Cura' },
  { value: 'summon', label: 'Invocar' },
];

export const SHAPE_OPTIONS: readonly { value: SpellDraft['shape']; label: string }[] = [
  { value: 'cone', label: 'Cone' },
  { value: 'cube', label: 'Cubo' },
  { value: 'cylinder', label: 'Cilindro' },
  { value: 'line', label: 'Linha' },
  { value: 'sphere', label: 'Esfera' },
];

/** The size's label follows the shape: the length of a cone and a line, the side of a cube, the radius of a sphere and a cylinder. */
export function sizeLabel(shape: SpellDraft['shape']): string {
  switch (shape) {
    case 'cube':
      return 'Lado';
    case 'sphere':
    case 'cylinder':
      return 'Raio';
    default:
      return 'Comprimento';
  }
}

export const SAVE_SUCCESS_OPTIONS: readonly {
  value: SpellDraft['saveOnSuccess'];
  label: string;
}[] = [
  { value: 'half', label: 'Metade do dano' },
  { value: 'none', label: 'Nada' },
];

export const ATTACK_OPTIONS: readonly { value: SpellDraft['attack']; label: string }[] = [
  { value: 'ranged', label: 'Ataque de magia à distância' },
  { value: 'melee', label: 'Ataque de magia corpo a corpo' },
];

export function circleLabel(level: number): string {
  return level === 0 ? 'Truque' : `${level}º nível`;
}

/** "Mais dano por nível acima do 1º", or at the truque's tiers. */
export function moreLabel(level: number): string {
  return level === 0
    ? 'Mais dano a cada degrau do truque'
    : `Mais dano por nível acima do ${level}º`;
}

export function moreHint(level: number): string {
  return level === 0 ? 'Nos níveis 5, 11 e 17 do personagem.' : 'Ao conjurar com um espaço maior.';
}
