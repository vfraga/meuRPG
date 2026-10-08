import { Code, ConnectError } from '@connectrpc/connect';

import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { blockedMessage, combatErrorMessage } from './combat-errors';

describe('combat errors', () => {
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
    expect(said(EncounterBlockedReason.ATTACKS_USED)).toMatch(/ataques/);
    expect(said(EncounterBlockedReason.REACTION_USED)).toMatch(/reação/);
    expect(said(EncounterBlockedReason.DEATH_SAVE_DUE)).toMatch(/teste contra a morte/);
    expect(said(EncounterBlockedReason.DEATH_SAVE_NOT_DUE)).toMatch(/Não há teste/);
    expect(said(EncounterBlockedReason.NOT_DYING)).toMatch(/três testes/);
    expect(said(EncounterBlockedReason.NOT_AWAITING_REACTION)).toMatch(/não espera/);
  });
});
