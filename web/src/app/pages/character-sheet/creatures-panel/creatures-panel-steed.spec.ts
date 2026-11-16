import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  flat,
  raven,
  summonAnswer,
} from '../../../core/creatures/creatures-testing';
import { steedSpell } from '../../../core/creatures/steed-testing';
import { OpenSessions } from '../../../core/play/open-sessions';
import { nextTurn } from '../../../../test-settle';
import { CreaturesPanel } from './creatures-panel';

describe('CreaturesPanel, a bonded steed that is away: the dashed "Fora" row (#588, board 588-26 card 1)', () => {
  let api: FakeCreaturesClient;
  const sessions = signal<readonly { campaignId: string }[]>([{ campaignId: 'camp-1' }]);

  const estrela = () =>
    creature('steed-1', 'Estrela', {
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      source: CreatureSource.FIND_STEED,
      creatureType: 'celestial',
      bonded: true,
    });

  async function setup(opts: { live?: boolean; away?: boolean; master?: boolean } = {}) {
    api = new FakeCreaturesClient();
    api.creatures = opts.live ? [estrela()] : [];
    api.options = summonAnswer([steedSpell(opts.away === false ? {} : { bondedSteed: estrela() })]);
    api.blocks.set('monster:warhorse', raven());
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: OpenSessions, useValue: { sessions } },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CreaturesPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('characterId', 'char-1');
    fixture.componentRef.setInput('characterName', 'Lia');
    fixture.componentRef.setInput('isMaster', opts.master ?? false);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
    };
    await settle();
    document.body.appendChild(fixture.nativeElement);
    const el = fixture.nativeElement as HTMLElement;
    const away = () => el.querySelector<HTMLElement>('app-steed-away');
    const button = (name: string) =>
      Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.startsWith(name),
      )!;
    const release = async () => {
      away()!.querySelector<HTMLButtonElement>('.js-more')!.click();
      await settle();
      const panels = document.querySelectorAll('.mat-mdc-menu-panel');
      panels[panels.length - 1].querySelector<HTMLButtonElement>('button')!.click();
      await settle();
    };
    return { fixture, el, away, button, release, settle };
  }

  afterEach(() => document.body.replaceChildren());

  it('draws the steed that is not among the creatures as a dashed row: the name, "Fora", the form and type, and how it comes back', async () => {
    const { el, away } = await setup();
    expect(el.querySelector('app-creature-card')).toBeNull();
    expect(el.querySelector('.empty')).toBeNull();
    const row = away()!;
    expect(flat(row.querySelector('h3'))).toBe('Estrela');
    expect(flat(row.querySelector('.mr-tag'))).toBe('Fora');
    expect(flat(row.querySelector('.away__sub'))).toBe('Cavalo de guerra · celestial');
    expect(flat(row.querySelector('.away__line'))).toBe('Convocar Montaria traz Estrela de volta.');
    expect(getComputedStyle(row.querySelector('.away')!).borderTopStyle).toBe('dashed');
    expect(row.querySelector('.tiles')).toBeNull();
  });

  it('no row while the steed is among the creatures, and none without a bond', async () => {
    const here = await setup({ live: true });
    expect(here.away()).toBeNull();
    expect(here.el.querySelectorAll('app-creature-card').length).toBe(1);

    TestBed.resetTestingModule();
    document.body.replaceChildren();
    const none = await setup({ away: false });
    expect(none.away()).toBeNull();
    expect(flat(none.el.querySelector('.empty'))).toContain('Nenhuma criatura ainda.');
  });

  it('"⋮" › "Libertar Estrela" asks in the row, ends the bond (RELEASE), and the panel reads again', async () => {
    const { away, button, release, settle } = await setup();
    await release();
    expect(flat(away()!.querySelector('.ask__t'))).toBe('Libertar Estrela?');
    expect(away()!.querySelector('.ask__chip')).toBeNull();
    api.options = summonAnswer([steedSpell()]);
    const reads = api.list.mock.calls.length;
    button('Libertar Estrela').click();
    await settle();
    await settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'release');
    expect(api.list.mock.calls.length).toBeGreaterThan(reads);
    expect(away()).toBeNull();
    expect(document.activeElement?.textContent?.trim()).toBe('Criaturas');
  });

  it('"Voltar" keeps the row, with the focus back on "⋮"', async () => {
    const { away, button, release, settle } = await setup();
    await release();
    button('Voltar').click();
    await settle();
    expect(away()!.querySelector('.ask')).toBeNull();
    expect(document.activeElement).toBe(away()!.querySelector('.js-more'));
    expect(api.dismiss).not.toHaveBeenCalled();
  });

  it("the master reads the row too, with the sentence in the owner's name", async () => {
    const { away, release } = await setup({ master: true });
    expect(flat(away()!.querySelector('h3'))).toBe('Estrela');
    await release();
    expect(flat(away()!.querySelector('.ask__d'))).toContain('da ficha de Lia');
  });
});
