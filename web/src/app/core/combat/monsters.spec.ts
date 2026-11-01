import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ADD_MAX,
  AddKeys,
  COMBAT_MAX,
  addMonstersErrorMessage,
  becomesText,
  groupLabel,
  monsterLabels,
  monsterSentence,
  roomFor,
} from './monsters';

const add = {
  creatureKey: 'monster:bandit',
  count: 3,
  name: 'Bandido',
  hp: 'average' as const,
  hidden: true,
  target: 'enc-1',
};

function blocked(reason: EncounterBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
  ]);
}

describe('"Pôr no combate": the names, the room and the key (MR-042, RN-29)', () => {
  it('numbers three monsters "Bandido 1" to "Bandido 3", and one keeps the plain name', () => {
    expect(monsterLabels('Bandido', 3)).toEqual(['Bandido 1', 'Bandido 2', 'Bandido 3']);
    expect(monsterLabels(' Bandido ', 1)).toEqual(['Bandido']);
    expect(monsterSentence('Bandido', 3)).toBe('Bandido 1, Bandido 2 e Bandido 3');
    expect(monsterSentence('Ogro', 2)).toBe('Ogro 1 e Ogro 2');
    expect(monsterSentence('Ogro', 1)).toBe('Ogro');
  });

  it('says what a group of an encounter becomes: "Ogro", "Bugbear 1 e 2", "Goblin 1 a 6"', () => {
    expect(becomesText('Ogro', 1)).toBe('Ogro');
    expect(becomesText('Bugbear', 2)).toBe('Bugbear 1 e 2');
    expect(becomesText('Goblin', 6)).toBe('Goblin 1 a 6');
  });

  it('names the monsters of one creature for the end-of-combat XP', () => {
    expect(groupLabel(['Bandido 1', 'Bandido 2', 'Bandido 3'])).toBe('Bandido 1 a 3');
    expect(groupLabel(['Bandido 1', 'Bandido 2'])).toBe('Bandido 1 e 2');
    expect(groupLabel(['Ogro'])).toBe('Ogro');
    expect(groupLabel(['Salteador', 'Bandido 2'])).toBe('Salteador, Bandido 2');
  });

  it("leaves room for what a combat of 40 still takes, never past one add's 10", () => {
    expect(roomFor(0)).toBe(ADD_MAX);
    expect(roomFor(COMBAT_MAX - 4)).toBe(4);
    expect(roomFor(COMBAT_MAX)).toBe(0);
    expect(roomFor(COMBAT_MAX + 3)).toBe(0);
  });

  it('keeps one key for the same parameters (a retry), and takes a new one when the master changed something', () => {
    const keys = new AddKeys();
    const first = keys.keyFor(add);
    expect(keys.keyFor(add)).toBe(first);
    expect(keys.keyFor({ ...add })).toBe(first);
    const rolled = keys.keyFor({ ...add, hp: 'rolled' });
    expect(rolled).not.toBe(first);
    // Back to the first choice is still another add from the server's side: it was not the one the first key made.
    expect(keys.keyFor({ ...add, hp: 'rolled' })).toBe(rolled);
    expect(keys.keyFor({ ...add, count: 2, hp: 'rolled' })).not.toBe(rolled);
  });
});

describe('the words of a refused "Pôr no combate" (by code and typed detail, never by the message)', () => {
  it('says the combat is full, ended or that there is no session', () => {
    expect(addMonstersErrorMessage(blocked(EncounterBlockedReason.TOO_MANY_COMBATANTS))).toContain(
      'Não cabem mais combatentes',
    );
    expect(addMonstersErrorMessage(blocked(EncounterBlockedReason.ENCOUNTER_ENDED))).toContain(
      'já terminou',
    );
    expect(addMonstersErrorMessage(blocked(EncounterBlockedReason.NO_CURRENT_MAP))).toContain(
      'mapa',
    );
  });

  it("names the 40 combatants for an invalid request (the server has no detail for the cap) and never shows the server's text", () => {
    const text = addMonstersErrorMessage(
      new ConnectError('a combat has at most 40 combatants', Code.InvalidArgument),
    );
    expect(text).toContain('40 combatentes');
    expect(text).not.toContain('a combat has at most');
  });

  it('says in words that the campaign reached the limit of NPCs for combat creatures', () => {
    const text = addMonstersErrorMessage(
      new ConnectError(
        'the campaign already has 1000 NPCs made for the creatures',
        Code.ResourceExhausted,
      ),
    );
    expect(text).toContain('limite de NPCs');
    expect(text).toContain('reaproveitar não conta');
    expect(text).not.toContain('the campaign already has');
  });

  it('maps the other codes', () => {
    expect(addMonstersErrorMessage(new ConnectError('x', Code.PermissionDenied))).toContain(
      'Só o mestre',
    );
    expect(addMonstersErrorMessage(new ConnectError('x', Code.NotFound))).toContain(
      'não existe mais',
    );
    expect(addMonstersErrorMessage(new ConnectError('x', Code.Unavailable))).toContain(
      'o servidor não respondeu',
    );
  });
});
