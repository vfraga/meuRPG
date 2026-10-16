import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatClient } from '../../core/combat/combat-client';
import { combatant, encounter } from '../../core/combat/combat-testing';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  beastSpell,
  flat,
  isOff,
  summary,
  summonAnswer,
} from '../../core/creatures/creatures-testing';
import { SummonSheet, type SummonSheetData } from './summon-sheet';
import { nextTurn } from '../../../test-settle';
import { fakeCombatClient } from '../../core/combat/combat-client-testing';

/** Conjurar Animais in a combat (E9-12): the group's one initiative d20 follows the campaign's dice (RN-18). */
describe("SummonSheet in a combat: the group's initiative die", () => {
  async function setup(diceMode: DiceMode, preference: DicePreference) {
    const api = new FakeCreaturesClient();
    api.options = summonAnswer([beastSpell()], [[3, 2, 2]]);
    api.catalog = [summary('monster:wolf', 'Lobo', { challengeRating: '1/4' })];
    const castSpell = vi.fn(async () => ({
      encounter: encounter({
        combatants: [combatant({ id: 'w1', label: 'Lobo 1', monsterKey: 'monster:wolf' })],
      }),
      cast: {},
      summoned: ['w1'],
    }));
    const state = { apply: vi.fn() };
    const data: SummonSheetData = {
      campaignId: 'camp',
      characterId: 'char-1',
      spellKey: 'spell:conjure-animals',
      combat: {
        encounterId: 'enc',
        casterId: 'sal',
        diceMode,
        preference,
        state: state as never,
        concentrating: '',
      },
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: CreaturesClient, useValue: api },
        { provide: CombatClient, useValue: fakeCombatClient({ castSpell }) },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(SummonSheet);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
    };
    fixture.detectChanges();
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const go = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.includes('Conjurar Animais'),
      )!;
    const choose = async () => {
      // Two wolves: the option of 2 beasts, then "Escolher" twice.
      el.querySelectorAll<HTMLInputElement>('input[name=option]')[1].click();
      await settle();
      const wolf = () =>
        el.querySelector<HTMLButtonElement>(
          'button[aria-label="Escolher Lobo"], button[aria-label="Mais Lobo"]',
        )!;
      wolf().click();
      await settle();
      wolf().click();
      await settle();
    };
    return { el, castSpell, go, choose, settle };
  }

  it('physical dice only: the face is asked for from the start, the cast waits for it and sends it', async () => {
    const { el, go, choose, castSpell, settle } = await setup(
      DiceMode.PHYSICAL,
      DicePreference.UNSPECIFIED,
    );
    expect(el.querySelector('#initiative-face')).not.toBeNull();
    await choose();
    expect(isOff(go())).toBe(true);
    const field = el.querySelector<HTMLInputElement>('#initiative-face')!;
    field.value = '14';
    field.dispatchEvent(new Event('input'));
    await settle();
    expect(isOff(go())).toBe(false);
    go().click();
    await settle();
    const die = (castSpell.mock.calls[0] as unknown[])[6];
    expect(die).toEqual({ face: 14 });
  });

  it('app dice only: no field, and the server rolls it', async () => {
    const { el, go, choose, castSpell, settle } = await setup(
      DiceMode.APP,
      DicePreference.UNSPECIFIED,
    );
    expect(el.querySelector('#initiative-face')).toBeNull();
    await choose();
    go().click();
    await settle();
    expect((castSpell.mock.calls[0] as unknown[])[6]).toEqual({ inApp: true });
  });

  it("both allowed: the player's preference decides where it starts, and the other way is one tap away", async () => {
    const physical = await setup(DiceMode.PLAYERS_CHOOSE, DicePreference.PHYSICAL);
    expect(physical.el.querySelector('#initiative-face')).not.toBeNull();
    expect(flat(physical.el.querySelector('.roll__hint'))).toContain('Rolar no app');
    TestBed.resetTestingModule();
    const app = await setup(DiceMode.PLAYERS_CHOOSE, DicePreference.APP);
    expect(app.el.querySelector('#initiative-face')).toBeNull();
    expect(flat(app.el.querySelector('.roll__line'))).toContain('Digitar o d20 de um dado físico');
  });

  it('the slot and the quantity fold into one line once a creature is chosen, with "Mudar" to open them again', async () => {
    const { el, choose, settle } = await setup(DiceMode.APP, DicePreference.UNSPECIFIED);
    expect(el.querySelector('app-slot-picker')).not.toBeNull();
    await choose();
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(flat(el.querySelector('.setup'))).toContain('3º nível · 2 feras de ND 1 ou menos');
    el.querySelector<HTMLButtonElement>('.setup__edit')!.click();
    await settle();
    expect(el.querySelector('app-slot-picker')).not.toBeNull();
  });
});
