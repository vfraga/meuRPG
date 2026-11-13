import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import {
  CombatantKind,
  CombatantSide,
  CoverDegree,
  CoverSource,
  SunlightTrait,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { Nudger } from '../../../../core/combat/nudger';
import { OrderList } from './order-list';
import { openAllRows } from './order-list-testing';

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
    openAllRows(fixture);
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
      'Dano/Cura: Pensantus',
      'Dano/Cura: Capitão Goblin',
      'Dano/Cura: Goblin 3',
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
      openAllRows(fixture);
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
      expect(plain(row.querySelector('.row__state-line')?.textContent)).toContain('Aliado');
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
    openAllRows(fixture);
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
      'Concentração: Conjurar Animais · 2 Lobos atrozes',
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
    openAllRows(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const row = (name: string) =>
      Array.from(el.querySelectorAll('li.row')).find((r) => r.textContent?.includes(name))!;
    expect(text(row('Capitão Goblin').querySelector('.row__extra'))).toContain(
      'Concentração: Teia',
    );
    expect(row('Capitão Goblin').textContent).not.toContain('Perdeu a concentração');
    // The caster with a casting's creatures does.
    expect(row('Sálvia').textContent).toContain('Perdeu a concentração');
    // One line with the hourglass, not a dashed "Concentrado" chip.
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
    openAllRows(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(text(el.querySelector('app-form-tag'))).toContain('Na forma de Lobo');
    const hp = text(el.querySelector('.row__open'));
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
    openAllRows(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('app-beast-pool')).toBeNull();
    const plain = TestBed.createComponent(OrderList);
    plain.componentRef.setInput(
      'encounter',
      encounter({ combatants: [salvia], currentCombatantId: 's' }),
    );
    plain.detectChanges();
    openAllRows(plain);
    expect(plain.nativeElement.querySelector('app-form-tag')).toBeNull();
  });
});

describe('OrderList: the monsters of the bestiary (MR-042, RN-29, E10-08 state 5)', () => {
  it('tells the master the ND of a monster in its opened row, and the armor class under the name; a combatant with none says nothing of it', () => {
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
    openAllRows(fixture);
    const subs = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.row__sub'),
    ).map((s) => s.textContent?.replace(/\s+/g, ' ').trim());
    expect(subs).toEqual(['Inic. 11 · CA 12', 'Escondido · inic. 5', 'Inic. 9 · CA 15']);
    // The ND is not a line of the opened row (C7-02): it is the first item of "Mais".
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.row__nd')).toBeNull();
    expect(el.querySelector('.row__open')!.textContent).not.toContain('Revelado');
    el.querySelector<HTMLButtonElement>('.row__more')!.click();
    fixture.detectChanges();
    const fact = document.querySelector('.row__menu-fact');
    expect(fact?.textContent?.replace(/\s+/g, ' ').trim()).toContain('ND 1/8');
    expect(el.textContent).toContain('Escondido');
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
    openAllRows(fixture);
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

describe('OrderList under Escudo Arcano and Ajuda (PM-03a)', () => {
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
  const pensantus = (bonus: number) =>
    combatant({
      id: 'pen',
      label: 'Pensantus',
      kind: CombatantKind.PLAYER,
      characterId: 'pen-c',
      hitPointsCurrent: 30,
      hitPointsMax: 30,
      initiative: 14,
      armorClass: 13,
      armorClassBonus: bonus,
    });
  const salvia = (over: Partial<Parameters<typeof combatant>[0]> = {}) =>
    combatant({
      id: 'sal',
      label: 'Sálvia',
      kind: CombatantKind.PLAYER,
      characterId: 'sal-c',
      hitPointsCurrent: 31,
      hitPointsMax: 43,
      hitPointsMaxBonus: 5,
      initiative: 13,
      armorClass: 14,
      ...over,
    });
  const hobgoblin = combatant({
    id: 'hob',
    label: 'Hobgoblin',
    hitPointsCurrent: 27,
    hitPointsMax: 27,
    initiative: 12,
    armorClass: 18,
  });

  function mount(combatants: ReturnType<typeof combatant>[]) {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants, currentCombatantId: 'hob' }),
    );
    fixture.componentRef.setInput('adjustable', new Set(['pen-c', 'sal-c']));
    fixture.detectChanges();
    openAllRows(fixture);
    return fixture;
  }
  const rowOf = (fixture: ReturnType<typeof mount>, name: string) =>
    Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row')).find((r) =>
      r.querySelector('.row__name')?.textContent?.includes(name),
    )!;

  it('says "CA 13 + 5" in bold and the label under the shield caster\'s name', () => {
    const row = rowOf(mount([pensantus(5), hobgoblin]), 'Pensantus');
    expect(flat(row.querySelector('.row__sub'))).toContain('CA 13 + 5');
    expect(flat(row.querySelector('.row__sub b'))).toBe('CA 13 + 5');
    expect(flat(row.querySelector('.row__effect'))).toContain('Escudo Arcano +5 até a vez dele');
  });

  it('says plain "CA 13" with no label when the caster has no shield', () => {
    const row = rowOf(mount([pensantus(0), hobgoblin]), 'Pensantus');
    expect(flat(row.querySelector('.row__sub'))).toContain('CA 13');
    expect(flat(row.querySelector('.row__sub'))).not.toContain('+ 5');
    expect(row.querySelector('.row__effect')).toBeNull();
  });

  it('says "31 de 43" with "+5 Ajuda" by the maximum, a striped bar piece and "Ajuda +5 PV" on the row', () => {
    const row = rowOf(mount([salvia(), hobgoblin]), 'Sálvia');
    expect(flat(row.querySelector('.row__hp-n'))).toBe('31/43+5 Ajuda');
    expect(flat(row.querySelector('.row__effect--aid'))).toContain('Ajuda +5 PV');
    const own = row.querySelector<HTMLElement>('.row__fill--own')!;
    const striped = row.querySelector<HTMLElement>('.row__fill--aid')!;
    expect(parseFloat(own.style.width)).toBeCloseTo((31 / 43) * 100);
    expect(parseFloat(striped.style.width)).toBe(0);
    expect(row.querySelector('.row__bar')?.getAttribute('aria-label')).toBe(
      '31 de 43 pontos de vida',
    );
  });

  it('offers "Encerrar Ajuda em Sálvia" only to a combatant under Ajuda', () => {
    const fixture = mount([salvia(), pensantus(0), hobgoblin]);
    const el = fixture.nativeElement as HTMLElement;
    const more = (name: string) =>
      el.querySelector<HTMLButtonElement>(`button[aria-label="Mais ações para ${name}"]`)!;
    more('Sálvia').click();
    fixture.detectChanges();
    const items = () =>
      Array.from(document.querySelectorAll('.mat-mdc-menu-item')).map((i) => flat(i));
    expect(items().some((i) => i?.includes('Encerrar Ajuda em Sálvia'))).toBe(true);
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.destroy();
    const other = mount([pensantus(0), hobgoblin]);
    (other.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('button[aria-label="Mais ações para Pensantus"]')!
      .click();
    other.detectChanges();
    expect(items().some((i) => i?.includes('Encerrar Ajuda'))).toBe(false);
  });

  it('puts an icon before every item of the row menu, with the labels of the board', () => {
    const fixture = mount([salvia(), hobgoblin]);
    const el = fixture.nativeElement as HTMLElement;
    const menuOf = (name: string) => {
      el.querySelector<HTMLButtonElement>(`button[aria-label="Mais ações para ${name}"]`)!.click();
      fixture.detectChanges();
      return Array.from(document.querySelectorAll('.mat-mdc-menu-item'));
    };
    const player = menuOf('Sálvia');
    expect(player.map((i) => flat(i))).toEqual([
      'favorite_borderEncerrar Ajuda em Sálvia',
      'editMudar condições',
      'shieldMarcar cobertura',
    ]);
    expect(player.every((i) => i.querySelector(':scope > .mat-icon') !== null)).toBe(true);
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    const npc = menuOf('Hobgoblin');
    expect(npc.map((i) => flat(i)?.replace(/^[a-z_]+(?=[A-Z])/, ''))).toContain('Esconder');
    expect(npc.every((i) => i.querySelector('.mat-icon') !== null)).toBe(true);
    expect(el.querySelectorAll('.row--effects')).toHaveLength(1);
  });

  function openQuestion(fixture: ReturnType<typeof mount>) {
    (
      fixture.componentInstance as unknown as { endingAid: { set(id: string): void } }
    ).endingAid.set('sal');
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).querySelector('app-end-aid-question')!;
  }

  it('asks in place, with the sum in words, the focus on "Cancelar" and the irreversible button outlined', () => {
    const fixture = mount([salvia({ hitPointsCurrent: 43 }), hobgoblin]);
    const question = openQuestion(fixture);
    const dialog = question.querySelector('[role="alertdialog"]')!;
    expect(dialog.getAttribute('aria-label')).toBe('Encerrar a Ajuda da Sálvia?');
    expect(flat(question.querySelector('.ask__text'))).toBe(
      'O máximo de PV volta a 38. Os PV atuais passam de 43 para 38: o que passa do novo máximo se perde, e isto não se desfaz.',
    );
    const buttons = Array.from(question.querySelectorAll('button'));
    expect(buttons.map((b) => flat(b))).toEqual(['Encerrar Ajuda', 'Cancelar']);
    expect(buttons[0].classList.contains('danger')).toBe(true);
    expect(document.activeElement).toBe(buttons[1]);
  });

  it('says the current hit points stay when they are below the new maximum', () => {
    const fixture = mount([salvia(), hobgoblin]);
    const question = openQuestion(fixture);
    expect(flat(question.querySelector('.ask__text'))).toBe(
      'O máximo de PV volta a 38. Os PV atuais ficam em 31: só o máximo cai, e isto não se desfaz.',
    );
  });

  it('sends the combatant on "Encerrar Ajuda", and nothing on "Cancelar"', () => {
    const fixture = mount([salvia(), hobgoblin]);
    const ended: string[] = [];
    fixture.componentInstance.endAid.subscribe((id) => ended.push(id));
    const question = openQuestion(fixture);
    const [end, cancel] = Array.from(question.querySelectorAll<HTMLButtonElement>('button'));
    cancel.click();
    fixture.detectChanges();
    expect(ended).toEqual([]);
    expect((fixture.nativeElement as HTMLElement).querySelector('app-end-aid-question')).toBeNull();
    openQuestion(fixture);
    (fixture.nativeElement as HTMLElement)
      .querySelectorAll<HTMLButtonElement>('app-end-aid-question button')[0]
      .click();
    fixture.detectChanges();
    expect(ended).toEqual(['sal']);
    expect(end).toBeDefined();
  });

  it('closes the question when the stream takes Ajuda away', () => {
    const fixture = mount([salvia(), hobgoblin]);
    openQuestion(fixture);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [salvia({ hitPointsMaxBonus: 0, hitPointsMax: 38 }), hobgoblin],
        currentCombatantId: 'hob',
      }),
    );
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('app-end-aid-question')).toBeNull();
  });

  it('says "O Escudo Arcano de Pensantus acabou" in the list when the bonus goes away, for six seconds', () => {
    vi.useFakeTimers();
    try {
      const fixture = mount([pensantus(5), hobgoblin]);
      const el = fixture.nativeElement as HTMLElement;
      expect(el.querySelector('.order__ended')).toBeNull();
      fixture.componentRef.setInput(
        'encounter',
        encounter({ combatants: [pensantus(0), hobgoblin], currentCombatantId: 'pen' }),
      );
      fixture.detectChanges();
      expect(flat(el.querySelector('.order__live[role="status"] .order__ended'))).toContain(
        'O Escudo Arcano de Pensantus acabou',
      );
      expect(el.querySelector('.row__effect')).toBeNull();
      vi.advanceTimersByTime(6000);
      fixture.detectChanges();
      expect(el.querySelector('.order__ended')).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not say the shield ended when its caster leaves the combat', () => {
    const fixture = mount([pensantus(5), hobgoblin]);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [hobgoblin], currentCombatantId: 'hob' }),
    );
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.order__ended')).toBeNull();
  });

  describe('Chamar a atenção', () => {
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
    ];
    function mount(campaignId: string) {
      const fixture = TestBed.createComponent(OrderList);
      fixture.componentRef.setInput(
        'encounter',
        encounter({ combatants: rows, currentCombatantId: 'cap' }),
      );
      fixture.componentRef.setInput('campaignId', campaignId);
      fixture.detectChanges();
      openAllRows(fixture);
      return fixture.nativeElement as HTMLElement;
    }

    it("is on a player's row alone, named for the character, and calls with the character", () => {
      const call = vi.spyOn(TestBed.inject(Nudger), 'call').mockResolvedValue(undefined);
      const el = mount('camp');
      const buttons = el.querySelectorAll<HTMLButtonElement>('app-nudge-button button');
      expect(buttons).toHaveLength(1);
      expect(buttons[0].getAttribute('aria-label')).toBe('Chamar a atenção de Pensantus');
      buttons[0].click();
      expect(call).toHaveBeenCalledWith('camp', 'pen-c');
    });

    it('is not there without the campaign', () => {
      expect(mount('').querySelector('app-nudge-button')).toBeNull();
    });
  });
});

describe('OrderList, the sun note', () => {
  function rows(...combatants: ReturnType<typeof combatant>[]): HTMLElement {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ sunlightOn: true, combatants, currentCombatantId: combatants[0].id }),
    );
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('ends the subline of a combatant with the sun trait that stands in the light, as muted text and not a chip', () => {
    const el = rows(
      combatant({
        id: 'gh',
        label: 'Carniçal',
        inSunlight: true,
        sunlightTrait: SunlightTrait.SENSITIVITY,
        initiative: 15,
      }),
    );
    const notes = el.querySelectorAll('[data-testid="order-sun"]');
    expect(notes.length).toBe(1);
    expect(notes[0].closest('.row__sub')?.textContent?.replace(/\s+/g, ' ').trim()).toContain(
      'Inic. 15 · Na luz do sol',
    );
    expect(el.querySelector('.mr-tag')).toBeNull();
  });

  it('shows nothing on a combatant without the trait, or with it out of the light', () => {
    const el = rows(
      combatant({ id: 'p1', label: 'Aria', inSunlight: true, initiative: 12 }),
      combatant({ id: 'go', label: 'Goblin', inSunlight: true, initiative: 10 }),
      combatant({
        id: 'gh',
        label: 'Carniçal',
        inSunlight: false,
        sunlightTrait: SunlightTrait.WEAKNESS,
        initiative: 9,
      }),
    );
    expect(el.querySelector('[data-testid="order-sun"]')).toBeNull();
    expect(el.textContent).not.toContain('Na luz do sol');
  });
});
