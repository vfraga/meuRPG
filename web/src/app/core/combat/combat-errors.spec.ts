import { Code, ConnectError } from '@connectrpc/connect';

import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { ResourceBlockedReason } from '../../../gen/meurpg/play/v1/resources_pb';
import {
  REROLL_PENDING_TEXT,
  blockedMessage,
  combatErrorMessage,
  pendingQuestionReasonText,
} from './combat-errors';
import { undoChangedText } from './combat-errors-limits';

describe('combat errors', () => {
  it('says an undo was refused because hit points changed, names who, and says nothing was undone', () => {
    const blocked = {
      reason: EncounterBlockedReason.UNDO_TARGET_CHANGED,
      combatantIds: ['a', 'b'],
    } as never;
    const names = new Map([
      ['a', 'Toren'],
      ['b', 'Goblin 2'],
    ]);
    expect(undoChangedText(blocked, (id) => names.get(id))).toMatch(
      /Toren, Goblin 2 mudaram.*Nada foi desfeito/,
    );
    // Without a name on screen the general sentence stands.
    expect(undoChangedText(blocked, () => undefined)).toMatch(/Nada foi desfeito/);
    expect(blockedMessage(blocked)).toMatch(/Nada foi desfeito/);
  });

  it('says every refusal of the legendary actions in words, and that nothing was spent', () => {
    for (const reason of [
      EncounterBlockedReason.LEGENDARY_OWN_TURN,
      EncounterBlockedReason.LEGENDARY_NONE_LEFT,
      EncounterBlockedReason.LEGENDARY_OPTION_UNKNOWN,
      EncounterBlockedReason.LEGENDARY_WRONG_KIND,
      EncounterBlockedReason.LEGENDARY_UNAVAILABLE,
      EncounterBlockedReason.LEGENDARY_RESISTANCE_NONE_LEFT,
      EncounterBlockedReason.LEGENDARY_RESISTANCE_UNAVAILABLE,
    ]) {
      expect(blockedMessage({ reason } as never)).toMatch(/[Nn]ada foi gasto\./);
    }
  });

  it('says every refusal of a spent action of a stat block in words, and that nothing was spent', () => {
    for (const reason of [
      EncounterBlockedReason.CREATURE_ACTION_RECHARGING,
      EncounterBlockedReason.CREATURE_ACTION_NO_USES_LEFT,
    ]) {
      expect(blockedMessage({ reason } as never)).toMatch(/[Nn]ada foi gasto\./);
    }
  });

  it('says a reserved character cannot fight, not that the combat is gone (R2-06)', () => {
    expect(
      blockedMessage({ reason: EncounterBlockedReason.CHARACTER_RESERVED } as never),
    ).toContain('reservado');
  });

  it('gives the effects that last their own words, without naming the cause', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.CANNOT_ACT } as never)).toBe(
      'Não dá para agir agora.',
    );
    expect(
      blockedMessage({ reason: EncounterBlockedReason.EXTRA_ACTION_UNAVAILABLE } as never),
    ).toMatch(/ação extra/);
  });

  it('says how far a refused move was, in meters', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.TOO_FAR, missingFt: 5 } as never)).toBe(
      'Esse caminho custa mais do que o movimento que sobra: faltam 1,5\u00a0m.',
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.SQUARE_OCCUPIED } as never)).toMatch(
      /Ocupado/,
    );
  });

  it('says how far, in tenths of a foot, and the limit of a jump that was too long (E9-06)', () => {
    expect(
      blockedMessage({
        reason: EncounterBlockedReason.TOO_FAR,
        missingDft: 29,
        missingFt: 3,
      } as never),
    ).toBe('Esse caminho custa mais do que o movimento que sobra: faltam 0,9\u00a0m.');
    expect(
      blockedMessage({
        reason: EncounterBlockedReason.TOO_FAR,
        missingDft: 50,
        jumpLimitDft: 160,
        jumpRunningStart: true,
      } as never),
    ).toBe('Longe demais para o seu salto: ele vai até 4,8\u00a0m com corrida. Faltam 1,5\u00a0m.');
    expect(
      blockedMessage({
        reason: EncounterBlockedReason.TOO_FAR,
        missingDft: 50,
        jumpLimitDft: 80,
        jumpRunningStart: false,
      } as never),
    ).toContain('até 2,4\u00a0m parado');
  });

  it('explains the refusals of walls, enemies and total cover (RN-21, MR-034)', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.MOVE_BLOCKED } as never)).toMatch(
      /parede ou outra criatura/,
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.ENEMY_IN_THE_WAY } as never)).toMatch(
      /inimigo está no caminho/,
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.TARGET_COVER_TOTAL } as never)).toMatch(
      /cobertura total/,
    );
  });

  it('says a wall of fire cast without its place was refused and that nothing was spent', () => {
    expect(
      blockedMessage({ reason: EncounterBlockedReason.WALL_PLACEMENT_REQUIRED } as never),
    ).toBe(
      'Esta magia cria uma muralha: falta dizer a forma, o tamanho e o lado que causa dano. Nada foi gasto.',
    );
  });

  it('says a wall is no place to stand and that the dead cannot be healed', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.WALL_ON_SQUARE } as never)).toMatch(
      'Esse quadrado é uma parede: ninguém fica dentro dele.',
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.NO_FREE_SQUARE } as never)).toBe(
      'Não há quadrado livre longe dos jogadores neste mapa.',
    );
    expect(blockedMessage({ reason: EncounterBlockedReason.TARGET_DEAD } as never)).toBe(
      'Não dá para curar quem já morreu.',
    );
  });

  it('says a locked door stopped the very first step: nothing moved, and only the master unlocks it (RN-26)', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.DOOR_LOCKED } as never)).toBe(
      'A porta está trancada: você não saiu do lugar. Só o mestre a destranca.',
    );
  });

  it('says the turn waits for an opportunity attack (MR-034)', () => {
    expect(blockedMessage({ reason: EncounterBlockedReason.OPPORTUNITY_PENDING } as never)).toMatch(
      /Esperando a reação do mestre/,
    );
  });

  it('says only that the turn waits for the master when a question about hidden creatures holds it (RN-10)', () => {
    const text = blockedMessage({
      reason: EncounterBlockedReason.HIDDEN_REVEAL_PENDING,
    } as never);
    expect(text).toBe('Esperando o mestre.');
    expect(text).not.toMatch(/escond|criatura|magia|área/i);
  });

  it('speaks by code when there is no typed detail', () => {
    expect(combatErrorMessage(new ConnectError('x', Code.Aborted))).toMatch(/mudou/);
    expect(combatErrorMessage(new ConnectError('x', Code.NotFound))).toMatch(/não existe mais/);
    expect(combatErrorMessage(new Error('network'))).toMatch(/servidor/);
  });
});

describe('the reasons slice 6.5c added', () => {
  const said = (reason: EncounterBlockedReason, more: object = {}) =>
    blockedMessage({ reason, ...more } as never);

  it('says why a spell, an action or a death save was refused', () => {
    expect(said(EncounterBlockedReason.NO_SLOT, { minLevel: 2 })).toBe(
      'Não há espaço de 2º\u00a0nível ou maior livre.',
    );
    expect(said(EncounterBlockedReason.NO_SLOT)).toBe('Não há espaço de magia livre.');
    expect(said(EncounterBlockedReason.NO_USES, { recharge: 1 })).toBe(
      'Sem usos: volta num descanso curto.',
    );
    expect(said(EncounterBlockedReason.ALREADY_USED_THIS_TURN)).toMatch(/uma vez por turno/);
    expect(said(EncounterBlockedReason.BONUS_ACTION_USED)).toMatch(/ação bônus/);
    expect(said(EncounterBlockedReason.BONUS_ACTION_SPELL_LIMIT)).toMatch(/truque de 1 ação/);
    expect(said(EncounterBlockedReason.ATTACKS_USED)).toMatch(/ataques/);
    expect(said(EncounterBlockedReason.ATTACK_ACTION_FIRST)).toMatch(/Rajada de Golpes.*Atacar/);
    expect(said(EncounterBlockedReason.REACTION_USED)).toMatch(/reação/);
    expect(said(EncounterBlockedReason.DEATH_SAVE_DUE)).toMatch(/teste contra a morte/);
    expect(said(EncounterBlockedReason.DEATH_SAVE_NOT_DUE)).toMatch(/Não há teste/);
    expect(said(EncounterBlockedReason.NOT_DYING)).toMatch(/três testes/);
    expect(said(EncounterBlockedReason.NOT_AWAITING_REACTION)).toMatch(/não espera/);
  });
});

describe('the question that holds a roll before a resource is spent', () => {
  it('says a roll waits for the answer about rolling the d20 again', () => {
    expect(pendingQuestionReasonText(ResourceBlockedReason.REROLL_PENDING)).toBe(
      REROLL_PENDING_TEXT,
    );
    expect(REROLL_PENDING_TEXT).toMatch(/d20 de novo/);
  });

  it('says nothing for a reason that is not a pending question', () => {
    expect(pendingQuestionReasonText(ResourceBlockedReason.UNSPECIFIED)).toBe('');
  });
});
