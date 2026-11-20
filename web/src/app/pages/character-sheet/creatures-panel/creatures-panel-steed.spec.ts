import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import {
  CreatureSource,
  GetSummonOptionsResponseSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { create } from '@bufbuild/protobuf';

import { type Encounter, EncounterSchema } from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../core/combat/combat-client-testing';
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

describe('CreaturesPanel, a bonded steed that is away: the dashed "Fora" row', () => {
  let api: FakeCreaturesClient;
  const getEncounter = vi.fn(async (): Promise<Encounter | null> => null);
  const sessions = signal<readonly { campaignId: string }[]>([{ campaignId: 'camp-1' }]);

  const estrela = () =>
    creature('steed-1', 'Estrela', {
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      source: CreatureSource.FIND_STEED,
      creatureType: 'celestial',
      bonded: true,
    });

  async function setup(
    opts: { live?: boolean; away?: boolean; master?: boolean; unprepared?: boolean } = {},
  ) {
    api = new FakeCreaturesClient();
    api.creatures = opts.live ? [estrela()] : [];
    api.options = summonAnswer([steedSpell(opts.away === false ? {} : { bondedSteed: estrela() })]);
    if (opts.unprepared) {
      // The spell is not prepared: no summon spell at all, and the bond on the answer itself.
      api.options = create(GetSummonOptionsResponseSchema, { bondedSteed: estrela() });
    }
    api.blocks.set('monster:warhorse', raven());
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: CombatClient, useValue: fakeCombatClient({ get: getEncounter }) },
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

  afterEach(() => {
    document.body.replaceChildren();
    getEncounter.mockReset();
    getEncounter.mockResolvedValue(null);
  });

  it('draws the steed that is not among the creatures as a dashed row: the name, "Fora", the form and type, and how it comes back', async () => {
    const { el, away } = await setup();
    expect(el.querySelector('app-creature-card')).toBeNull();
    expect(el.querySelector('.empty')).toBeNull();
    const row = away()!;
    expect(flat(row.querySelector('h3'))).toBe('Estrela');
    // "Fora" is the one visible word that says it is away; "da mesa" is for a screen reader, and the line does not repeat it.
    expect(flat(row.querySelector('.mr-tag'))).toBe('Fora da mesa');
    expect(row.querySelector('.mr-tag .mr-visually-hidden')?.textContent).toBe(' da mesa');
    expect(flat(row.querySelector('.away__sub'))).toBe('Cavalo de guerra · celestial');
    // The name is 15 px bold sans and the line under it is one line.
    const name = getComputedStyle(row.querySelector('h3')!);
    expect(name.fontSize).toBe('var(--mr-font-15)');
    expect(name.fontFamily).toBe('var(--mr-font-sans)');
    expect(name.fontWeight).toBe('700');
    expect(getComputedStyle(row.querySelector('.away__sub')!).whiteSpace).toBe('nowrap');
    // the muted silhouette, no line in the row; how it comes back is under the cast button.
    expect(row.querySelector('app-creature-art')).not.toBeNull();
    expect(row.querySelector('.away__line')).toBeNull();
    expect(getComputedStyle(row.querySelector('.mr-tag')!).borderTopStyle).toBe('dashed');
    expect(flat(el.querySelector('.cast__cost'))).toBe(
      'Traz Estrela de volta, com a mesma forma, o mesmo tipo e os PV cheios.',
    );
    expect(getComputedStyle(row.querySelector('.away')!).borderTopStyle).toBe('dashed');
    expect(row.querySelector('.tiles')).toBeNull();
  });

  it('no row while the steed is among the creatures, and none without a bond', async () => {
    const here = await setup({ live: true });
    expect(here.away()).toBeNull();
    expect(here.el.querySelectorAll('app-creature-card').length).toBe(1);
    // with the steed here, the cast only fills its hit points.
    expect(flat(here.el.querySelector('.cast__cost'))).toBe(
      'Gasta um espaço de 2º nível ou maior. Só durante uma sessão, fora de combate. Com Estrela aqui, os PV voltam cheios.',
    );

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

  it('in a combat that holds the character, the cast button is dashed and says the ten minutes', async () => {
    getEncounter.mockResolvedValue(
      create(EncounterSchema, { id: 'enc-1', combatants: [{ id: 'c-1', characterId: 'char-1' }] }),
    );
    const { el } = await setup();
    const cast = el.querySelector<HTMLButtonElement>('.js-cast')!;
    expect(getEncounter).toHaveBeenCalledWith('camp-1');
    expect(cast.classList).toContain('mr-button--off');
    expect(cast.getAttribute('aria-disabled')).toBe('true');
    expect(flat(el.querySelector('.cast__why'))).toBe(
      'Leva 10 minutos: não dá durante um combate.',
    );
    // The reason is right under the button, before the cost line.
    expect(cast.nextElementSibling).toBe(el.querySelector('.cast__why'));
    expect(el.querySelector('.cast__why')?.nextElementSibling).toBe(
      el.querySelector('.cast__cost'),
    );
    expect(el.querySelector('.cast')?.classList).toContain('mr-outline-ink');
  });

  it('a combat without the character, or none, leaves the cast button on; outside a session it says so', async () => {
    getEncounter.mockResolvedValue(
      create(EncounterSchema, { id: 'enc-1', combatants: [{ id: 'c-1', characterId: 'char-2' }] }),
    );
    const other = await setup();
    expect(other.el.querySelector('.js-cast')?.classList).not.toContain('mr-button--off');
    expect(other.el.querySelector('.cast__why')).toBeNull();

    TestBed.resetTestingModule();
    document.body.replaceChildren();
    sessions.set([]);
    const closed = await setup();
    sessions.set([{ campaignId: 'camp-1' }]);
    expect(flat(closed.el.querySelector('.cast__why'))).toBe('Agora não há sessão aberta.');
  });

  it('the row comes from the bond, not from the spell: a character that cannot cast Convocar Montaria now still has it, with its release', async () => {
    const { el, away, release, button, settle } = await setup({ unprepared: true });
    expect(el.querySelector('.js-cast')).toBeNull();
    expect(flat(away()!.querySelector('h3'))).toBe('Estrela');
    await release();
    button('Libertar Estrela').click();
    await settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'release');
  });

  it("the master reads the row too, with the sentence in the owner's name", async () => {
    const { away, release } = await setup({ master: true });
    expect(flat(away()!.querySelector('h3'))).toBe('Estrela');
    await release();
    expect(flat(away()!.querySelector('.ask__d'))).toContain('da ficha de Lia');
  });
});
