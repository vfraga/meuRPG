import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import {
  CombatantKind,
  CombatantSide,
  CoverDegree,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterMode,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { COVER_CHOICES, reactorRows } from '../../../../core/combat/theatre';
import { CoverPanel } from './cover-panel';
import { MasterSpend } from './master-spend';
import { NoMapPanel, TheatreReaction } from './no-map-panel';
import { OfferPanel } from './offer-panel';
import { SpendSheet, type SpendSheetData } from './spend-sheet';
import { TheatrePill } from './theatre-pill';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const button = (el: HTMLElement, name: string) =>
  Array.from(el.querySelectorAll('button')).find(
    (b) => plain(b.textContent).includes(name) || b.getAttribute('aria-label') === name,
  ) as HTMLButtonElement;

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  mine: true,
  side: CombatantSide.PARTY,
  speedFt: 30,
  speedDft: 300,
  movementLeftFt: 30,
  movementLeftDft: 300,
});
const goblin = combatant({ id: 'g1', label: 'Goblin 1', movementLeftFt: 30, movementLeftDft: 300 });
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });
const theatre = (over: Parameters<typeof encounter>[0] = {}) =>
  encounter({
    mode: EncounterMode.THEATRE,
    currentCombatantId: 't',
    combatants: [toren, goblin, cap],
    ...over,
  });

describe('SpendSheet, "Gastar movimento" (RN-25, E10-04)', () => {
  function setup(opts: { fail?: unknown } = {}) {
    const state = new CombatState();
    state.apply(theatre());
    const spent: [string, number][] = [];
    const closed: unknown[] = [];
    const data: SpendSheetData = { campaignId: 'c', encounterId: 'enc', combatantId: 't', state };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: (r: unknown) => closed.push(r) } },
        {
          provide: CombatClient,
          useValue: {
            spendMovement: async (_c: string, _e: string, id: string, ft: number) => {
              if (opts.fail) {
                throw opts.fail;
              }
              spent.push([id, ft]);
              return {
                encounter: theatre({
                  revision: 2,
                  combatants: [
                    { ...toren, movementLeftFt: 10, movementLeftDft: 100, movementUsedDft: 200 },
                    goblin,
                    cap,
                  ],
                }),
                movementLeftDft: 100,
              };
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(SpendSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, spent, closed, state };
  }

  it('says what the turn has, starts at one step and shows what is left after it', () => {
    const { el } = setup();
    const text = plain(el.textContent);
    expect(text).toContain('Gastar movimento');
    expect(text).toContain('Você tem 9,0 m neste turno');
    expect(text).toContain('Em passos de 1,5 m');
    expect(plain(el.querySelector('output')?.textContent)).toBe('1,5 m');
    expect(plain(el.querySelector('.sum__row--after dd')?.textContent)).toBe('7,5 m');
    expect(plain(el.querySelector('.sum__row--after dt')?.textContent)).toBe('Depois restam');
    expect(text).toContain(
      'O app não confere o caminho nem o alcance. Combine com o mestre onde você ficou.',
    );
  });

  it('steps from 1,5 m to 1,5 m, and the minus waits at one step with aria-disabled (still focusable)', () => {
    const { fixture, el } = setup();
    const minus = button(el, 'Menos 1,5 m');
    expect(minus.getAttribute('aria-disabled')).toBe('true');
    const plus = button(el, 'Mais 1,5 m');
    for (let i = 0; i < 3; i++) {
      plus.click();
      fixture.detectChanges();
    }
    expect(plain(el.querySelector('output')?.textContent)).toBe('6,0 m');
    expect(plain(el.querySelector('.sum__row--after dd')?.textContent)).toBe('3,0 m');
    expect(button(el, 'Menos 1,5 m').getAttribute('aria-disabled')).toBe('false');
    expect(plain(el.querySelector('.actions .btn--go')?.textContent)).toBe('Gastar 6,0 m');
  });

  it('stops the plus at what is left', () => {
    const { fixture, el } = setup();
    for (let i = 0; i < 9; i++) {
      button(el, 'Mais 1,5 m').click();
      fixture.detectChanges();
    }
    expect(plain(el.querySelector('output')?.textContent)).toBe('9,0 m');
    expect(button(el, 'Mais 1,5 m').getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.textContent)).toContain('Só restam 9,0 m.');
  });

  it('spends in whole feet, hands the combat back and closes', async () => {
    const { fixture, el, spent, closed, state } = setup();
    for (let i = 0; i < 3; i++) {
      button(el, 'Mais 1,5 m').click();
      fixture.detectChanges();
    }
    button(el, 'Gastar 6,0 m').click();
    await fixture.whenStable();
    expect(spent).toEqual([['t', 20]]);
    expect(closed).toEqual([true]);
    expect(state.encounter()?.combatants[0].movementLeftDft).toBe(100);
  });

  it("says why a spend was refused, from the server's reason", async () => {
    const blocked = create(EncounterBlockedSchema, {
      reason: EncounterBlockedReason.NOT_YOUR_TURN,
    });
    const { fixture, el } = setup({
      fail: new ConnectError('x', Code.FailedPrecondition, undefined, [
        { desc: EncounterBlockedSchema, value: blocked },
      ]),
    });
    button(el, 'Gastar 1,5 m').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain('Não é a sua vez.');
  });

  it('reads the combat again after a refusal that means the screen is stale, and says what the server has left', async () => {
    const blocked = create(EncounterBlockedSchema, { reason: EncounterBlockedReason.TOO_FAR });
    const refreshed = theatre({
      revision: 5,
      combatants: [{ ...toren, movementLeftFt: 10, movementLeftDft: 100 }, goblin, cap],
    });
    let reads = 0;
    const err = new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: blocked },
    ]);
    const state = new CombatState();
    state.apply(theatre());
    const data: SpendSheetData = { campaignId: 'c', encounterId: 'enc', combatantId: 't', state };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        {
          provide: CombatClient,
          useValue: {
            spendMovement: async () => {
              throw err;
            },
            get: async () => {
              reads++;
              return refreshed;
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(SpendSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    button(el, 'Gastar 1,5 m').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(reads).toBe(1);
    expect(state.encounter()?.revision).toBe(5);
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Você só tem 3,0 m neste turno. Escolha menos.',
    );
  });

  it('cancels without spending', () => {
    const { el, closed, spent } = setup();
    button(el, 'Cancelar').click();
    expect(closed).toEqual([false]);
    expect(spent).toEqual([]);
  });
});

describe("MasterSpend, the master's movement for an NPC (E10-04 state 2)", () => {
  function setup(subject = goblin) {
    const state = new CombatState();
    state.apply(theatre({ currentCombatantId: subject.id }));
    const calls: number[] = [];
    TestBed.configureTestingModule({
      providers: [
        {
          provide: CombatClient,
          useValue: {
            spendMovement: async (_c: string, _e: string, _id: string, ft: number) => {
              calls.push(ft);
              return {
                encounter: theatre({
                  revision: 2,
                  combatants: [toren, { ...goblin, movementLeftFt: 10, movementLeftDft: 100 }, cap],
                }),
                movementLeftDft: 100,
              };
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(MasterSpend);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('encounterId', 'enc');
    fixture.componentRef.setInput('subject', subject);
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, calls };
  }

  it('shows the bar, the step and "Gastar movimento", and spends what was chosen', async () => {
    const { fixture, el, calls } = setup();
    expect(plain(el.textContent)).toContain('Movimento');
    expect(plain(el.textContent)).toContain('Sem mapa: só o número');
    expect(plain(el.textContent)).toContain('Gastar, em passos de 1,5 m');
    expect(plain(el.querySelector('output')?.textContent)).toBe('1,5 m');
    button(el, 'Mais 1,5 m').click();
    fixture.detectChanges();
    button(el, 'Gastar movimento').click();
    await fixture.whenStable();
    expect(calls).toEqual([10]);
  });

  it('is dashed with its reason when nothing is left', () => {
    const { el } = setup({ ...goblin, movementLeftFt: 0, movementLeftDft: 0 } as typeof goblin);
    const go = button(el, 'Gastar movimento');
    expect(go.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.textContent)).toContain('Goblin 1 já gastou todo o movimento.');
    expect(el.querySelector('app-spend-stepper')).toBeNull();
  });
});

describe('OfferPanel, "Oferecer ataque de oportunidade" (E10-04 state 3)', () => {
  function setup(
    rows = reactorRows(theatre({ currentCombatantId: 'g1' }), (c) =>
      c.id === 't' ? 'Guerreiro 4' : '',
    ),
  ) {
    const fixture = TestBed.createComponent(OfferPanel);
    fixture.componentRef.setInput('moverLabel', 'Goblin 1');
    fixture.componentRef.setInput('rows', rows);
    const offered: string[] = [];
    let cancelled = 0;
    fixture.componentInstance.offer.subscribe((id) => offered.push(id));
    fixture.componentInstance.cancel.subscribe(() => cancelled++);
    fixture.detectChanges();
    return {
      fixture,
      el: fixture.nativeElement as HTMLElement,
      offered,
      cancelled: () => cancelled,
    };
  }

  it('lists who the mover can have left the reach of, outlined buttons of the same size, and offers the one picked', async () => {
    const { fixture, el, offered } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const text = plain(el.textContent);
    expect(text).toContain('Oferecer ataque de oportunidade');
    expect(text).toContain('O Goblin 1 saiu do alcance de alguém?');
    expect(text).toContain('atacar gasta a reação dele');
    expect(text).toContain('Saiu do alcance de');
    expect(text).toContain('Toren');
    expect(text).toContain('Guerreiro 4');
    // The one that can react is picked, and the offer names it.
    const go = button(el, 'Oferecer a Toren');
    expect(go).toBeTruthy();
    // Both are outlined, at the same width: the page's one filled button is "Próximo turno".
    const no = button(el, 'Não oferecer');
    expect(
      [go, no].every(
        (b) => b.classList.contains('mat-mdc-outlined-button') && b.classList.contains('btn'),
      ),
    ).toBe(true);
    go.click();
    expect(offered).toEqual(['t']);
  });

  it('says "Reação usada" on a spent reaction and leaves it off', () => {
    const spent = reactorRows(
      theatre({
        currentCombatantId: 'g1',
        combatants: [{ ...toren, reactionUsed: true }, goblin, cap],
      }),
      () => '',
    );
    const { el } = setup(spent);
    expect(plain(el.textContent)).toContain('Reação usada');
    expect((el.querySelector('input[type="radio"]') as HTMLInputElement).disabled).toBe(true);
  });

  it('marks an offer that already waits (one per reactor) and leaves it off', () => {
    const rows = reactorRows(
      theatre({
        currentCombatantId: 'g1',
        opportunityOffers: [{ id: 'o', moverId: 'g1', reactorId: 't' } as never],
      }),
      () => '',
    );
    const { el } = setup(rows);
    expect(plain(el.textContent)).toContain('Oferta feita');
    expect((el.querySelector('input[type="radio"]') as HTMLInputElement).disabled).toBe(true);
  });

  it('cancels with "Não oferecer"', () => {
    const { el, cancelled } = setup();
    button(el, 'Não oferecer').click();
    expect(cancelled()).toBe(1);
  });
});

describe('CoverPanel, "Cobertura dos alvos" (E10-04 state 4)', () => {
  function setup(targets = [{ ...cap, coverMark: CoverDegree.HALF }, goblin]) {
    const fixture = TestBed.createComponent(CoverPanel);
    fixture.componentRef.setInput('targets', targets);
    const saved: { id: string; cover: CoverDegree }[] = [];
    fixture.componentInstance.cover.subscribe((c) => saved.push(c));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, saved };
  }

  it("says each target's degree in words", () => {
    const { el } = setup();
    const text = plain(el.textContent);
    expect(text).toContain('Cobertura dos alvos');
    expect(text).toContain('Sem mapa, a cobertura é a que você marca.');
    expect(text).toContain('Capitão Goblin');
    expect(text).toContain('Meia cobertura: +2 na CA');
    expect(text).toContain('Sem cobertura');
    expect(el.querySelectorAll('.mr-swatch--half').length).toBe(1);
  });

  it('opens four rows on the one marked, saves the one chosen and goes back to the list', () => {
    const { fixture, el, saved } = setup();
    button(el, 'Mudar a cobertura de Capitão Goblin').click();
    fixture.detectChanges();
    expect(plain(el.querySelector('h3')?.textContent)).toBe('Cobertura do Capitão Goblin');
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    expect(radios.length).toBe(COVER_CHOICES.length);
    expect(radios.map((r) => r.checked)).toEqual([false, true, false, false]);
    expect(plain(el.textContent)).toContain('Total (não dá para mirar)');
    radios[3].click();
    fixture.detectChanges();
    button(el, 'Salvar cobertura').click();
    fixture.detectChanges();
    expect(saved).toEqual([{ id: 'cap', cover: CoverDegree.TOTAL }]);
    expect(plain(el.querySelector('h3')?.textContent)).toBe('Cobertura dos alvos');
  });

  it('says each degree with its bonus (+2, +5), and names the target with its article', () => {
    const { fixture, el } = setup([
      { ...cap, label: 'Brisa', kind: CombatantKind.PLAYER, coverMark: CoverDegree.NONE },
    ]);
    button(el, 'Mudar a cobertura de Brisa').click();
    fixture.detectChanges();
    expect(plain(el.querySelector('h3')?.textContent)).toBe('Cobertura da Brisa');
    const text = plain(el.textContent);
    expect(text).toContain('+2 na CA e em Destreza');
    expect(text).toContain('+5 na CA e em Destreza');
    expect(text).toContain('“Meia cobertura (marcada pelo mestre)”');
  });

  it("opens the editor on a combatant the order's menu asked for (the fallback for a joint or master turn)", () => {
    const { fixture, el } = setup([]);
    fixture.componentRef.setInput('everyone', [goblin, cap]);
    fixture.componentRef.setInput('request', { id: 'g1', n: 1 });
    fixture.detectChanges();
    expect(plain(el.querySelector('h3')?.textContent)).toBe('Cobertura do Goblin 1');
  });

  it('cancels without sending anything', () => {
    const { fixture, el, saved } = setup();
    button(el, 'Mudar a cobertura de Goblin 1').click();
    fixture.detectChanges();
    button(el, 'Cancelar').click();
    fixture.detectChanges();
    expect(saved).toEqual([]);
  });
});

describe("the player's panels without a map (E10-04 state 10)", () => {
  it('says why once, with no map button', () => {
    const el = TestBed.createComponent(NoMapPanel);
    el.detectChanges();
    const text = plain(el.nativeElement.textContent);
    expect(text).toContain('Combate sem mapa');
    expect(text).toContain('Sem mapa, o app não sabe onde ninguém está.');
    expect(text).toContain(
      'O mestre diz quem está ao alcance e a que distância; você diz quanto andou.',
    );
    expect(el.nativeElement.querySelector('button')).toBeNull();
  });

  it('says where the opportunity attack comes from, and whether the reaction is free', () => {
    const f = TestBed.createComponent(TheatreReaction);
    f.detectChanges();
    expect(plain(f.nativeElement.textContent)).toContain('Disponível');
    expect(plain(f.nativeElement.textContent)).toContain(
      'a pergunta do ataque de oportunidade abre aqui',
    );
    f.componentRef.setInput('used', true);
    f.detectChanges();
    expect(plain(f.nativeElement.textContent)).toContain('Usada');
  });

  it('draws the pill of the mode', () => {
    const f = TestBed.createComponent(TheatrePill);
    f.detectChanges();
    expect(plain(f.nativeElement.textContent)).toContain('Teatro da mente');
  });
});
