import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { Code, ConnectError } from '@connectrpc/connect';

import { CombatantKind, ReactionKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { EffectsClient } from '../../../../core/effects/effects-client';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { settle } from '../../../../../test-settle';
import { ParryOffer } from './parry-offer';
import { ReactionQueue } from './reaction-queue';

const plain = (text: string | null | undefined) =>
  (text ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

const trait = (left: number) => ({
  actionsPerRound: 3,
  actionsLeft: 3,
  resistancePerDay: 3,
  resistanceLeft: left,
  usableNow: true,
  options: [],
});

const combatants = (left = 3) => [
  combatant({ id: 't', label: 'Tor', kind: CombatantKind.PLAYER }),
  combatant({ id: 'c', label: 'Capitão bandido' }),
  combatant({ id: 'd', label: 'Dragão vermelho adulto', legendary: trait(left) } as never),
  combatant({ id: 'v', label: 'Vampiro', legendary: trait(1) } as never),
];

const parry = (answerNow = true) =>
  reactionWindow({
    id: 'p',
    kind: ReactionKind.PARRY,
    reactorId: 'c',
    reactorLabel: 'Capitão bandido',
    groupId: 'g1',
    answerNow,
    trigger: { actorId: 't', actorLabel: 'Tor', attackTotal: 16, targetArmorClass: 15 } as never,
    prompt: {
      case: 'parry',
      value: {
        attackerLabel: 'Tor',
        attackNamePt: 'Espada longa',
        acBonus: 2,
        armorClassWithParry: 17,
      },
    },
  } as never);

const offer = (id: string, label: string, total: number, left: number) => ({
  combatantId: id,
  label,
  d20: 4,
  total,
  dc: 15,
  usesLeft: left,
  usesPerDay: 3,
});

const resistance = (answerNow = true) =>
  reactionWindow({
    id: 'lr',
    kind: ReactionKind.LEGENDARY_RESISTANCE,
    groupId: 'g2',
    answerNow,
    trigger: { actorId: 'pen', actorLabel: 'Pensantus', actionNamePt: 'Relâmpago' } as never,
    prompt: {
      case: 'legendaryResistance',
      value: {
        offers: [offer('d', 'Dragão vermelho adulto', 10, 3), offer('v', 'Vampiro', 11, 1)],
      },
    },
  } as never);

const dragonSave = (kind: ReactionKind) =>
  reactionWindow({
    id: 'sv',
    kind,
    groupId: 'g3',
    reactorId: 'd',
    reactorLabel: 'Dragão vermelho adulto',
    trigger: { damageTaken: 9, concentrationSpellKey: 'spell:fly', slotEffects: [] } as never,
    prompt:
      kind === ReactionKind.EFFECT_SAVE
        ? ({
            case: 'effectSave',
            value: {
              sourceNamePt: 'Teia',
              abilityNamePt: 'Destreza',
              dc: 15,
              modifier: 6,
              bonusKnown: true,
              mode: '',
              textPt: '',
              skippable: false,
              handedToMaster: false,
            },
          } as never)
        : ({
            case: 'concentrationSave',
            value: {
              spellNamePt: 'Voo',
              damageTaken: 9,
              dc: 10,
              saveBonus: 0,
              bonusKnown: true,
              mode: '',
              modeSourcesPt: [],
            },
          } as never),
  } as never);

describe("the master's offers in the queue (#557)", () => {
  const api = {
    answerReaction: vi.fn(),
    answerParry: vi.fn(),
    answerLegendaryResistance: vi.fn(),
    resolveConcentrationSave: vi.fn(),
  };
  const effectsApi = { rollEffectSave: vi.fn() };
  const done = async () => ({ encounter: encounter({ revision: 9 } as never), result: undefined });

  function setup(windows: ReturnType<typeof reactionWindow>[], left = 3) {
    for (const fn of Object.values(api)) {
      fn.mockReset().mockImplementation(done);
    }
    effectsApi.rollEffectSave.mockReset().mockImplementation(done);
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: EffectsClient, useValue: effectsApi },
        { provide: RollAnimator, useValue: { play: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(ReactionQueue);
    const e = encounter({
      id: 'enc',
      combatants: combatants(left),
      reactionWindows: windows,
    } as never);
    fixture.componentRef.setInput('encounter', e);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const texts = (el: HTMLElement, sel: string) =>
    Array.from(el.querySelectorAll(sel)).map((n) => plain(n.textContent));

  describe('the header and the badges', () => {
    it('counts the windows that block, with "uma de cada vez", "Agora" first and "Depois" after', () => {
      const { el } = setup([parry(), resistance(false)]);
      const head = el.querySelector('[data-testid="queue-header"]');
      const h2 = head?.querySelector('h2');
      expect(plain(h2?.textContent)).toBe('Esperando você 2 (2 janelas)');
      expect(h2?.querySelector('.qhead__count')?.getAttribute('aria-hidden')).toBe('true');
      expect(plain(h2?.querySelector('.mr-visually-hidden')?.textContent)).toBe('(2 janelas)');
      expect(plain(head?.querySelector('.qhead__hint')?.textContent)).toBe('uma de cada vez');
      expect(texts(el, '[data-testid="queue-badge"]')).toEqual(['Agora', 'Depois']);
      expect(
        plain(el.querySelector('[data-testid="resistance-offer"] .offer__wait')?.textContent),
      ).toContain('Depois da janela acima.');
    });

    it('says "trava a mesa" for a single window', () => {
      const { el } = setup([parry()]);
      expect(plain(el.querySelector('.qhead__hint')?.textContent)).toBe('trava a mesa');
      expect(plain(el.querySelector('h2')?.textContent)).toContain('(1 janela)');
    });

    it('draws no pill and no buttons for a card without a badge', () => {
      const { fixture } = setup([parry()]);
      const queued = fixture.debugElement.query(By.directive(ParryOffer));
      const own = TestBed.createComponent(ParryOffer);
      own.componentRef.setInput('card', queued.componentInstance.card());
      own.componentRef.setInput('encounter', encounter({ id: 'enc' } as never));
      own.componentRef.setInput('campaignId', 'camp');
      own.componentRef.setInput('state', new CombatState());
      own.componentRef.setInput('badge', null);
      own.detectChanges();
      const el = own.nativeElement as HTMLElement;
      expect(el.querySelector('[data-testid="queue-badge"]')).toBeNull();
      expect(el.querySelectorAll('button')).toHaveLength(0);
    });

    it('offers buttons only on the window to answer now', () => {
      const { el } = setup([parry(), resistance(false)]);
      expect(el.querySelectorAll('[data-testid="parry-offer"] button')).toHaveLength(2);
      expect(el.querySelectorAll('[data-testid="resistance-offer"] button')).toHaveLength(0);
      expect(el.querySelectorAll('[data-testid="resistance-offer"] input')).toHaveLength(0);
    });

    it('draws no header when nothing waits for the master', () => {
      const { el } = setup([]);
      expect(el.querySelector('[data-testid="queue-header"]')).toBeNull();
    });
  });

  describe('Parry', () => {
    it('says the question and puts "Deixar passar" before "Aparar"', () => {
      const { el } = setup([parry()]);
      const card = el.querySelector('[data-testid="parry-offer"]');
      expect(plain(card?.querySelector('.offer__over')?.textContent)).toContain('Reação');
      expect(plain(card?.querySelector('.offer__who')?.textContent)).toBe('Capitão bandido');
      expect(plain(card?.querySelector('h3')?.textContent)).toBe('Aparar o golpe de Tor?');
      expect(texts(el, '[data-testid="parry-offer"] button')).toEqual([
        'Deixar passar',
        'Aparar (+2 CA)',
      ]);
    });

    it('sends "Aparar" as true and "Deixar passar" as false, with the window and the combat', async () => {
      const first = setup([parry()]);
      button(first.el, 'Aparar (+2 CA)').click();
      await settle(first.fixture);
      expect(api.answerParry).toHaveBeenCalledWith('camp', 'enc', 'p', true, expect.any(String));
      const second = setup([parry()]);
      button(second.el, 'Deixar passar').click();
      await settle(second.fixture);
      expect(api.answerParry).toHaveBeenCalledWith('camp', 'enc', 'p', false, expect.any(String));
    });

    it('shows the sending state: both buttons dashed and inert, "Aparando…"', async () => {
      const { fixture, el } = setup([parry()]);
      let release: (v: unknown) => void = () => undefined;
      api.answerParry.mockImplementationOnce(() => new Promise((r) => (release = r)));
      button(el, 'Aparar (+2 CA)').click();
      fixture.detectChanges();
      expect(texts(el, '[data-testid="parry-offer"] button')).toEqual([
        'Deixar passar',
        'Aparando…',
      ]);
      const buttons = Array.from(
        el.querySelectorAll<HTMLButtonElement>('[data-testid="parry-offer"] button'),
      );
      expect(buttons.every((b) => b.classList.contains('btn--off'))).toBe(true);
      buttons[0].click();
      expect(api.answerParry).toHaveBeenCalledTimes(1);
      release(await done());
      await settle(fixture);
    });

    it('announces the answer from the queue region, not from the card that goes away', async () => {
      const { fixture, el } = setup([parry()]);
      button(el, 'Aparar (+2 CA)').click();
      await settle(fixture);
      expect(el.querySelector('[data-testid="parry-offer"] [role="status"]')).toBeNull();
      expect(plain(el.querySelector(':scope > [role="status"]')?.textContent)).toBe(
        'Golpe aparado.',
      );
    });

    it('moves the focus to "Tentar de novo" and back to the first answer on a retry', async () => {
      const { fixture, el } = setup([parry()]);
      document.body.append(el);
      api.answerParry.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      button(el, 'Aparar (+2 CA)').click();
      await settle(fixture);
      expect(document.activeElement).toBe(button(el, 'Tentar de novo'));
      let release: (v: unknown) => void = () => undefined;
      api.answerParry.mockImplementationOnce(() => new Promise((r) => (release = r)));
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      expect(document.activeElement).toBe(button(el, 'Deixar passar'));
      release(await done());
      await settle(fixture);
      el.remove();
    });

    it('keeps the offer on a lost connection and retries with the same key', async () => {
      const { fixture, el } = setup([parry()]);
      api.answerParry.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      button(el, 'Aparar (+2 CA)').click();
      await settle(fixture);
      expect(plain(el.querySelector('[role="alert"] p')?.textContent)).toBe(
        'Sem conexão. A resposta não foi enviada; a oferta continua esperando você.',
      );
      expect(texts(el, '[data-testid="parry-offer"] button')).toEqual(['refreshTentar de novo']);
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      expect(api.answerParry).toHaveBeenCalledTimes(2);
      const [a, b] = api.answerParry.mock.calls;
      expect(b[3]).toBe(true);
      expect(b[4]).toBe(a[4]);
    });

    it('says why a refusal came and keeps both buttons', async () => {
      const { fixture, el } = setup([parry()]);
      api.answerParry.mockRejectedValueOnce(new ConnectError('x', Code.NotFound));
      button(el, 'Deixar passar').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
      expect(button(el, 'Tentar de novo')).toBeUndefined();
      expect(texts(el, '[data-testid="parry-offer"] button')).toHaveLength(2);
    });
  });

  describe('Legendary Resistance', () => {
    it('draws one row per failing creature, each an unchecked checkbox labelled by its row', () => {
      const { el } = setup([resistance()]);
      expect(texts(el, '.lr__failed')).toEqual([
        'Dragão vermelho adulto falhou: d20 4 + 6 = 10 contra CD 15',
        'Vampiro falhou: d20 4 + 7 = 11 contra CD 15',
      ]);
      const boxes = Array.from(
        el.querySelectorAll<HTMLInputElement>('label input[type="checkbox"]'),
      );
      expect(boxes).toHaveLength(2);
      expect(boxes.every((b) => !b.checked)).toBe(true);
      const row = boxes[0].closest('label');
      expect(plain(row?.querySelector('.lr__name')?.textContent)).toBe('Usar Resistência Lendária');
      expect(plain(row?.querySelector('.lr__line')?.textContent)).toBe(
        'vira sucesso · gasta 1 · restam 3 de 3 hoje',
      );
      expect(plain(el.querySelector('h3')?.textContent)).toBe('Transformar falhas em sucesso?');
    });

    it('puts "Deixar como rolou" first and keeps "Usar nas marcadas" inert with nothing ticked', async () => {
      const { fixture, el } = setup([resistance()]);
      expect(texts(el, '[data-testid="resistance-offer"] button')).toEqual([
        'Deixar como rolou',
        'Usar nas marcadas',
      ]);
      button(el, 'Usar nas marcadas').click();
      await settle(fixture);
      expect(api.answerLegendaryResistance).not.toHaveBeenCalled();
    });

    it('sends exactly the ticked creature', async () => {
      const { fixture, el } = setup([resistance()]);
      const boxes = el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
      boxes[1].click();
      fixture.detectChanges();
      button(el, 'Usar nas marcadas').click();
      await settle(fixture);
      expect(api.answerLegendaryResistance).toHaveBeenCalledWith(
        'camp',
        'enc',
        'lr',
        { creatureIds: ['v'] },
        expect.any(String),
      );
    });

    it('sends no creature with "Deixar como rolou", even with one ticked', async () => {
      const { fixture, el } = setup([resistance()]);
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      fixture.detectChanges();
      button(el, 'Deixar como rolou').click();
      await settle(fixture);
      expect(api.answerLegendaryResistance).toHaveBeenCalledWith(
        'camp',
        'enc',
        'lr',
        { creatureIds: [] },
        expect.any(String),
      );
    });

    it('has no checkbox for a single creature: the primary uses it', async () => {
      const w = reactionWindow({
        ...resistance(),
        prompt: {
          case: 'legendaryResistance',
          value: { offers: [offer('d', 'Dragão vermelho adulto', 10, 3)] },
        },
      } as never);
      const { fixture, el } = setup([w]);
      expect(el.querySelectorAll('input[type="checkbox"]')).toHaveLength(0);
      expect(plain(el.querySelector('.lr__line')?.textContent)).toBe(
        'Resistência Lendária: restam 3 de 3 hoje · usar gasta 1',
      );
      expect(texts(el, '[data-testid="resistance-offer"] button')).toEqual([
        'Deixar como rolou',
        'Usar Resistência Lendária',
      ]);
      button(el, 'Usar Resistência Lendária').click();
      await settle(fixture);
      expect(api.answerLegendaryResistance).toHaveBeenCalledWith(
        'camp',
        'enc',
        'lr',
        { creatureIds: ['d'] },
        expect.any(String),
      );
    });

    it('names each checkbox by its creature, inside a group labelled by the title', () => {
      const { el } = setup([resistance()]);
      const group = el.querySelector('[role="group"]');
      expect(group?.getAttribute('aria-labelledby')).toBe(el.querySelector('h3')?.id);
      const box = el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[1];
      const name = el.querySelector(`#${box.getAttribute('aria-labelledby')}`);
      expect(plain(name?.textContent)).toBe('Usar Resistência Lendária em Vampiro');
    });

    it('locks the rows while "Tentar de novo" waits, so the retry resends the same answer', async () => {
      const { fixture, el } = setup([resistance()]);
      api.answerLegendaryResistance.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      fixture.detectChanges();
      button(el, 'Usar nas marcadas').click();
      await settle(fixture);
      const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'));
      expect(boxes.every((b) => b.disabled)).toBe(true);
    });

    it('shows the sending state and locks the rows', async () => {
      const { fixture, el } = setup([resistance()]);
      let release: (v: unknown) => void = () => undefined;
      api.answerLegendaryResistance.mockImplementationOnce(() => new Promise((r) => (release = r)));
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      fixture.detectChanges();
      button(el, 'Usar nas marcadas').click();
      fixture.detectChanges();
      expect(texts(el, '[data-testid="resistance-offer"] button')).toEqual([
        'Deixar como rolou',
        'Usando…',
      ]);
      expect(el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.disabled).toBe(true);
      release(await done());
      await settle(fixture);
    });

    it('retries a lost answer with the same creatures and the same key', async () => {
      const { fixture, el } = setup([resistance()]);
      api.answerLegendaryResistance.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      fixture.detectChanges();
      button(el, 'Usar nas marcadas').click();
      await settle(fixture);
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain('Sem conexão.');
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      const [a, b] = api.answerLegendaryResistance.mock.calls;
      expect(b[3]).toEqual({ creatureIds: ['d'] });
      expect(b[4]).toBe(a[4]);
    });
  });

  describe('"Se falhar, usar Resistência Lendária"', () => {
    it('is in an NPC effect save, unticked, and sent when ticked', async () => {
      const { fixture, el } = setup([dragonSave(ReactionKind.EFFECT_SAVE)]);
      const box = el.querySelector<HTMLInputElement>('[data-testid="resistance-box"] input')!;
      expect(box.checked).toBe(false);
      const row = box.closest('label');
      expect(plain(row?.querySelector('.box__name')?.textContent)).toBe(
        'Se falhar, usar Resistência Lendária',
      );
      expect(plain(row?.querySelector('.box__line')?.textContent)).toContain(
        'gasta 1 · restam 3 de 3 hoje',
      );
      box.click();
      fixture.detectChanges();
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3]).toMatchObject({
        kind: 'app',
        useLegendaryResistance: true,
      });
    });

    it('sends nothing extra when left unticked', async () => {
      const { fixture, el } = setup([dragonSave(ReactionKind.EFFECT_SAVE)]);
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3].useLegendaryResistance).toBeUndefined();
    });

    it('is in an NPC concentration save and rides the answer', async () => {
      const { fixture, el } = setup([dragonSave(ReactionKind.CONCENTRATION_SAVE)]);
      el.querySelector<HTMLInputElement>('[data-testid="resistance-box"] input')!.click();
      fixture.detectChanges();
      button(el, 'Rolar o teste').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toMatchObject({
        kind: 'app',
        useLegendaryResistance: true,
      });
    });

    it('is not drawn when no use is left', () => {
      const { el } = setup([dragonSave(ReactionKind.EFFECT_SAVE)], 0);
      expect(el.querySelector('[data-testid="resistance-box"]')).toBeNull();
    });

    it('is not drawn for a creature without the trait', () => {
      const w = reactionWindow({
        ...dragonSave(ReactionKind.EFFECT_SAVE),
        reactorId: 'c',
        reactorLabel: 'Capitão bandido',
      } as never);
      const { el } = setup([w]);
      expect(el.querySelector('[data-testid="resistance-box"]')).toBeNull();
    });
  });
});
