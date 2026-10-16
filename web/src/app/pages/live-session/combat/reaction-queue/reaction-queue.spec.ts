import { TestBed } from '@angular/core/testing';

import { CombatantKind, ReactionKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { EffectsClient } from '../../../../core/effects/effects-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { ReactionQueue } from './reaction-queue';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const combatants = [
  combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER }),
  combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER }),
  combatant({ id: 'nael', label: 'Nael', kind: CombatantKind.PLAYER }),
  combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
  combatant({ id: 'm1', label: 'Mago 1' }),
  combatant({ id: 'm2', label: 'Mago 2' }),
  combatant({ id: 'hob', label: 'Hobgoblin' }),
];

const spell = {
  actorId: 'm1',
  actorLabel: 'Mago 1',
  actionNamePt: 'Bola de Fogo',
  distanceFt: 30,
  slotEffects: [],
};

const counter = (id: string, reactor: string, label: string, over: object = {}) =>
  reactionWindow({
    id,
    kind: ReactionKind.COUNTERSPELL,
    groupId: 'spell-1',
    reactorId: reactor,
    reactorLabel: label,
    reactorIsPlayer: true,
    trigger: spell as never,
    ...over,
  } as never);

describe("ReactionQueue, the master's side of the reaction windows", () => {
  const api = { answerReaction: vi.fn(), resolveConcentrationSave: vi.fn() };
  const effectsApi = { rollEffectSave: vi.fn() };

  function setup(windows: ReturnType<typeof reactionWindow>[]) {
    api.answerReaction.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: { used: true, result: { case: 'shield', value: { stopped: true } } },
    }));
    api.resolveConcentrationSave.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    effectsApi.rollEffectSave.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: EffectsClient, useValue: effectsApi },
      ],
    });
    const fixture = TestBed.createComponent(ReactionQueue);
    const state = new CombatState();
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants, reactionWindows: windows } as never),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, state };
  }
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const flush = async (fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
    await fixture.whenStable();
    fixture.detectChanges();
  };

  describe('two queues, in the order the server gives', () => {
    const windows = () => [
      counter('a', 'pen', 'Pensantus', { answerNow: true }),
      counter('b', 'nael', 'Nael', { answerNow: false }),
      reactionWindow({
        id: 'u',
        kind: ReactionKind.UNCANNY_DODGE,
        groupId: 'attack-1',
        reactorId: 'brisa',
        reactorLabel: 'Brisa',
        reactorIsPlayer: true,
        trigger: {
          actorId: 'hob',
          actorLabel: 'Hobgoblin',
          targetLabel: 'Brisa',
          slotEffects: [],
        } as never,
      }),
    ];

    it('draws a card for the spell and one for the attack, with the tags and the descriptions', () => {
      const { el } = setup(windows());
      const titles = Array.from(el.querySelectorAll('.card__title')).map((h) => h.textContent);
      expect(titles).toEqual(['Reações a uma magia', 'Reações a um ataque']);
      const rows = Array.from(el.querySelectorAll('.row'));
      expect(rows).toHaveLength(3);
      expect(plain(rows[0].querySelector('.row__name b')?.textContent)).toBe('Pensantus');
      expect(plain(rows[0].querySelector('.tag')?.textContent)).toBe('Jogador');
      expect(plain(rows[0].textContent)).toContain('Contramágica · espaço de 3º nível ou maior');
      expect(plain(rows[0].textContent)).toContain('Respondendo no celular · vez de responder');
      expect(plain(rows[1].textContent)).toContain('Depois de Pensantus');
    });

    it('the one to answer has live buttons, the others are grey and dashed with the reason beside them', () => {
      const { el } = setup(windows());
      const rows = Array.from(el.querySelectorAll('.row'));
      const first = Array.from(rows[0].querySelectorAll('button'));
      const second = Array.from(rows[1].querySelectorAll('button'));
      expect(first.map((b) => plain(b.textContent))).toEqual([
        'Usar pelo jogador',
        'Deixar passar pelo jogador',
      ]);
      expect(first.every((b) => b.getAttribute('aria-disabled') !== 'true')).toBe(true);
      expect(second.every((b) => b.getAttribute('aria-disabled') === 'true')).toBe(true);
      expect(second.every((b) => b.classList.contains('btn--off'))).toBe(true);
      // Still reachable, and the reason is read with them.
      expect(second.every((b) => b.tabIndex >= 0)).toBe(true);
      const why = el.querySelector(`#${second[0].getAttribute('aria-describedby')}`);
      expect(plain(why?.textContent)).toBe('Depois de Pensantus');
    });

    it('answers for the player with the window id and a key, and says so in a status line', async () => {
      const { fixture, el } = setup(windows());
      button(el, 'Usar pelo jogador').click();
      await flush(fixture);
      const [campaign, enc, id, answer, key] = api.answerReaction.mock.calls[0];
      expect([campaign, enc, id, answer]).toEqual(['camp', 'enc', 'a', { use: true }]);
      expect(key).toEqual(expect.any(String));
      expect(plain(el.querySelector('[role="status"]')?.textContent)).toContain(
        'Escudo Arcano usado',
      );
    });

    it('lets the pass for the player go with "Deixar passar pelo jogador"', async () => {
      const { fixture, el } = setup(windows());
      button(el, 'Deixar passar pelo jogador').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({ use: false });
    });

    it('opens the focus on the first button of the first window to answer', () => {
      const { el } = setup(windows());
      return new Promise<void>((resolve) =>
        setTimeout(() => {
          expect(document.activeElement).toBe(button(el, 'Usar pelo jogador'));
          resolve();
        }),
      );
    });
  });

  describe('an NPC alone', () => {
    const shield = () =>
      reactionWindow({
        id: 's',
        kind: ReactionKind.SHIELD,
        reactorId: 'm1',
        reactorLabel: 'Mago 1',
        prompt: {
          case: 'shield',
          value: {
            slots: [
              { level: 2, pact: false, free: 1 },
              { level: 1, pact: false, free: 2 },
              { level: 3, pact: false, free: 0 },
            ],
          },
        } as never,
        trigger: {
          actorId: 't',
          actorLabel: 'Toren',
          attackTotal: 15,
          armorClassWithShield: 17,
          slotEffects: [],
        } as never,
      });

    it("is a card with the numbers that are the master's and two buttons of one size", () => {
      const { el } = setup([shield()]);
      expect(plain(el.querySelector('.warn')?.textContent)).toContain(
        'Esperando a sua reação: o Mago 1 pode conjurar Escudo Arcano (+5 na CA: 17 contra 15 viraria erro).',
      );
      expect(plain(button(el, 'Usar Escudo Arcano').textContent)).toBe(
        'Usar Escudo Arcano pelo Mago 1',
      );
      expect(plain(button(el, 'Deixar passar').textContent)).toBe('Deixar passar');
    });

    it('casts Escudo with the lowest free slot', async () => {
      const { fixture, el } = setup([shield()]);
      button(el, 'Usar Escudo Arcano').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({
        use: true,
        slot: { level: 1, pact: false },
      });
    });

    it("lets the master pick the slot of an NPC's Counterspell, with what each does", async () => {
      const w = counter('c', 'm1', 'Mago 1', {
        reactorIsPlayer: false,
        trigger: {
          actorId: 'pen',
          actorLabel: 'Pensantus',
          actionNamePt: 'Bola de Fogo',
          spellKey: 'spell:fireball',
          spellLevel: 3,
          slotEffects: [
            { slot: { level: 3, pact: false, free: 2 }, noCheck: true, checkDc: 0, checkBonus: 0 },
            {
              slot: { level: 4, pact: false, free: 1 },
              noCheck: false,
              checkDc: 13,
              checkBonus: 3,
            },
          ],
        },
      });
      const { fixture, el } = setup([w]);
      const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
      expect(plain(el.querySelector('.slots')?.textContent)).toContain(
        '3º nível · anula sem teste',
      );
      expect(plain(el.querySelector('.slots')?.textContent)).toContain(
        '4º nível · teste de conjuração CD 13',
      );
      expect(radios[0].checked).toBe(true);
      radios[1].click();
      fixture.detectChanges();
      button(el, 'Usar Contramágica').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({
        use: true,
        slot: { level: 4, pact: false },
      });
    });
  });

  describe('the "Sem reação" check of the rule "Sempre"', () => {
    const check = () =>
      reactionWindow({
        id: 'check',
        kind: ReactionKind.MASTER_CHECK,
        prompt: {
          case: 'masterCheck',
          value: { summaryPt: 'Ataque de Toren contra o Goblin 2 · acertou', enemyCanReact: false },
        } as never,
      });

    it('opens the focus on "Sem reação", the primary answer', () => {
      const { el } = setup([check()]);
      expect(plain(el.textContent)).toContain('Nenhum inimigo tem uma reação para isto.');
      expect(plain(el.textContent)).toContain('os jogadores veem só “Esperando o mestre”');
      return new Promise<void>((resolve) =>
        setTimeout(() => {
          expect(document.activeElement).toBe(button(el, 'Sem reação'));
          resolve();
        }),
      );
    });

    it('"Sem reação" closes the window (an enemy that can react has a window of its own)', async () => {
      const { fixture, el } = setup([check()]);
      button(el, 'Sem reação').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({ use: false });
      expect(
        Array.from(el.querySelectorAll('button')).some((b) =>
          b.textContent?.includes('Usar uma reação'),
        ),
      ).toBe(false);
    });
  });

  describe("Repreensão Infernal, the aggressor's saving throw", () => {
    const save = () =>
      reactionWindow({
        id: 'save',
        kind: ReactionKind.HELLISH_REBUKE,
        secondStep: true,
        reactorLabel: 'Mirta',
        reactorIsPlayer: true,
        prompt: {
          case: 'hellishRebukeSave',
          value: {
            aggressorLabel: 'Hobgoblin',
            saveDc: 13,
            saveBonus: 1,
            bonusKnown: true,
            diceCount: 3,
          },
        } as never,
      });

    it('is the master\'s: "Rolar o teste do Hobgoblin" or "Digitar o resultado"', async () => {
      const { fixture, el } = setup([save()]);
      expect(plain(el.querySelector('.card__title')?.textContent)).toBe(
        'Repreensão Infernal de Mirta',
      );
      expect(plain(el.textContent)).toContain(
        'Hobgoblin sofre 3d10 de fogo, ou metade se passar no teste de Destreza contra CD 13.',
      );
      expect(plain(el.textContent)).toContain('Destreza do Hobgoblin: +1 · o teste é seu');
      expect(plain(el.textContent)).toContain('O teste do agressor é do mestre.');
      button(el, 'Rolar o teste do Hobgoblin').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({ use: true, die: { inApp: true } });
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '6';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[1][3]).toEqual({ use: true, die: { typed: 6 } });
    });
  });

  describe('the concentration save', () => {
    const npc = () =>
      reactionWindow({
        id: 'conc',
        kind: ReactionKind.CONCENTRATION_SAVE,
        reactorId: 'm2',
        reactorLabel: 'Mago 2',
        trigger: { damageTaken: 9, concentrationSpellKey: 'spell:fly', slotEffects: [] } as never,
        prompt: {
          case: 'concentrationSave',
          value: { spellNamePt: 'Voo', damageTaken: 9, dc: 10, saveBonus: 0, bonusKnown: true },
        } as never,
      });
    const player = () =>
      reactionWindow({
        id: 'conc2',
        kind: ReactionKind.CONCENTRATION_SAVE,
        reactorId: 'brisa',
        reactorLabel: 'Sálvia',
        reactorIsPlayer: true,
        trigger: {
          damageTaken: 28,
          concentrationSpellKey: 'spell:entangle',
          slotEffects: [],
        } as never,
        prompt: {
          case: 'concentrationSave',
          value: {
            spellNamePt: 'Constrição',
            damageTaken: 28,
            dc: 14,
            saveBonus: 2,
            bonusKnown: true,
          },
        } as never,
      });

    it("an NPC's is the master's: roll, type, or keep", async () => {
      const { fixture, el } = setup([npc()]);
      expect(plain(el.querySelector('.card__title')?.textContent)).toBe('Mago 2 sofreu 9 de dano');
      expect(plain(el.textContent)).toContain('Mago 2 concentra em Voo.');
      expect(plain(el.textContent)).toContain('contra CD 10 (metade de 9 seria 4; o mínimo é 10)');
      button(el, 'Rolar o teste do Mago 2').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0].slice(0, 4)).toEqual([
        'camp',
        'enc',
        'conc',
        { kind: 'app' },
      ]);
      button(el, 'Manter a concentração').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[1][3]).toEqual({ kind: 'keep' });
    });

    it('a player who does not answer: "Rolar o teste por Sálvia" and "Manter a concentração por Sálvia", never a pronoun', async () => {
      const { fixture, el } = setup([player()]);
      expect(plain(el.textContent)).toContain(
        'Esperando o teste de Constituição de Sálvia. O jogador está respondendo no celular.',
      );
      expect(plain(el.textContent)).not.toMatch(/ ela | ele |por ela|por ele/);
      button(el, 'Rolar o teste por Sálvia').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({ kind: 'app' });
      button(el, 'Manter a concentração por Sálvia').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[1][3]).toEqual({ kind: 'keep' });
    });
  });

  describe("an effect's saving throw", () => {
    const goblin = () =>
      reactionWindow({
        id: 'es',
        kind: ReactionKind.EFFECT_SAVE,
        reactorId: 'm1',
        reactorLabel: 'Goblin 1',
        prompt: {
          case: 'effectSave',
          value: {
            sourceNamePt: 'Riso Histérico',
            abilityNamePt: 'Sabedoria',
            dc: 13,
            modifier: -1,
            bonusKnown: true,
            mode: '',
            textPt: '',
            skippable: true,
            handedToMaster: false,
          },
        } as never,
      });

    it('says what is asked and rolls it in the app once, with the window id', async () => {
      const { fixture, el } = setup([goblin()]);
      expect(plain(el.querySelector('.card__title')?.textContent)).toBe(
        'Teste de resistência de Sabedoria do Goblin 1 · Riso Histérico · CD 13',
      );
      expect(el.textContent).not.toContain('Esperando a sua reação');
      button(el, 'Rolar no app').click();
      button(el, 'Rolar no app').click();
      await flush(fixture);
      expect(effectsApi.rollEffectSave).toHaveBeenCalledTimes(1);
      expect(effectsApi.rollEffectSave.mock.calls[0].slice(0, 4)).toEqual([
        'camp',
        'enc',
        'es',
        { kind: 'app' },
      ]);
      expect(api.answerReaction).not.toHaveBeenCalled();
    });

    it('sends the typed d20', async () => {
      const { fixture, el } = setup([goblin()]);
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '14';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3]).toEqual({
        kind: 'typed',
        face: 14,
        extra: [],
      });
    });

    it('with advantage, types both d20 and sends the two faces', async () => {
      const w = goblin();
      (w.prompt.value as { mode: string }).mode = 'advantage';
      const { fixture, el } = setup([w]);
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      // The hint of the typed form says which die counts.
      expect(el.textContent).toContain('Com vantagem conta o maior');
      const fields = el.querySelectorAll<HTMLInputElement>('input[type="text"]');
      expect(fields.length).toBe(2);
      [15, 9].forEach((n, i) => {
        fields[i].value = String(n);
        fields[i].dispatchEvent(new Event('input'));
      });
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3]).toEqual({
        kind: 'typed',
        face: 15,
        second: 9,
        extra: [],
      });
    });

    it('skipping the save asks to confirm first', async () => {
      const { fixture, el } = setup([goblin()]);
      button(el, 'Pular o teste').click();
      fixture.detectChanges();
      expect(effectsApi.rollEffectSave).not.toHaveBeenCalled();
      button(el, 'Sim, pular o teste').click();
      await flush(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3]).toEqual({ kind: 'skip' });
    });
  });

  it('shows the refusal in an alert and keeps the cards', async () => {
    const { fixture, el } = setup([counter('a', 'pen', 'Pensantus')]);
    api.answerReaction.mockRejectedValue(new Error('boom'));
    button(el, 'Usar pelo jogador').click();
    await flush(fixture);
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    expect(el.querySelector('.row')).not.toBeNull();
  });

  it('reads "Rolando…" with the longer wait on the card of a window its player is rolling', () => {
    const { fixture, el } = setup([counter('a', 'pen', 'Pensantus', { rolling: true })]);
    fixture.componentRef.setInput('autoLeft', { a: 112 });
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Rolando… passa sozinho em 112 s');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants, reactionWindows: [counter('a', 'pen', 'Pensantus')] } as never),
    );
    fixture.componentRef.setInput('autoLeft', { a: 12 });
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Passa sozinho em 12 s');
    expect(plain(el.textContent)).not.toContain('Rolando…');
  });

  it('draws nothing when no window is open', () => {
    const { el } = setup([]);
    expect(el.querySelector('.card')).toBeNull();
  });
});
