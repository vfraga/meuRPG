import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  CombatantSide,
  CoverDegree,
  CoverSource,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';

describe('OrderList', () => {
  const rows = [
    combatant({
      id: 'pen',
      label: 'Pensantus',
      kind: CombatantKind.PLAYER,
      characterId: 'pen-c',
      hitPointsCurrent: 17,
      hitPointsMax: 23,
      initiative: 14,
      armorClass: 13,
    }),
    combatant({
      id: 'cap',
      label: 'Capitão Goblin',
      hitPointsCurrent: 19,
      hitPointsMax: 27,
      initiative: 16,
      armorClass: 17,
    }),
    combatant({
      id: 'g3',
      label: 'Goblin 3',
      hidden: true,
      hitPointsCurrent: 7,
      hitPointsMax: 7,
      initiative: 9,
      armorClass: 15,
    }),
  ];

  function setup() {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: rows, currentCombatantId: 'cap' }),
    );
    fixture.componentRef.setInput('adjustable', new Set(['pen-c']));
    const adjusted: string[] = [];
    const npcAdjusted: string[] = [];
    const revealed: { id: string; hidden: boolean }[] = [];
    fixture.componentInstance.adjust.subscribe((id) => adjusted.push(id));
    fixture.componentInstance.adjustNpc.subscribe((id) => npcAdjusted.push(id));
    fixture.componentInstance.reveal.subscribe((r) => revealed.push(r));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, adjusted, npcAdjusted, revealed };
  }

  it('marks the turn and shows the hit points the master sees', () => {
    const { el } = setup();
    const turn = el.querySelector('.row--turn');
    expect(turn?.textContent).toContain('Capitão Goblin');
    expect(turn?.textContent).toContain('Vez');
    expect(el.textContent).toContain('17 de 23');
  });

  it('says the armor class of each combatant, which only the master gets', () => {
    const { el } = setup();
    expect(el.textContent).toContain('CA 13');
    expect(el.textContent).toContain('CA 17');
  });

  it('opens "Dano/Cura" for a player\'s character by its sheet and for an NPC by its combatant', () => {
    const { el, adjusted, npcAdjusted } = setup();
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.row__adjust'));
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Dano ou cura em Pensantus',
      'Dano ou cura em Capitão Goblin',
      'Dano ou cura em Goblin 3',
    ]);
    buttons[0].click();
    buttons[1].click();
    expect(adjusted).toEqual(['pen-c']);
    expect(npcAdjusted).toEqual(['cap']);
  });

  it("says a hidden NPC is only the master's and reveals it with one tap", () => {
    const { el, revealed } = setup();
    expect(el.textContent).toContain('Só você vê este combatente.');
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Revelar aos jogadores'))!
      .click();
    expect(revealed).toEqual([{ id: 'g3', hidden: false }]);
  });

  describe('cover and sides (E9-07)', () => {
    function withCover(over: { marked?: CoverDegree; ally?: boolean } = {}) {
      const fixture = TestBed.createComponent(OrderList);
      const g2 = combatant({
        id: 'g2',
        label: 'Goblin 2',
        hitPointsCurrent: 7,
        hitPointsMax: 7,
        initiative: 12,
        coverMark: over.marked ?? CoverDegree.NONE,
        side: over.ally ? CombatantSide.PARTY : CombatantSide.ENEMY,
      });
      fixture.componentRef.setInput(
        'encounter',
        encounter({ combatants: [rows[0], rows[1], g2], currentCombatantId: 'pen' }),
      );
      fixture.componentRef.setInput(
        'coverAgainst',
        new Map([
          ['cap', { cover: CoverDegree.THREE_QUARTERS, source: CoverSource.MAP }],
          ['g2', { cover: CoverDegree.HALF, source: CoverSource.MAP }],
        ]),
      );
      fixture.componentRef.setInput('turnLabel', 'Pensantus');
      const picked: { id: string; cover: CoverDegree }[] = [];
      const sides: { id: string; side: CombatantSide }[] = [];
      fixture.componentInstance.cover.subscribe((c) => picked.push(c));
      fixture.componentInstance.side.subscribe((c) => sides.push(c));
      fixture.detectChanges();
      return { fixture, el: fixture.nativeElement as HTMLElement, picked, sides };
    }

    const plain = (t: string | null | undefined) =>
      (t ?? '')
        .replace(/\u00a0/g, ' ')
        .replace(/\s+/g, ' ')
        .trim();

    it("says each enemy's cover against whoever has the turn, with its source", () => {
      const { el } = withCover();
      const lines = Array.from(el.querySelectorAll('app-row-cover .cover'), (c) =>
        plain(c.textContent),
      );
      expect(lines).toEqual([
        'Três quartos (do mapa) contra o Pensantus',
        'Meia cobertura (do mapa) contra o Pensantus',
      ]);
    });

    it('opens the mark in place, a radio group named for the combatant, applying at once', () => {
      const { fixture, el, picked } = withCover({ marked: CoverDegree.HALF });
      const open = Array.from(el.querySelectorAll<HTMLButtonElement>('.action')).find(
        (b) => b.getAttribute('aria-label') === 'Marcar cobertura de Goblin 2',
      )!;
      open.click();
      fixture.detectChanges();
      const group = el.querySelector('[role="radiogroup"]')!;
      expect(group.getAttribute('aria-label')).toBe('Cobertura marcada de Goblin 2');
      const radios = Array.from(group.querySelectorAll<HTMLInputElement>('input'));
      expect(radios.map((r) => r.checked)).toEqual([false, true, false, false]);
      radios[2].click();
      expect(picked).toEqual([{ id: 'g2', cover: CoverDegree.THREE_QUARTERS }]);
      Array.from(el.querySelectorAll('button'))
        .find((b) => plain(b.textContent) === 'Fechar')!
        .click();
      fixture.detectChanges();
      expect(el.querySelector('[role="radiogroup"]')).toBeNull();
    });

    it('says "Aliado" as a side, apart from the conditions, and leaves the mark to the cover line', () => {
      const { el } = withCover({ marked: CoverDegree.THREE_QUARTERS, ally: true });
      const row = Array.from(el.querySelectorAll('li.row')).find((r) =>
        r.textContent?.includes('Goblin 2'),
      )!;
      expect(plain(row.querySelector('.row__meta')?.textContent)).toContain('Aliado');
      // Not inside "Condições de Goblin 2", and the master's list has no mark pill: the line says it.
      expect(Array.from(row.querySelectorAll('.tag'), (t) => plain(t.textContent))).toEqual([]);
    });
  });
});

describe("OrderList with a player's creatures (E9-12)", () => {
  const salvia = combatant({
    id: 's',
    label: 'Sálvia',
    kind: CombatantKind.PLAYER,
    characterId: 'sc',
    hitPointsCurrent: 38,
    hitPointsMax: 38,
    initiative: 13,
    concentrationSpell: 'spell:conjure-animals',
    concentrationSpellNamePt: 'Conjurar Animais',
  });
  const wolf = (n: number) =>
    combatant({
      id: `w${n}`,
      label: `Lobo atroz ${n}`,
      kind: CombatantKind.CREATURE,
      characterId: '',
      ownerCharacterId: 'sc',
      summonGroupId: 'cast',
      monsterKey: 'monster:dire-wolf',
      monsterNamePt: 'Lobo atroz',
      hitPointsCurrent: 37,
      hitPointsMax: 37,
      armorClass: 14,
      initiative: 10,
    });

  function setup() {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [salvia, wolf(1), wolf(2)],
        currentCombatantId: 'w1',
        turnGroupIds: ['w1', 'w2'],
      }),
    );
    const ended: string[] = [];
    fixture.componentInstance.endConcentration.subscribe((id) => ended.push(id));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, ended, fixture };
  }
  const text = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  it('draws a creature as a dashed round token, with whose it is, in a box named for the group, and a legend', () => {
    const { el } = setup();
    expect(el.querySelectorAll('.row__token.tk--creature').length).toBe(2);
    expect(text(el.querySelector('.row--turn, .row'))).toBeTruthy();
    const rows = Array.from(el.querySelectorAll('.row__sub'), (n) => text(n));
    expect(rows).toContain('CA 14 · da Sálvia');
    expect(text(el.querySelector('app-order-group'))).toContain('Lobos atrozes da Sálvia');
    expect(text(el.querySelector('.legend'))).toBe('P Jogador C NPC N Criatura de um jogador');
  });

  it('asks in place before the concentration is lost, with "Voltar" first, and says what goes with it', () => {
    const { el, ended, fixture } = setup();
    const open = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes('Perdeu a concentração'),
    )!;
    expect(text(open.closest('.row__extra'))).toContain(
      'Concentra em Conjurar Animais · 2 Lobos atrozes',
    );
    open.click();
    fixture.detectChanges();
    const ask = el.querySelector('[role=alertdialog]')!;
    expect(text(ask)).toContain('A Sálvia perdeu a concentração?');
    expect(text(ask)).toContain(
      'Conjurar Animais acaba e os 2 Lobos atrozes somem do combate, da ordem e do mapa.',
    );
    expect(text(ask)).not.toContain('Isso não se desfaz');
    const buttons = ask.querySelectorAll('button');
    expect(text(buttons[0])).toBe('Voltar');
    expect(text(buttons[1])).toBe('Dispensar os Lobos');
    buttons[1].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    expect(ended).toEqual(['s']);
  });

  it('offers to dismiss only a caster that has creatures: an NPC on Teia has the line and no question', () => {
    const fixture = TestBed.createComponent(OrderList);
    const web = combatant({
      id: 'cap',
      label: 'Capitão Goblin',
      concentrationSpell: 'spell:web',
      concentrationSpellNamePt: 'Teia',
      hitPointsCurrent: 20,
      hitPointsMax: 20,
    });
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [web, salvia, wolf(1)], currentCombatantId: 'cap' }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const row = (name: string) =>
      Array.from(el.querySelectorAll('li.row')).find((r) => r.textContent?.includes(name))!;
    expect(text(row('Capitão Goblin').querySelector('.row__extra'))).toBe('Concentra em Teia');
    expect(row('Capitão Goblin').textContent).not.toContain('Perdeu a concentração');
    // The caster with a casting's creatures does.
    expect(row('Sálvia').textContent).toContain('Perdeu a concentração');
    // The pill is one solid "Concentração", not a dashed "Concentrado" chip.
    expect(text(row('Capitão Goblin').querySelector('.row__conc'))).toContain('Concentração');
    expect(el.textContent).not.toContain('Concentrado');
  });

  it('shows a druid in a beast form to the master: "Na forma de Lobo" and the beast\'s pool beside the druid\'s own', () => {
    const wolfForm = {
      ...salvia,
      wildShapeBeastKey: 'monster:wolf',
      wildShapeBeastNamePt: 'Lobo',
      wildShapeHitPointsCurrent: 11,
      wildShapeHitPointsMax: 11,
      concentrationSpell: '',
    } as typeof salvia;
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [wolfForm], currentCombatantId: 's' }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(text(el.querySelector('app-form-tag'))).toContain('Na forma de Lobo');
    const hp = text(el.querySelector('.row__hp'));
    expect(hp).toContain('Lobo 11 de 11');
    expect(hp).toContain('38 de 38');
    expect(hp!.indexOf('11 de 11')).toBeLessThan(hp!.indexOf('38 de 38'));
  });

  it('shows no pool where the combat sends none, and no form tag for a druid in her own shape', () => {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          {
            ...salvia,
            wildShapeBeastKey: 'monster:wolf',
            wildShapeBeastNamePt: 'Lobo',
          } as typeof salvia,
        ],
        currentCombatantId: 's',
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('app-beast-pool')).toBeNull();
    const plain = TestBed.createComponent(OrderList);
    plain.componentRef.setInput(
      'encounter',
      encounter({ combatants: [salvia], currentCombatantId: 's' }),
    );
    plain.detectChanges();
    expect(plain.nativeElement.querySelector('app-form-tag')).toBeNull();
  });
});

describe('OrderList: the monsters of the bestiary (MR-042, RN-29, E10-08 state 5)', () => {
  it('tells the master the ND of a monster beside its armor class; a combatant with none says nothing of it', () => {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({
            id: 'b1',
            label: 'Bandido 1',
            hitPointsCurrent: 11,
            hitPointsMax: 11,
            initiative: 11,
            armorClass: 12,
            challengeRating: '1/8',
            bestiaryCreatureKey: 'monster:bandit',
          }),
          combatant({
            id: 'b3',
            label: 'Bandido 3',
            hidden: true,
            hitPointsCurrent: 11,
            hitPointsMax: 11,
            initiative: 5,
            armorClass: 12,
            challengeRating: '1/8',
            bestiaryCreatureKey: 'monster:bandit',
          }),
          combatant({
            id: 'g1',
            label: 'Goblin 1',
            hitPointsCurrent: 7,
            hitPointsMax: 7,
            initiative: 9,
            armorClass: 15,
          }),
        ],
        currentCombatantId: 'b1',
      }),
    );
    fixture.detectChanges();
    const subs = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.row__sub'),
    ).map((s) => s.textContent?.replace(/\s+/g, ' ').trim());
    expect(subs).toEqual(['NPC · ND 1/8 · CA 12', 'NPC · ND 1/8 · CA 12', 'NPC · CA 15']);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Escondido');
  });

  it('keeps the question to remove a combatant open, and sends nothing, while the page is busy', () => {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'cap', label: 'Capitão Goblin', initiative: 16 }),
          combatant({ id: 'g3', label: 'Goblin 3', initiative: 9 }),
        ],
        currentCombatantId: 'cap',
      }),
    );
    const removed: string[] = [];
    fixture.componentInstance.remove.subscribe((id) => removed.push(id));
    fixture.detectChanges();
    const cmp = fixture.componentInstance as unknown as {
      ask(id: string): void;
      confirmRemove(id: string): void;
      removing(): string | null;
    };
    cmp.ask('g3');
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    cmp.confirmRemove('g3');
    expect(removed).toEqual([]);
    expect(cmp.removing()).toBe('g3');
    fixture.componentRef.setInput('busy', false);
    fixture.detectChanges();
    cmp.confirmRemove('g3');
    expect(removed).toEqual(['g3']);
    expect(cmp.removing()).toBeNull();
  });
});
