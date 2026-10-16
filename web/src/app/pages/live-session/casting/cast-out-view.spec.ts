import { CastingEffectKind } from '../../../../gen/meurpg/play/v1/casting_pb';
import { castingSpell } from '../../../core/casting/casting-testing';
import {
  castRequestOf,
  diceOf,
  endsConfirmText,
  missingOf,
  summonedNotice,
  targetChoicesOf,
  wayRowsOf,
} from './cast-out-view';

const heal = castingSpell('spell:cure-wounds', 'Curar Ferimentos', 1, {
  effect: CastingEffectKind.HEAL,
  maxTargets: 1,
});
/** The notices glue their words with non-breaking spaces. */
const plain = (s: string) => s.replace(/\u00a0/g, ' ');
const slot = { level: 1, pact: false } as never;

describe('cast-out rows the sheet spec does not read', () => {
  it('words the slot by its casting time and the ritual by its extra time', () => {
    const alarm = castingSpell('spell:alarm', 'Alarme', 1, {
      ritualAllowed: true,
      ritualMinutes: 11,
      castingMinutes: 1,
      castingTimePt: '1 minuto',
    });
    const [way, ritual] = wayRowsOf(alarm, ['slot', 'ritual']);
    expect(way).toMatchObject({
      id: 'slot',
      title: 'Com espaço de magia',
      detail: 'Tempo: 1 minuto',
    });
    expect(ritual).toMatchObject({ id: 'ritual', title: 'Como ritual' });
    expect(ritual.detail).toContain('· sem espaço de magia');
    const instant = castingSpell('spell:bless', 'Bênção', 1, {
      castingMinutes: 0,
      castingTimePt: '1 ação',
    });
    expect(wayRowsOf(instant, ['slot'])[0].detail).toBe('1 ação');
  });

  it('marks the caster among the targets', () => {
    const rows = [
      { id: 'a', name: 'Ilaria', self: true, detail: 'd', enabled: true, reason: '' },
      { id: 'b', name: 'Toren', self: false, detail: 'e', enabled: false, reason: 'longe' },
    ];
    expect(targetChoicesOf(rows).map((r) => [r.title, r.enabled, r.reason])).toEqual([
      ['Ilaria (você)', true, ''],
      ['Toren', false, 'longe'],
    ]);
  });
});

describe('diceOf', () => {
  it('reads "2d6" and falls back to one d4 for anything else', () => {
    expect(diceOf('2d6')).toEqual({ count: 2, sides: 6 });
    expect(diceOf('')).toEqual({ count: 1, sides: 4 });
    expect(diceOf('1d8+2')).toEqual({ count: 1, sides: 4 });
  });
});

describe('missingOf', () => {
  const picks = {
    way: 'slot',
    slot: null,
    showTargets: true,
    chosen: [] as string[],
    spellKey: 'spell:cure-wounds',
    ability: '',
  } as const;

  it('has nothing to ask without a spell', () => {
    expect(missingOf(null, picks)).toBe('');
  });

  it('asks for the slot, then the target, then the ability', () => {
    expect(missingOf(heal, picks)).toBe('Escolha o espaço de magia.');
    expect(missingOf(heal, { ...picks, slot })).toBe('Escolha em quem a magia age.');
    expect(missingOf(heal, { ...picks, slot, chosen: ['toren'] })).toBe('');
    expect(
      missingOf(heal, { ...picks, slot, chosen: ['toren'], spellKey: 'spell:enhance-ability' }),
    ).toBe('Escolha a habilidade.');
  });

  it('asks for no slot as a ritual or for a cantrip, and for no target when none are shown', () => {
    expect(missingOf(heal, { ...picks, way: 'ritual', showTargets: false })).toBe('');
    const cantrip = castingSpell('spell:flame', 'Chama', 0, { effect: CastingEffectKind.HEAL });
    expect(missingOf(cantrip, { ...picks, chosen: ['x'] })).toBe('');
  });
});

describe('castRequestOf', () => {
  const parts = {
    campaignId: 'c',
    casterId: 'ilaria',
    spellKey: 'spell:cure-wounds',
    way: 'slot',
    spell: heal,
    slot: { level: 2, pact: true } as never,
    chosen: ['toren'],
    dice: { inApp: true },
    ability: null,
  } as const;

  it('sends the slot, the targets and the dice, and no ability key when none is asked', () => {
    const req = castRequestOf(parts);
    expect(req).toMatchObject({
      asRitual: false,
      slot: { level: 2, pact: true },
      targetIds: ['toren'],
      dice: { inApp: true },
    });
    expect('abilityKey' in req).toBe(false);
  });

  it('sends no slot for a ritual or a cantrip, and the ability when the spell asks', () => {
    expect(castRequestOf({ ...parts, way: 'ritual' })).toMatchObject({
      asRitual: true,
      slot: null,
    });
    const cantrip = castingSpell('spell:flame', 'Chama', 0);
    expect(castRequestOf({ ...parts, spell: cantrip }).slot).toBeNull();
    expect(castRequestOf({ ...parts, ability: 'cha' })).toMatchObject({ abilityKey: 'cha' });
    expect(castRequestOf({ ...parts, ability: '' })).toMatchObject({ abilityKey: '' });
  });
});

describe('summonedNotice and endsConfirmText', () => {
  const base = {
    spellName: 'Convocar Familiar',
    ritual: false,
    castingTime: '1 hora',
    dismissed: 0,
  };

  it('names one creature, says "a criatura" for an unnamed one, or counts several', () => {
    expect(plain(summonedNotice({ ...base, names: ['Nanquim'], count: 1 }))).toContain(
      'Nanquim chegou.',
    );
    expect(plain(summonedNotice({ ...base, names: [], count: 1 }))).toContain('A criatura chegou.');
    expect(plain(summonedNotice({ ...base, names: ['A', 'B'], count: 2 }))).toContain(
      '2 criaturas chegaram.',
    );
  });

  it('adds what the cast cost: the slot, or the ritual with its time', () => {
    expect(plain(summonedNotice({ ...base, names: ['N'], count: 1 }))).toContain(
      'o espaço de magia foi gasto',
    );
    expect(plain(summonedNotice({ ...base, names: ['N'], count: 1, ritual: true }))).toContain(
      'ritual de 1 hora',
    );
  });

  it('asks before ending a concentration, naming it', () => {
    expect(endsConfirmText('Bênção', 'Escudo da Fé')).toEqual({
      title: 'Isso encerra Escudo da Fé',
      body: 'Conjurar Bênção encerra a concentração em Escudo da Fé, e os efeitos dela terminam.',
      confirm: 'Encerrar Escudo da Fé e conjurar',
      cancel: 'Cancelar',
    });
  });
});
