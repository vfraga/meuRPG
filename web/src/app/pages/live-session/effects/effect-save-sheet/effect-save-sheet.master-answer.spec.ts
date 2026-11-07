// An effect's saving throw held for a question: when the master answers it for the roller, the sheet drops the question
// and says so, while the window is still open.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ReactionKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { EffectPhase } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { heldLucky } from '../../../../core/held-rolls/held-roll-testing';
import { EffectsClient } from '../../../../core/effects/effects-client';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { EffectSaveSheet, type EffectSaveSheetData } from './effect-save-sheet';

const plain = (text: string | null | undefined) => (text ?? '').replace(/\s+/g, ' ').trim();

const save = () =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.EFFECT_SAVE,
    reactorId: 'me',
    reactorLabel: 'Pipo',
    reactorIsPlayer: true,
    prompt: {
      case: 'effectSave',
      value: {
        effectId: 'e1',
        sourceNamePt: 'Imobilizar Pessoa',
        ability: 'wis',
        abilityNamePt: 'Sabedoria',
        phase: EffectPhase.END,
        modifier: 1,
        bonusKnown: true,
        mode: 'normal',
        modeSourcesPt: [],
        extraDice: [],
        autoFail: false,
        handedToMaster: false,
        textPt: 'Teste de resistência de Sabedoria. Se passar, o efeito acaba sobre você.',
      },
    } as never,
  } as never);

// A hold of its own: the page remembers which questions this browser answered, across specs.
const lucky = heldLucky([1], 1, { holdId: 'hold-save-master' });

const combat = (offer?: typeof lucky) =>
  encounter({
    reactionWindows: [save()],
    combatants: [combatant({ id: 'me', label: 'Pipo', heldRollOffer: offer })],
  } as never);

describe('EffectSaveSheet: the master answers the question for the roller', () => {
  const api = { rollEffectSave: vi.fn() };

  function setup() {
    const state = new CombatState();
    state.apply(combat());
    const data: EffectSaveSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      window: save(),
      round: 3,
      state,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: EffectsClient, useValue: api },
        { provide: ResourceClient, useValue: { answerHeldRoll: vi.fn() } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(EffectSaveSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const buttons = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).map((b) => plain(b.textContent));
    const flush = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { fixture, el, state, buttons, flush };
  }

  /** Rolls in the app and gets the question back: the combat carries it on the roller and the window is still open. */
  async function asked(s: ReturnType<typeof setup>, kept = true) {
    api.rollEffectSave.mockResolvedValue({
      encounter: combat(kept ? lucky : undefined),
      held: lucky,
    });
    Array.from(s.el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => plain(b.textContent).includes('Rolar no app'))!
      .click();
    await s.flush();
  }

  beforeEach(() => api.rollEffectSave.mockReset());

  it('drops the question and says the master answered when the roller no longer carries it', async () => {
    const s = setup();
    await asked(s);
    expect(s.buttons()).toContain('Ficar com o 1');
    // The master answered: the next read of the combat has no question on the roller, the window still open.
    s.state.apply(combat());
    await s.flush();
    expect(plain(s.el.querySelector('[data-testid="sheet-held-vanished"]')?.textContent)).toContain(
      'O mestre respondeu por você.',
    );
    expect(s.buttons()).not.toContain('Ficar com o 1');
    expect(s.buttons().filter((b) => b.startsWith('Rolar de novo'))).toEqual([]);
    // The save the master settled cannot be rolled again from the footer.
    expect(s.buttons().filter((b) => b.includes('Rolar no app') || b.includes('Digitar'))).toEqual(
      [],
    );
  });

  it('keeps the question while the combat still carries it', async () => {
    const s = setup();
    await asked(s);
    s.state.apply(combat(lucky));
    await s.flush();
    expect(s.el.querySelector('[data-testid="sheet-held-vanished"]')).toBeNull();
    expect(s.buttons()).toContain('Ficar com o 1');
  });

  it('says the master answered when the window closed with the answer, in place of the generic line', async () => {
    const s = setup();
    await asked(s);
    // The master kept the d20: the save is settled, so the window and the question are both gone.
    s.state.apply(encounter({ combatants: [combatant({ id: 'me', label: 'Pipo' })] } as never));
    await s.flush();
    expect(plain(s.el.textContent)).toContain('O mestre respondeu por você.');
    expect(plain(s.el.textContent)).not.toContain('Este teste já foi respondido');
    expect(
      s.buttons().filter((b) => b.startsWith('Rolar de novo') || b.startsWith('Ficar com o')),
    ).toEqual([]);
  });
});
