import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../core/combat/combat-client-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  flat,
  raven,
  summary,
} from '../../../core/creatures/creatures-testing';
import { nextTurn } from '../../../../test-settle';
import { CreatureCard } from './creature-card';

describe('CreatureCard, a bonded steed (#588, boards 588-23 to 588-26)', () => {
  let api: FakeCreaturesClient;
  let getEncounter: ReturnType<typeof vi.fn>;

  const estrela = () =>
    creature('steed-1', 'Estrela', {
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      source: CreatureSource.FIND_STEED,
      creatureType: 'celestial',
      bonded: true,
      hitPointsCurrent: 19,
      hitPointsMax: 19,
    });
  /** A running combat that holds the steed. */
  const combatWithSteed = () =>
    create(EncounterSchema, { id: 'enc-1', combatants: [{ id: 'c-9', creatureId: 'steed-1' }] });

  async function setup(inputs: Record<string, unknown> = {}) {
    api = new FakeCreaturesClient();
    api.creatures = [estrela()];
    api.blocks.set(
      'monster:warhorse',
      raven({ summary: summary('monster:warhorse', 'Cavalo de guerra', { sizePt: 'Grande' }) }),
    );
    getEncounter = vi.fn(async () => null);
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: CombatClient, useValue: fakeCombatClient({ get: getEncounter }) },
      ],
    });
    const fixture = TestBed.createComponent(CreatureCard);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('characterId', 'char-1');
    fixture.componentRef.setInput('ownerName', 'Lia');
    fixture.componentRef.setInput('creature', estrela());
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    const changed: string[] = [];
    fixture.componentInstance.changed.subscribe((m) => changed.push(m));
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
    const acts = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.acts .act'));
    const button = (name: string) =>
      Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        flat(b)?.startsWith(name),
      )!;
    /** Opens "⋮" and taps "Libertar Estrela". */
    const release = async () => {
      el.querySelector<HTMLButtonElement>('.js-more')!.click();
      await settle();
      const panels = document.querySelectorAll('.mat-mdc-menu-panel');
      const item = panels[panels.length - 1].querySelector<HTMLButtonElement>('button')!;
      item.click();
      await settle();
    };
    return { fixture, el, acts, button, release, settle, changed };
  }

  afterEach(() => document.body.replaceChildren());

  const blocked = (reason: EncounterBlockedReason) =>
    new ConnectError('no', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
    ]);

  it('carries the tag "Montaria vinculada" with the link icon, and the type\'s word in the subtitle', async () => {
    const { el } = await setup();
    const tag = el.querySelector('.head .mr-tag');
    expect(flat(tag)).toBe('Montaria vinculada');
    expect(tag?.querySelector('mat-icon')?.textContent?.trim()).toBe('link');
    expect(flat(el.querySelector('.sub'))).toBe('Cavalo de guerra · Grande · celestial');
  });

  it('a creature that is no bonded steed keeps its card: no tag, no "⋮", the origin in the subtitle', async () => {
    const { el } = await setup({ creature: creature('cr-1', 'Nanquim') });
    expect(el.querySelector('.head .mr-tag')).toBeNull();
    expect(el.querySelector('.js-more')).toBeNull();
    expect(flat(el.querySelector('.sub'))).toContain('Familiar de Lia');
  });

  it('the owner has "Renomear", "Dispensar" in danger and "⋮"; the master has "Corrigir PV" too; each is a 44 px target', async () => {
    const owner = await setup();
    expect(owner.acts().map((b) => flat(b) || b.getAttribute('aria-label'))).toEqual([
      'Renomear',
      'Dispensar',
      'Mais ações de Estrela',
    ]);
    expect(owner.acts()[1].classList).toContain('act--danger');
    for (const act of owner.acts()) {
      expect(parseFloat(getComputedStyle(act).minHeight)).toBe(44);
    }
    expect(parseFloat(getComputedStyle(owner.acts()[2]).minWidth)).toBe(44);

    TestBed.resetTestingModule();
    document.body.replaceChildren();
    const master = await setup({ isMaster: true });
    expect(master.acts().map((b) => flat(b) || b.getAttribute('aria-label'))).toEqual([
      'Renomear',
      'Corrigir PV',
      'Dispensar',
      'Mais ações de Estrela',
    ]);
  });

  it('"⋮" offers "Libertar Estrela" in danger, with what it does under it', async () => {
    const { el, settle } = await setup();
    el.querySelector<HTMLButtonElement>('.js-more')!.click();
    await settle();
    const panels = document.querySelectorAll('.mat-mdc-menu-panel');
    const item = panels[panels.length - 1].querySelector<HTMLElement>('[mat-menu-item]')!;
    expect(flat(item.querySelector('.release__t'))).toBe('Libertar Estrela');
    expect(item.classList).toContain('release');
    expect(flat(item.querySelector('.release__line'))).toBe(
      'Acaba o vínculo: a próxima é outra montaria.',
    );
  });

  it('"Dispensar" asks in place and keeps the bond: DISMISS, then the panel is told', async () => {
    const { el, acts, button, settle, changed } = await setup();
    acts()[1].click();
    await settle();
    expect(el.querySelector('.acts')).toBeNull();
    expect(flat(el.querySelector('app-steed-ask .ask__t'))).toBe('Dispensar Estrela?');
    expect(el.querySelector('.ask__chip')).toBeNull();
    button('Dispensar Estrela').click();
    await settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'dismiss');
    expect(changed).toEqual(['dismiss']);
  });

  it('"Libertar Estrela" asks in place and ends the bond: RELEASE', async () => {
    const { el, button, release, settle, changed } = await setup();
    await release();
    expect(flat(el.querySelector('app-steed-ask .ask__t'))).toBe('Libertar Estrela?');
    button('Libertar Estrela').click();
    await settle();
    expect(api.dismiss).toHaveBeenCalledWith('camp-1', 'steed-1', 'release');
    expect(changed).toEqual(['dismiss']);
  });

  it('"Voltar" brings the actions back with the focus on the one that asked', async () => {
    const { el, acts, button, release, settle } = await setup();
    acts()[1].click();
    await settle();
    button('Voltar').click();
    await settle();
    expect(document.activeElement).toBe(el.querySelector('.js-dismiss'));
    await release();
    button('Voltar').click();
    await settle();
    expect(document.activeElement).toBe(el.querySelector('.js-more'));
  });

  it("during a session, the owner's question says the action it spends when a combat holds the steed", async () => {
    const { el, acts, settle } = await setup({ live: true });
    getEncounter.mockResolvedValue(combatWithSteed());
    acts()[1].click();
    await settle();
    expect(getEncounter).toHaveBeenCalledWith('camp-1');
    expect(flat(el.querySelector('.ask__chip'))).toBe('● Gasta a sua ação');
  });

  it('no chip when the combat does not hold the steed, when no combat runs, or when the read fails', async () => {
    const other = await setup({ live: true });
    getEncounter.mockResolvedValue(
      create(EncounterSchema, { id: 'enc-1', combatants: [{ id: 'c-1', creatureId: 'cr-other' }] }),
    );
    other.acts()[1].click();
    await other.settle();
    expect(other.el.querySelector('app-steed-ask')).not.toBeNull();
    expect(other.el.querySelector('.ask__chip')).toBeNull();

    TestBed.resetTestingModule();
    document.body.replaceChildren();
    const failing = await setup({ live: true });
    getEncounter.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    failing.acts()[1].click();
    await failing.settle();
    expect(failing.el.querySelector('app-steed-ask')).not.toBeNull();
    expect(failing.el.querySelector('.ask__chip')).toBeNull();
  });

  it('the master spends no action, and no combat is read outside a session', async () => {
    const master = await setup({ live: true, isMaster: true });
    getEncounter.mockResolvedValue(combatWithSteed());
    master.acts()[2].click();
    await master.settle();
    expect(master.el.querySelector('.ask__chip')).toBeNull();

    TestBed.resetTestingModule();
    document.body.replaceChildren();
    const closed = await setup({ live: false });
    getEncounter.mockResolvedValue(combatWithSteed());
    closed.acts()[1].click();
    await closed.settle();
    expect(getEncounter).not.toHaveBeenCalled();
    expect(closed.el.querySelector('.ask__chip')).toBeNull();
  });

  it('a refusal for the turn closes the question and turns "Dispensar" off, muted, with "desligado" and the reason linked under the actions', async () => {
    const { el, acts, button, settle, changed } = await setup({ live: true });
    getEncounter.mockResolvedValue(combatWithSteed());
    acts()[1].click();
    await settle();
    api.failWith = blocked(EncounterBlockedReason.NOT_YOUR_TURN);
    button('Dispensar Estrela').click();
    await settle();
    expect(el.querySelector('app-steed-ask')).toBeNull();
    expect(changed).toEqual([]);
    const off = el.querySelector<HTMLButtonElement>('.js-dismiss')!;
    expect(off.getAttribute('aria-disabled')).toBe('true');
    expect(off.classList).toContain('act--off');
    expect(off.querySelector('mat-icon')?.textContent?.trim()).toBe('block');
    expect(flat(off)).toBe('Dispensar desligado');
    const why = el.querySelector(`#${off.getAttribute('aria-describedby')}`);
    expect(flat(why)).toBe('Só na vez de Lia: dispensar gasta a ação.');
    // Off is off: a tap asks nothing.
    off.click();
    await settle();
    expect(el.querySelector('app-steed-ask')).toBeNull();
  });

  it('"Você já usou a ação desta vez." for a spent action; the action comes back when the page says the combat changed', async () => {
    const { fixture, el, acts, button, settle } = await setup({ live: true });
    getEncounter.mockResolvedValue(combatWithSteed());
    acts()[1].click();
    await settle();
    api.failWith = blocked(EncounterBlockedReason.ACTION_USED);
    button('Dispensar Estrela').click();
    await settle();
    expect(flat(el.querySelector('.acts__why'))).toBe('Você já usou a ação desta vez.');
    fixture.componentRef.setInput('tick', 1);
    await settle();
    expect(el.querySelector('.acts__why')).toBeNull();
    expect(el.querySelector('.js-dismiss')?.getAttribute('aria-disabled')).toBeNull();
    expect(flat(el.querySelector('.js-dismiss'))).toBe('Dispensar');
  });
});
