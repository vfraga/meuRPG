import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  CombatLogEffectChange,
  CombatLogEffectSchema,
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logLine } from './combat-log';

const effect = (fields: MessageInitShape<typeof CombatLogEffectSchema>): CombatLogEntry =>
  create(CombatLogEntrySchema, {
    id: 'x',
    kind: CombatLogKind.EFFECT,
    effect: create(CombatLogEffectSchema, {
      sourceKey: 'spell:moonbeam',
      sourceNamePt: 'Raio Lunar',
      targetLabels: ['Bartolo'],
      ...fields,
    }),
  });

const text = (e: CombatLogEntry, master = false) =>
  logLine(e, '', { master, players: new Set(['Bartolo', 'Tor']) })?.text.replace(/ /g, ' ');

describe('the log lines of spell zones', () => {
  it('says a zone moved, naming its caster', () => {
    expect(text(effect({ change: CombatLogEffectChange.ZONE_MOVED }))).toBe(
      'Raio Lunar de Bartolo mudou de lugar',
    );
    expect(
      text(effect({ change: CombatLogEffectChange.ZONE_MOVED, targetLabels: ['Cultista 1'] })),
    ).toBe('Raio Lunar da Cultista 1 mudou de lugar');
  });

  it('says a creature began its turn in a zone, and how the save went', () => {
    const fail = effect({
      change: CombatLogEffectChange.SAVE_FAILED,
      reason: 'zone_start',
      targetLabels: ['Tor'],
      abilityNamePt: 'Constituição',
    });
    expect(text(fail)).toBe('Tor começou o turno no Raio Lunar e falhou no teste de Constituição');
    const pass = effect({
      change: CombatLogEffectChange.SAVED,
      reason: 'zone_start',
      sourceNamePt: 'Teia',
      targetLabels: ['Tor'],
      abilityNamePt: 'Destreza',
    });
    expect(text(pass)).toBe('Tor começou o turno na Teia e passou no teste de Destreza');
  });

  it('says a creature was in the area at the cast, or ended its turn in it', () => {
    const cast = effect({
      change: CombatLogEffectChange.SAVE_FAILED,
      reason: 'zone_cast',
      sourceNamePt: 'Área Escorregadia',
      targetLabels: ['Tor'],
      abilityNamePt: 'Destreza',
    });
    expect(text(cast)).toBe(
      'Tor estava na Área Escorregadia quando a magia foi conjurada e falhou no teste de Destreza',
    );
    const end = effect({
      change: CombatLogEffectChange.SAVED,
      reason: 'zone_end',
      sourceNamePt: 'Área Escorregadia',
      targetLabels: ['Tor'],
      abilityNamePt: 'Destreza',
    });
    expect(text(end)).toBe(
      'Tor terminou o turno na Área Escorregadia e passou no teste de Destreza',
    );
  });

  it('says a creature came into a zone, with the numbers only for the master', () => {
    const e = effect({
      change: CombatLogEffectChange.SAVE_FAILED,
      reason: 'zone_enter',
      targetLabels: ['Goblin 2'],
      abilityNamePt: 'Constituição',
      d20: 9,
      total: 12,
      dc: 14,
    });
    // The rolling player reads the DC only when the server sent it (the table rule decides).
    expect(text(e)).toBe(
      'O Goblin 2 entrou no Raio Lunar e falhou no teste de Constituição (CD 14)',
    );
    const bare = effect({
      change: CombatLogEffectChange.SAVE_FAILED,
      reason: 'zone_enter',
      targetLabels: ['Goblin 2'],
      abilityNamePt: 'Constituição',
    });
    expect(text(bare)).toBe('O Goblin 2 entrou no Raio Lunar e falhou no teste de Constituição');
    expect(text(e, true)).toBe(
      'O Goblin 2 entrou no Raio Lunar e falhou no teste de Constituição (d20 9, total 12 contra CD 14)',
    );
  });

  it('says the damage of a zone that asks no save, with the way the creature met it', () => {
    const e = effect({
      change: CombatLogEffectChange.DAMAGE,
      reason: 'zone_enter',
      sourceNamePt: 'Nuvem de Adagas',
      targetLabels: ['Tor'],
      amount: 11,
      damageTypePt: 'cortante',
    });
    expect(text(e)).toBe('Tor entrou na Nuvem de Adagas e sofreu 11 de dano cortante');
  });

  it.each([
    ['concentration', 'Raio Lunar acabou em Bartolo: a concentração acabou'],
    ['duration', 'Raio Lunar acabou em Bartolo: o tempo da magia passou'],
    ['ended_by_master', 'Raio Lunar acabou em Bartolo: o mestre a encerrou'],
  ])('says why a zone ended: %s', (reason, line) => {
    expect(text(effect({ change: CombatLogEffectChange.ENDED, reason }))).toBe(line);
  });

  it('says a zone ended because its caster began another concentration spell', () => {
    expect(
      text(effect({ change: CombatLogEffectChange.ENDED, reason: 'concentration_replaced' })),
    ).toBe('Raio Lunar de Bartolo acabou: Bartolo começou outra magia de concentração');
  });

  it('keeps the old lines for an effect that is no zone', () => {
    expect(text(effect({ change: CombatLogEffectChange.ENDED }))).toBe(
      'Raio Lunar acabou em Bartolo',
    );
    expect(text(effect({ change: CombatLogEffectChange.SAVED, abilityNamePt: 'Destreza' }))).toBe(
      'Bartolo passou no teste de Destreza contra Raio Lunar',
    );
  });
});
