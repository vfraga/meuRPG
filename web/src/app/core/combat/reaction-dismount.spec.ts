import { ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from './combat-testing';
import { autoPasses, isOptionalReaction } from './reaction-autopass';
import { dismountGoneText, dismountView, isDismountWindow } from './reaction-dismount';
import { sheetWindow } from './reactions';

const window = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.DISMOUNT,
    reactorId: 'tor',
    reactorLabel: 'Tor',
    reactorIsPlayer: true,
    forYou: true,
    ...over,
  } as never);

const fight = (over: object = {}) =>
  encounter({
    round: 4,
    currentCombatantId: 'z1',
    combatants: [
      combatant({ id: 'tor', label: 'Tor', mine: true, mountedOnId: 'horse' }),
      combatant({ id: 'horse', label: 'Cavalo de guerra', riderId: 'tor' }),
      combatant({ id: 'z1', label: 'Zumbi 1' }),
    ],
    reactionWindows: [window()],
    ...over,
  } as never);

describe("the rider's dismount as its mount falls (#588, board 588-07)", () => {
  it('knows its window by the kind', () => {
    expect(isDismountWindow(window())).toBe(true);
    expect(isDismountWindow(window({ kind: ReactionKind.FEATURE }))).toBe(false);
  });

  it('opens by itself for the rider, though the window has no prompt of its own', () => {
    expect(sheetWindow(fight())?.id).toBe('w1');
    expect(sheetWindow(fight({ reactionWindows: [window({ forYou: false })] }))).toBeUndefined();
  });

  it('is never an optional reaction: nothing passes it by itself, not answering is a fall', () => {
    expect(isOptionalReaction(window())).toBe(false);
    expect(autoPasses(window())).toBe(false);
  });

  it('says what fell, what the reaction buys and what the table waits for', () => {
    const v = dismountView(fight(), window());
    expect(v.title).toBe('A montaria caiu');
    expect(v.subtitle).toBe('Reação · desmontar · Rodada 4');
    expect(v.lead).toBe('O Cavalo de guerra caiu.');
    expect(v.question).toBe('Usar a reação para desmontar de pé?');
    expect(v.note).toBe(
      'Com a reação, você desce de pé num quadrado ao lado da montaria. Sem ela, cai Derrubado a até 1,5 m.',
    );
    expect(v.chips).toEqual(['Gasta a sua reação', 'Volta no começo da sua vez']);
    expect(v.wait).toBe('A vez do Zumbi 1 espera a resposta. O mestre também pode responder.');
    expect(v.declineLabel).toBe('Cair derrubado');
    expect(v.useLabel).toBe('Desmontar de pé');
    expect(v.ariaLabel).toBe('O Cavalo de guerra caiu: usar a reação para desmontar de pé?');
  });

  it('takes the gender of the mount and of who is on turn from their names', () => {
    const e = fight({
      currentCombatantId: 'a1',
      combatants: [
        combatant({ id: 'tor', label: 'Tor', mine: true, mountedOnId: 'm' }),
        combatant({ id: 'm', label: 'Égua', riderId: 'tor' }),
        combatant({ id: 'a1', label: 'Aranha gigante' }),
      ],
    });
    const v = dismountView(e, window());
    expect(v.lead).toBe('A Égua caiu.');
    expect(v.wait).toBe(
      'A vez da Aranha gigante espera a resposta. O mestre também pode responder.',
    );
  });

  it('names no mount the player does not read, and no turn that is hidden; its own turn is "a sua vez"', () => {
    const hidden = fight({
      currentCombatantId: '',
      combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
    });
    const v = dismountView(hidden, window());
    expect(v.lead).toBe('A sua montaria caiu.');
    expect(v.wait).toBe('A vez espera a resposta. O mestre também pode responder.');
    expect(v.ariaLabel).toBe('A sua montaria caiu: usar a reação para desmontar de pé?');
    expect(dismountView(fight({ currentCombatantId: 'tor' }), window()).wait).toBe(
      'A sua vez espera a resposta. O mestre também pode responder.',
    );
  });

  it('says how a window that closed without the player ended', () => {
    expect(dismountGoneText(combatant({ id: 'tor', label: 'Tor', mountedOnId: 'horse' }))).toBe(
      'Não é mais preciso responder: você continua montado.',
    );
    expect(
      dismountGoneText(combatant({ id: 'tor', label: 'Tor', conditions: ['condition:prone'] })),
    ).toBe('A pergunta se fechou sem a sua resposta: você caiu Derrubado.');
    expect(dismountGoneText(combatant({ id: 'tor', label: 'Tor', reactionUsed: true }))).toBe(
      'O mestre respondeu por você: você desmontou de pé e gastou a reação.',
    );
    expect(dismountGoneText(null)).toBe('A pergunta se fechou sem a sua resposta.');
    expect(dismountGoneText(combatant({ id: 'tor', label: 'Tor' }))).toBe(
      'A pergunta se fechou sem a sua resposta.',
    );
  });
});
