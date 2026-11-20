import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  flat,
  isOff,
  raven,
  summary,
  summonAnswer,
} from '../../core/creatures/creatures-testing';
import { steedSpell } from '../../core/creatures/steed-testing';
import { nextTurn } from '../../../test-settle';
import { SummonSheet, type SummonSheetData } from './summon-sheet';

describe('SummonSheet: Convocar Montaria', () => {
  let api: FakeCreaturesClient;
  let close: ReturnType<typeof vi.fn>;

  async function setup(spell: SummonSpellOptions = steedSpell()) {
    api = new FakeCreaturesClient();
    api.options = summonAnswer([spell], [[2, 2, 2]]);
    api.blocks.set(
      'monster:warhorse',
      raven({
        summary: summary('monster:warhorse', 'Cavalo de guerra', { sizePt: 'Grande' }),
        armorClass: 11,
        hitPoints: 19,
        speedWalkFt: 60,
        speedFlyFt: 0,
      }),
    );
    api.blocks.set('monster:pony', raven({ summary: summary('monster:pony', 'Pônei') }));
    close = vi.fn();
    const data: SummonSheetData = {
      campaignId: 'camp-1',
      characterId: 'char-1',
      spellKey: 'spell:find-steed',
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: CreaturesClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
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
    const go = () => el.querySelector<HTMLButtonElement>('button.go')!;
    const pick = async (name: string) => {
      const r = Array.from(el.querySelectorAll<HTMLLabelElement>('label'))
        .find((l) => flat(l)?.includes(name))!
        .querySelector<HTMLInputElement>('input[type=radio]')!;
      r.checked = true;
      r.dispatchEvent(new Event('change'));
      await settle();
    };
    const foot = () => flat(el.querySelector('[foot]')) ?? '';
    return { fixture, el, go, pick, settle, foot };
  }

  const estrela = () =>
    creature('steed-1', 'Estrela', {
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      creatureType: 'celestial',
      armorClass: 11,
      hitPointsMax: 19,
      bonded: true,
    });

  it('shows every form of a short list at once, with no scroller of its own, in a dialog too', async () => {
    const { el } = await setup();
    const list = el.querySelector('app-creature-choice-list')!;
    expect(list.classList).toContain('flow');
    expect(list.querySelector('.rows')?.classList).toContain('rows--flow');
  });

  it('the first cast lists the forms, warns of the one too small to ride, and asks the type before it casts', async () => {
    const { el, go, pick, foot } = await setup();
    const rows = Array.from(el.querySelectorAll('app-creature-choice-list .row'));
    expect(rows.map((r) => r.querySelector('.row__title')?.textContent?.trim())).toEqual([
      'Cavalo de guerra',
      'Pônei',
    ]);
    expect(rows[1].querySelector('.row__warn')?.textContent).toContain(
      'Tamanho menor que o necessário para você montar',
    );
    // the subtitle, the slot folded from the start, the list named "Forma", each row with its silhouette and numbers.
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Magia de 2º nível · 10 minutos · um corcel vinculado',
    );
    expect(flat(el.querySelector('.setup__text'))).toBe('Espaço de 2º nível · 2 livres');
    expect(flat(el.querySelector('.setup__edit'))).toBe('Mudar');
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(flat(el.querySelector('#forms-cap'))).toBe('Forma');
    expect(rows[0].querySelector('app-creature-art')).not.toBeNull();
    expect(flat(rows[0].querySelector('.row__sub'))).toBe('Grande · 18 m · CA 11 · PV 19');
    expect(isOff(go())).toBe(true);
    expect(flat(go())).toBe('Convocar a montaria');
    expect(foot()).toContain('Escolha a forma e o tipo do corcel.');

    await pick('Cavalo de guerra');
    expect(flat(go())).toBe('Convocar Cavalo de guerra');
    expect(isOff(go())).toBe(true);
    expect(foot()).toContain('Escolha o tipo do corcel.');

    await pick('Celestial');
    expect(isOff(go())).toBe(false);
    expect(foot()).toContain(
      'Gasta um espaço de 2º nível ou maior. Só durante uma sessão, fora de combate.',
    );
    // only what it spends is bold.
    expect(flat(el.querySelector('[foot] .line b'))).toBe('Gasta um espaço de 2º nível ou maior.');
  });

  it("casts with the form and the type picked, by the server's value", async () => {
    const { go, pick, settle } = await setup();
    await pick('Pônei');
    await pick('Ínfero');
    go().click();
    await settle();
    expect(api.casts).toHaveLength(1);
    expect(api.casts[0].spellKey).toBe('spell:find-steed');
    expect(api.casts[0].slot).toEqual({ level: 2, pact: false });
    expect(api.casts[0].summon).toEqual({
      option: 0,
      creatureKeys: ['monster:pony'],
      names: [],
      creatureType: 'fiend',
    });
    // The steed is named by its form: the cast's result says who arrived.
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ names: ['Pônei'], count: 1 }));
  });

  it('with a bonded steed there is no list and no type: the fixed row, and "Convocar Estrela" casts the same steed', async () => {
    const { el, go, settle } = await setup(steedSpell({ bondedSteed: estrela() }));
    expect(el.querySelector('app-creature-choice-list')).toBeNull();
    expect(el.querySelector('[aria-label="Tipo do corcel"]')).toBeNull();
    expect(flat(el.querySelector('.same .mr-tag'))).toBe('Mesmo corcel');
    // the fixed row and its line come before the slot, "Como funciona" after it.
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Magia de 2º nível · 10 minutos · o seu corcel vinculado',
    );
    expect(flat(el.querySelector('.same__sub'))).toBe(
      'Cavalo de guerra · celestial · Grande · 18 m · CA 11 · PV 19',
    );
    const order = Array.from(el.querySelectorAll('.same, .setup, .mr-fold__head')).map(
      (n) => n.className.split(' ')[0],
    );
    expect(order).toEqual(['same', 'setup', 'mr-fold__head']);
    expect(flat(go())).toBe('Convocar Estrela');
    expect(isOff(go())).toBe(false);
    go().click();
    await settle();
    expect(api.casts[0].summon).toEqual({
      option: 0,
      creatureKeys: ['monster:warhorse'],
      names: [],
      creatureType: 'celestial',
    });
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ names: ['Estrela'] }));
  });

  it('a cast refused during a combat says the ten minutes, with the sheet open', async () => {
    const { el, go, settle } = await setup(steedSpell({ bondedSteed: estrela() }));
    api.failWith = new ConnectError('in combat', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.SUMMON_IN_COMBAT }),
      },
    ]);
    go().click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain(
      'Leva 10 minutos: não dá durante um combate.',
    );
    expect(close).not.toHaveBeenCalled();
  });

  it('"Mudar" opens the slot picker of the folded slot line', async () => {
    const { el, settle } = await setup();
    el.querySelector<HTMLButtonElement>('.setup__edit')!.click();
    await settle();
    expect(el.querySelector('app-slot-picker')).not.toBeNull();
  });

  it('a choice the server refuses for a bonded steed says to release it first', async () => {
    const { el, go, settle } = await setup(steedSpell({ bondedSteed: estrela() }));
    api.failWith = new ConnectError('no', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, {
          reason: EncounterBlockedReason.SUMMON_CHOICE_INVALID,
        }),
      },
    ]);
    go().click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain(
      'Estrela já tem forma e tipo. Para escolher outros, liberte Estrela antes.',
    );
  });

  it('keeps "Como funciona" closed in a dialog (tablet and up) too', async () => {
    const { el } = await setup();
    expect(el.textContent).not.toContain('A Inteligência mínima do corcel é 6.');
    const fold = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes('Como funciona'),
    );
    expect(fold?.getAttribute('aria-expanded')).toBe('false');
  });
});
