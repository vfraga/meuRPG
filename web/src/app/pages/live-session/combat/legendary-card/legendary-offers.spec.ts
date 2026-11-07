import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CombatLogKind,
  CombatantKind,
  ReactionKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { settle } from '../../../../../test-settle';
import { LegendaryOffers, type LegendaryPick } from './legendary-offers';

/** The words of a node, with no icon ligature (`crown`, `bolt`) and the spaces folded. */
const plain = (node: Node | string | null | undefined): string => {
  let text = typeof node === 'string' ? node : '';
  if (node && typeof node !== 'string') {
    const copy = node.cloneNode(true) as HTMLElement;
    copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
    text = copy.textContent ?? '';
  }
  return text
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};
const off = (b: HTMLElement) => b.getAttribute('aria-disabled') === 'true';

const options = [
  {
    key: 'detect',
    name: 'Detect',
    namePt: 'Detectar',
    cost: 1,
    kind: 'text',
    text: 'English',
    affordable: true,
    attackKey: '',
  },
  {
    key: 'tail',
    name: 'Tail Attack',
    namePt: 'Ataque de cauda',
    cost: 1,
    kind: 'attack',
    text: 'x',
    affordable: true,
    attackKey: 'm#tail',
  },
  {
    key: 'wing',
    name: 'Wing Attack',
    namePt: 'Ataque de asas',
    cost: 2,
    kind: 'move',
    text: 'y',
    affordable: false,
    attackKey: '',
  },
];

function dragon(over: object = {}) {
  return combatant({
    id: 'd',
    label: 'Dragão vermelho adulto',
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 1,
      resistancePerDay: 3,
      resistanceLeft: 2,
      usableNow: true,
      options,
      ...over,
    },
  } as never);
}

const affordable = options.map((o) => ({ ...o, affordable: true }));
const tor = combatant({ id: 't', label: 'Tor', kind: CombatantKind.PLAYER });

describe("the master's legendary cards", () => {
  const api = { spendLegendaryAction: vi.fn() };
  const applied = encounter({ id: 'enc', revision: 9 } as never);

  function setup(dragonOver: object = {}, extra: object = {}, log: object = {}) {
    api.spendLegendaryAction.mockReset().mockResolvedValue(applied);
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient(api) }],
    });
    const fixture = TestBed.createComponent(LegendaryOffers);
    const state = new CombatState();
    const e = encounter({
      id: 'enc',
      combatants: [dragon(dragonOver), tor],
      currentCombatantId: 't',
      ...extra,
    } as never);
    fixture.componentRef.setInput('encounter', e);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('log', { undoable: () => null, ...log });
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, e, state };
  }
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b).includes(label),
    )!;
  const rows = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('[data-testid="legendary-row"]'));

  it('draws the card of a creature off turn: the badge, the count, the recovery, the hint and one row per option', () => {
    const { el } = setup();
    const text = plain(el);
    expect(text).toContain('Ações lendárias');
    expect(plain(el.querySelector('[data-testid="legendary-badge"]'))).toBe('Opcional');
    expect(text).toContain('Dragão vermelho adulto');
    expect(text).toContain('1 de 3');
    expect(text).toContain('voltam no começo do turno dele');
    expect(text).toContain('Use no fim do turno de Tor, uma opção de cada vez.');
    expect(rows(el).map((r) => plain(r.querySelector('.lrow__name')))).toEqual([
      'Detectarcusta 1',
      'Ataque de caudacusta 1',
      'Ataque de asascusta 2',
    ]);
    expect(plain(rows(el)[0].querySelector('.lrow__detail'))).toBe(
      'Você resolve: o app só conta o gasto.',
    );
    expect(el.querySelector('mat-icon')?.textContent?.trim()).toBe('crown');
  });

  it('never shows the SRD text of an option nor its English name', () => {
    const { el } = setup();
    expect(el.textContent).not.toContain('Tail Attack');
    expect(el.textContent).not.toContain('English');
  });

  it('gives each kind its button and turns off the one that costs more than what is left, with the reason', () => {
    const { el } = setup();
    const buttons = rows(el).map((r) => r.querySelector('button')!);
    expect(buttons.map((b) => plain(b))).toEqual(['Usar', 'Atacar', 'Usar']);
    expect(buttons.map(off)).toEqual([false, false, true]);
    expect(plain(rows(el)[2].querySelector('.lrow__detail'))).toBe('Custa 2; restam 1.');
    expect(buttons[2].classList.contains('btn--off')).toBe(true);
  });

  it('draws no card for the creature on turn, for a stat block without options, or for a player', () => {
    expect(
      setup({}, { currentCombatantId: 'd' }).el.querySelector('[data-testid="legendary-card"]'),
    ).toBeNull();
    expect(setup({ options: [] }).el.querySelector('[data-testid="legendary-card"]')).toBeNull();
  });

  describe('folding', () => {
    it('"Agora não" folds the card to "Ações lendárias · 1 de 3" and "Abrir" brings it back, sending nothing', () => {
      const { el, fixture } = setup();
      button(el, '').click();
      fixture.detectChanges();
      expect(plain(el)).toBe('Ações lendárias · 1 de 3Abrir');
      expect(rows(el)).toHaveLength(0);
      button(el, 'Abrir').click();
      fixture.detectChanges();
      expect(rows(el)).toHaveLength(3);
      expect(api.spendLegendaryAction).not.toHaveBeenCalled();
    });

    it('stays folded through an update of the combat, and opens again on the next turn', () => {
      const { el, fixture, e } = setup();
      button(el, '').click();
      fixture.detectChanges();
      fixture.componentRef.setInput('encounter', encounter({ ...e, revision: 5 } as never));
      fixture.detectChanges();
      expect(el.querySelector('[data-testid="legendary-folded"]')).not.toBeNull();
      fixture.componentRef.setInput(
        'encounter',
        encounter({ ...e, round: 3, combatants: [dragon(), tor] } as never),
      );
      fixture.detectChanges();
      expect(el.querySelector('[data-testid="legendary-card"]')).not.toBeNull();
    });
  });

  describe('"Usar"', () => {
    it('spends the option with the combat, the creature and a key, and applies the answer', async () => {
      const { el, fixture, state } = setup();
      const apply = vi.spyOn(state, 'apply');
      button(el, 'Usar').click();
      await settle(fixture);
      expect(api.spendLegendaryAction).toHaveBeenCalledWith(
        'camp',
        'enc',
        'd',
        'detect',
        expect.any(String),
      );
      expect(apply).toHaveBeenCalledWith(applied);
    });

    it('keeps the key across a lost answer and sends a new one for the next spend', async () => {
      const { el, fixture } = setup();
      api.spendLegendaryAction.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
      button(el, 'Usar').click();
      await settle(fixture);
      expect(plain(el.querySelector('[data-testid="legendary-error"]'))).toContain(
        'Sem conexão. A resposta não foi enviada; a oferta continua esperando você.',
      );
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      const [first, second] = api.spendLegendaryAction.mock.calls.map((c) => c[4]);
      expect(first).toBe(second);
      expect(el.querySelector('[data-testid="legendary-error"]')).toBeNull();
      button(el, 'Usar').click();
      await settle(fixture);
      expect(api.spendLegendaryAction.mock.calls[2][4]).not.toBe(first);
    });

    it('says why a refusal was one and that nothing was spent, with no retry button', async () => {
      const { el, fixture } = setup();
      api.spendLegendaryAction.mockRejectedValueOnce(
        new ConnectError('no', Code.FailedPrecondition),
      );
      button(el, 'Usar').click();
      await settle(fixture);
      expect(el.querySelector('[data-testid="legendary-error"]')).not.toBeNull();
      expect(button(el, 'Tentar de novo')).toBeUndefined();
    });

    it('turns every row off while an answer is in the air', async () => {
      const { el, fixture } = setup();
      let release: (e: unknown) => void = () => undefined;
      api.spendLegendaryAction.mockReturnValueOnce(new Promise((r) => (release = r)));
      button(el, 'Usar').click();
      fixture.detectChanges();
      expect(rows(el).every((r) => off(r.querySelector('button')!))).toBe(true);
      expect(plain(rows(el)[0])).toContain('Usando…');
      release(applied);
      await settle(fixture);
      expect(off(rows(el)[0].querySelector('button')!)).toBe(false);
    });
  });

  describe('"Atacar" and the move', () => {
    it('hands the attack row to the page, with the option and the attack key, and spends nothing itself', () => {
      const { el, fixture } = setup();
      const picks: LegendaryPick[] = [];
      fixture.componentInstance.attack.subscribe((p) => picks.push(p));
      button(el, 'Atacar').click();
      expect(picks).toHaveLength(1);
      expect(picks[0].combatantId).toBe('d');
      expect(picks[0].row.key).toBe('tail');
      expect(picks[0].row.attackKey).toBe('m#tail');
      expect(api.spendLegendaryAction).not.toHaveBeenCalled();
    });

    it('hands a move row to the page and says the token is dragged next, until "Cancelar"', async () => {
      const { el, fixture } = setup({ actionsLeft: 3, options: affordable });
      const moves: LegendaryPick[] = [];
      fixture.componentInstance.move.subscribe((p) => moves.push(p));
      button(el, 'Usar').click();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(moves).toHaveLength(0);
      rows(el)[2].querySelector('button')!.click();
      expect(moves.map((m) => m.row.key)).toEqual(['wing']);
      fixture.componentRef.setInput('armed', { combatantId: 'd', optionKey: 'wing' });
      fixture.detectChanges();
      expect(plain(el.querySelector('[data-testid="legendary-armed"]'))).toContain(
        'Arraste a ficha de Dragão vermelho adulto no mapa',
      );
      let cancelled = 0;
      fixture.componentInstance.cancelMove.subscribe(() => cancelled++);
      button(el, 'Cancelar').click();
      expect(cancelled).toBe(1);
    });

    it('turns the move off in a combat without a map, with the reason', () => {
      const { el, fixture } = setup({ actionsLeft: 3, options: affordable });
      fixture.componentRef.setInput('theatre', true);
      fixture.detectChanges();
      expect(off(rows(el)[2].querySelector('button')!)).toBe(true);
      expect(plain(rows(el)[2])).toContain('Sem mapa');
    });
  });

  describe('the states of the card', () => {
    it('a window that holds the table makes the card "Depois", with no rows and the reason', () => {
      const { el } = setup(
        {},
        {
          reactionWindows: [
            reactionWindow({
              id: 'p',
              kind: ReactionKind.PARRY,
              reactorId: 'c',
              answerNow: true,
              prompt: {
                case: 'parry',
                value: {
                  attackerLabel: 'Tor',
                  attackNamePt: 'Espada',
                  acBonus: 2,
                  armorClassWithParry: 17,
                },
              },
            } as never),
          ],
        },
      );
      expect(plain(el.querySelector('[data-testid="legendary-badge"]'))).toBe('Depois');
      expect(plain(el.querySelector('[data-testid="legendary-waiting"]'))).toContain(
        'Depois da janela acima: enquanto uma reação espera, nada anda.',
      );
      expect(rows(el)).toHaveLength(0);
      expect(el.querySelector('.offer--later')).not.toBeNull();
    });

    it('an incapacitated creature gets the dashed card with the reason and no rows', () => {
      const { el } = setup({ usableNow: false });
      expect(plain(el.querySelector('[data-testid="legendary-badge"]'))).toBe('Depois');
      expect(plain(el.querySelector('[data-testid="legendary-unusable"]'))).toBe(
        'Incapacitado: a criatura não pode usar ações lendárias agora.',
      );
      expect(rows(el)).toHaveLength(0);
    });
  });

  describe('after a spend', () => {
    const spent = {
      kind: CombatLogKind.LEGENDARY_SPENT,
      actorId: 'd',
      actorLabel: 'Dragão vermelho adulto',
      legendarySpent: { optionName: 'Wing Attack', cost: 2 },
    };
    const log = { undoable: () => spent };

    it('says what was spent in one line, in Portuguese, with no undo of its own (the log has it)', () => {
      const { el, fixture } = setup({}, {}, log);
      fixture.componentRef.setInput(
        'names',
        new Map([['d', new Map([['Wing Attack', 'Ataque de asas']])]]),
      );
      fixture.detectChanges();
      const box = el.querySelector('[data-testid="legendary-spent"]');
      expect(plain(box)).toBe('Gastou 2: Ataque de asas.');
      expect(box?.querySelector('button')).toBeNull();
      expect(plain(el)).not.toContain('Só você vê');
      expect(plain(el)).not.toContain('Desfazer última ação');
    });

    it('shows no line when the last undoable entry is another one', () => {
      const { el } = setup(
        {},
        {},
        { undoable: () => ({ kind: CombatLogKind.MOVED, actorId: 'd' }) },
      );
      expect(el.querySelector('[data-testid="legendary-spent"]')).toBeNull();
    });
  });
});
