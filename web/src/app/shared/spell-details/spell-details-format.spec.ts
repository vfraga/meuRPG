import { withoutMaterial } from './spell-details-map';
import { SpellDetailsVm } from './spell-details.types';
import {
  formatCastingTime,
  formatComponents,
  formatDuration,
  formatRange,
  formatRangeFt,
  spellFields,
  spellSubtitle,
} from './spell-details-format';

type Time = SpellDetailsVm['castingTime'];
type Range = SpellDetailsVm['range'];
type Duration = SpellDetailsVm['duration'];

const time = (amount: number, unit: Time['unit'], raw = '', trigger = ''): Time => ({
  amount,
  unit,
  raw,
  trigger,
});
const range = (kind: Range['kind'], distanceFt = 0, raw = ''): Range => ({ kind, distanceFt, raw });
const duration = (over: Partial<Duration>): Duration => ({
  kind: 'timed',
  amount: 0,
  unit: '',
  upTo: false,
  concentration: false,
  raw: '',
  ...over,
});

describe('formatCastingTime', () => {
  it('writes every unit, singular and plural', () => {
    expect(formatCastingTime(time(1, 'action')).text).toBe('1 ação');
    expect(formatCastingTime(time(1, 'bonus_action')).text).toBe('1 ação bônus');
    expect(formatCastingTime(time(1, 'reaction')).text).toBe('1 reação');
    expect(formatCastingTime(time(1, 'minute')).text).toBe('1 minuto');
    expect(formatCastingTime(time(10, 'minute')).text).toBe('10 minutos');
    expect(formatCastingTime(time(1, 'hour')).text).toBe('1 hora');
    expect(formatCastingTime(time(8, 'hour')).text).toBe('8 horas');
  });

  it("keeps a reaction's trigger as English text", () => {
    const value = formatCastingTime(
      time(1, 'reaction', '1 reaction', 'which you take when you are hit by an attack'),
    );
    expect(value.text).toBe('1 reação');
    expect(value.english).toBe('which you take when you are hit by an attack');
  });

  it('falls back to the SRD text for an unknown unit or a bad amount', () => {
    expect(formatCastingTime(time(1, '', '1 round'))).toEqual({
      text: '',
      english: '1 round',
      fallback: true,
    });
    expect(formatCastingTime(time(0, 'action', 'Special')).fallback).toBe(true);
  });
});

describe('formatRange', () => {
  it('names the fixed kinds', () => {
    expect(formatRange(range('self')).text).toBe('Pessoal');
    expect(formatRange(range('touch')).text).toBe('Toque');
    expect(formatRange(range('sight')).text).toBe('À vista');
    expect(formatRange(range('unlimited')).text).toBe('Ilimitado');
  });

  it('turns feet into metres at 5 ft = 1,5 m', () => {
    expect(formatRange(range('ranged', 5)).text).toBe('1,5\u00a0m');
    expect(formatRange(range('ranged', 60)).text).toBe('18\u00a0m');
    expect(formatRange(range('ranged', 120)).text).toBe('36\u00a0m');
    expect(formatRange(range('ranged', 25)).text).toBe('7,5\u00a0m');
  });

  it('uses kilometres from 1.000 m', () => {
    expect(formatRangeFt(5280)).toBe('1,6 km');
    expect(formatRangeFt(500 * 5280)).toBe('792 km');
  });

  it('falls back for "Special", an unknown kind, or a ranged spell without a distance', () => {
    expect(formatRange(range('special', 0, 'Special'))).toEqual({
      text: '',
      english: 'Special',
      fallback: true,
    });
    expect(formatRange(range('', 0, '')).fallback).toBe(true);
    expect(formatRange(range('ranged', 0, '30 feet')).english).toBe('30 feet');
  });
});

describe('a grant that waives the material', () => {
  it('takes the M and its text off the component line', () => {
    const c = { verbal: true, somatic: true, material: true, materialText: 'a pinch of salt' };
    expect(formatComponents(c)).toEqual({ text: 'V, S, M', english: '(a pinch of salt)' });
    expect(
      formatComponents(
        withoutMaterial({ components: c } as SpellDetailsVm, 'Graça de teste').components,
      ),
    ).toEqual({ text: 'V, S · sem material (Graça de teste)' });
  });
});

describe('formatComponents', () => {
  it('lists the letters, with the material in English', () => {
    expect(
      formatComponents({ verbal: true, somatic: false, material: false, materialText: '' }),
    ).toEqual({
      text: 'V',
    });
    expect(
      formatComponents({
        verbal: true,
        somatic: true,
        material: true,
        materialText: 'a pinch of sulfur',
      }),
    ).toEqual({ text: 'V, S, M', english: '(a pinch of sulfur)' });
  });

  it('says so when there are none', () => {
    expect(
      formatComponents({ verbal: false, somatic: false, material: false, materialText: '' }).text,
    ).toBe('Nenhum');
  });
});

describe('formatDuration', () => {
  it('writes the fixed kinds', () => {
    expect(formatDuration(duration({ kind: 'instantaneous' })).text).toBe('Instantânea');
    expect(formatDuration(duration({ kind: 'until_dispelled' })).text).toBe('Até ser dissipada');
  });

  it('writes every unit of a timed duration', () => {
    expect(formatDuration(duration({ amount: 1, unit: 'round' })).text).toBe('1 rodada');
    expect(formatDuration(duration({ amount: 10, unit: 'minute' })).text).toBe('10 minutos');
    expect(formatDuration(duration({ amount: 1, unit: 'hour' })).text).toBe('1 hora');
    expect(formatDuration(duration({ amount: 30, unit: 'day' })).text).toBe('30 dias');
  });

  it('writes "até" and the concentration', () => {
    expect(formatDuration(duration({ amount: 1, unit: 'minute', upTo: true })).text).toBe(
      'até 1 minuto',
    );
    expect(
      formatDuration(duration({ amount: 1, unit: 'minute', upTo: true, concentration: true })).text,
    ).toBe('Concentração, até 1 minuto');
    expect(
      formatDuration(duration({ amount: 8, unit: 'hour', upTo: true, concentration: true })).text,
    ).toBe('Concentração, até 8 horas');
  });

  it('falls back for "Special", an unknown unit or an empty amount', () => {
    expect(formatDuration(duration({ kind: 'special', raw: 'Special' })).fallback).toBe(true);
    expect(formatDuration(duration({ amount: 3, unit: '', raw: '3 fortnights' })).english).toBe(
      '3 fortnights',
    );
    expect(formatDuration(duration({ amount: 0, unit: 'hour', raw: 'x' })).fallback).toBe(true);
  });
});

describe('spellSubtitle and spellFields', () => {
  it('names the circle and the school', () => {
    expect(spellSubtitle({ level: 2, schoolNamePt: 'Transmutação' })).toBe(
      '2º nível · Transmutação',
    );
    expect(spellSubtitle({ level: 0, schoolNamePt: 'Evocação' })).toBe('Truque · Evocação');
    expect(spellSubtitle({ level: 1, schoolNamePt: '' })).toBe('1º nível');
  });

  it('formats the four fields of Knock', () => {
    const knock = {
      castingTime: time(1, 'action'),
      range: range('ranged', 60),
      components: { verbal: true, somatic: false, material: false, materialText: '' },
      duration: duration({ kind: 'instantaneous' }),
    } as SpellDetailsVm;
    expect(spellFields(knock)).toEqual({
      castingTime: { text: '1 ação' },
      range: { text: '18\u00a0m' },
      components: { text: 'V' },
      duration: { text: 'Instantânea' },
    });
  });
});
