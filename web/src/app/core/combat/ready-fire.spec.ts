import { create } from '@bufbuild/protobuf';

import {
  CombatantSchema,
  CombatantState,
  EncounterSchema,
  EncounterStatus,
  HiddenRevealQuestionSchema,
  ReactionKind,
  ReactionWindowSchema,
  ReactionWindowStatus,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { fireRows } from './ready-fire';

const readied = (over: Record<string, unknown> = {}) =>
  create(ReadiedActionSchema, {
    id: 'r-tor',
    triggerText: 'Quando o zumbi passar pela porta',
    kind: ReadiedKind.ATTACK,
    actionKey: 'equipment:longsword',
    actionNamePt: 'Espada longa',
    targetCombatantId: 'z1',
    status: ReadiedStatus.WAITING,
    ...over,
  } as never);

const who = (id: string, label: string, over: Record<string, unknown> = {}) =>
  create(CombatantSchema, { id, label, hitPointsCurrent: 10, ...over } as never);

const tor = (over: Record<string, unknown> = {}) =>
  who('tor', 'Tor', { readied: readied(), ...over });

const combat = (members: ReturnType<typeof who>[], over: Record<string, unknown> = {}) =>
  create(EncounterSchema, {
    id: 'e1',
    status: EncounterStatus.ACTIVE,
    currentCombatantId: 'z1',
    combatants: [who('z1', 'Zumbi 1'), ...members],
    ...over,
  } as never);

const only = (e: ReturnType<typeof combat>, surprised: string[] = []) => {
  const rows = fireRows(e, surprised);
  expect(rows).toHaveLength(1);
  return rows[0];
};

describe('the master’s readied actions (#582, board 582-04)', () => {
  it('lists who readied what, the trigger quoted, in the order of the fight', () => {
    const row = only(combat([tor()]));
    expect(row.id).toBe('r-tor');
    expect(row.who).toBe('Tor');
    expect(row.held).toBe('Espada longa no Zumbi 1');
    expect(row.quote).toBe('“Quando o zumbi passar pela porta”');
    expect(row.why).toBe('');
  });

  it('lists nothing for a row that no longer waits or for nobody who readied', () => {
    expect(
      fireRows(combat([tor({ readied: readied({ status: ReadiedStatus.FIRED }) })]), []),
    ).toEqual([]);
    expect(fireRows(combat([who('b', 'Brisa')]), [])).toEqual([]);
  });

  it('marks a hidden creature that readied: only the master sees it', () => {
    expect(only(combat([tor({ hidden: true })])).hidden).toBe(true);
    expect(only(combat([tor()])).hidden).toBe(false);
  });

  it('refuses on the readier’s own turn (READY_OWN_TURN)', () => {
    const row = only(combat([tor()], { currentCombatantId: 'tor', turnGroupIds: ['tor'] }));
    expect(row.why).toBe('É a vez de Tor: só dispara fora da vez dele.');
  });

  it('refuses in a shared turn until the readier ends their part, then lets it fire', () => {
    const group = { currentCombatantId: 'z1', turnGroupIds: ['tor', 'z1'] };
    expect(only(combat([tor()], group)).why).toBe(
      'Tor ainda joga a parte dele da vez, junto com o Zumbi 1. Dispare depois que ele encerrar a parte dele.',
    );
    expect(only(combat([tor({ turnPartEnded: true })], group)).why).toBe('');
  });

  it('says the reaction is spent', () => {
    expect(only(combat([tor({ reactionUsed: true })])).why).toBe(
      'Tor já usou a reação nesta rodada.',
    );
  });

  it('says the readier is down, with 0 hit points', () => {
    expect(only(combat([tor({ hitPointsCurrent: 0, state: CombatantState.DOWN })])).why).toBe(
      'Tor está Caído (0 PV): não reage.',
    );
  });

  it('says the readier is incapacitated', () => {
    expect(only(combat([tor({ conditions: ['condition:stunned'] })])).why).toBe(
      'Tor está Incapacitado: não reage.',
    );
  });

  it('says the readier is surprised until the first turn ends', () => {
    expect(only(combat([tor()]), ['tor']).why).toBe(
      'Tor foi surpreendido: só reage depois da primeira vez dele.',
    );
  });

  it('says a concentration save is owed, or a hidden reveal', () => {
    const save = create(ReactionWindowSchema, {
      id: 'w1',
      kind: ReactionKind.CONCENTRATION_SAVE,
      status: ReactionWindowStatus.OPEN,
      reactorId: 'tor',
    } as never);
    expect(only(combat([tor()], { reactionWindows: [save] })).why).toBe(
      'Falta o teste de concentração de Tor.',
    );
    const ask = create(HiddenRevealQuestionSchema, {} as never);
    expect(only(combat([tor()], { pendingHiddenReveals: [ask] })).why).toBe(
      'Responda antes a pergunta sobre a criatura escondida.',
    );
  });

  it('says its window is already open', () => {
    const open = create(ReactionWindowSchema, {
      id: 'w2',
      kind: ReactionKind.READIED,
      status: ReactionWindowStatus.OPEN,
      reactorId: 'tor',
    } as never);
    expect(only(combat([tor()], { reactionWindows: [open] })).why).toBe(
      'A janela de Tor já está aberta: espere a resposta.',
    );
  });

  it('gives the first reason the server checks when there are several', () => {
    const row = only(combat([tor({ reactionUsed: true, conditions: ['condition:stunned'] })]));
    expect(row.why).toBe('Tor está Incapacitado: não reage.');
  });
});
