import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { CombatantKind, ReactionKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { EffectsClient } from '../../../../core/effects/effects-client';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { settle } from '../../../../../test-settle';
import { ReactionQueue } from './reaction-queue';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const combatants = [
  combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
  combatant({ id: 'm1', label: 'Mago 1' }),
  combatant({ id: 'm2', label: 'Mago 2' }),
];

/** A d20 the app rolled, in the shape the overlay can draw. */
const rolled = {
  diceSides: 20,
  faces: [14],
  countedIndex: 0,
  physical: false,
  total: 16,
  modifier: 2,
  diceCount: 1,
  extraDice: [],
};

const concentrationWindow = (id: string, over: object, value: object) =>
  reactionWindow({
    id,
    kind: ReactionKind.CONCENTRATION_SAVE,
    reactorId: 'm2',
    reactorLabel: 'Mago 2',
    trigger: { damageTaken: 9, concentrationSpellKey: 'spell:fly', slotEffects: [] } as never,
    prompt: {
      case: 'concentrationSave',
      value: {
        spellNamePt: 'Voo',
        damageTaken: 9,
        dc: 10,
        saveBonus: 0,
        bonusKnown: true,
        mode: '',
        modeSourcesPt: [],
        ...value,
      },
    } as never,
    ...over,
  } as never);

const effectSaveWindow = (value: object, over: object = {}) =>
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
        skippable: false,
        handedToMaster: false,
        ...value,
      },
    } as never,
    ...over,
  } as never);

describe('ReactionQueue, the answers and what each card says', () => {
  const api = { answerReaction: vi.fn(), resolveConcentrationSave: vi.fn() };
  const effectsApi = { rollEffectSave: vi.fn() };
  const animator = { play: vi.fn() };

  function setup(windows: ReturnType<typeof reactionWindow>[]) {
    api.answerReaction.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    api.resolveConcentrationSave.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    effectsApi.rollEffectSave.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    animator.play.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: EffectsClient, useValue: effectsApi },
        { provide: RollAnimator, useValue: animator },
      ],
    });
    const fixture = TestBed.createComponent(ReactionQueue);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants, reactionWindows: windows } as never),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const type = (el: HTMLElement, faces: number[]) => {
    const fields = el.querySelectorAll<HTMLInputElement>('input[type="text"]');
    faces.forEach((n, i) => {
      fields[i].value = String(n);
      fields[i].dispatchEvent(new Event('input'));
    });
  };
  const typeLast = (el: HTMLElement, face: number) => {
    const fields = el.querySelectorAll<HTMLInputElement>('input[type="text"]');
    const last = fields[fields.length - 1];
    last.value = String(face);
    last.dispatchEvent(new Event('input'));
  };
  /** What the last answer said, in the live region. */
  const status = (el: HTMLElement) =>
    plain(el.querySelector('.mr-visually-hidden[role="status"]')?.textContent);

  describe('a reaction answered', () => {
    it('says the shield did not stop the attack', async () => {
      const w = reactionWindow({ id: 'a', reactorId: 'm1', reactorLabel: 'Mago 1' });
      const { fixture, el } = setup([w]);
      api.answerReaction.mockResolvedValue({
        encounter: encounter({ revision: 9 } as never),
        result: { result: { case: 'shield', value: { stopped: false } } },
      });
      button(el, 'Usar').click();
      await settle(fixture);
      expect(status(el)).toContain('Escudo Arcano usado: o ataque ainda acerta.');
    });

    it('says only "Reação usada." for any other result, and "Deixou passar" for a pass', async () => {
      const w = reactionWindow({ id: 'a', reactorId: 'm1', reactorLabel: 'Mago 1' });
      const { fixture, el } = setup([w]);
      button(el, 'Usar').click();
      await settle(fixture);
      expect(status(el)).toContain('Reação usada.');
      button(el, 'Deixar passar').click();
      await settle(fixture);
      expect(status(el)).toContain('Deixou passar do Mago 1.');
    });

    it('answers once while an answer is in the air', async () => {
      const { fixture, el } = setup([reactionWindow({ id: 'a', reactorLabel: 'Mago 1' })]);
      button(el, 'Usar').click();
      button(el, 'Deixar passar').click();
      await settle(fixture);
      expect(api.answerReaction).toHaveBeenCalledTimes(1);
    });

    it('lets a failing animation pass: the answer is told all the same', async () => {
      const { fixture, el } = setup([reactionWindow({ id: 'a', reactorLabel: 'Mago 1' })]);
      animator.play.mockImplementation(() => {
        throw new Error('no canvas');
      });
      api.answerReaction.mockResolvedValue({
        encounter: encounter({ revision: 9 } as never),
        result: {
          result: { case: 'counterspell', value: { check: rolled, countered: true } },
        },
      });
      button(el, 'Usar').click();
      await settle(fixture);
      expect(animator.play).toHaveBeenCalledTimes(1);
      expect(el.querySelector('[role="alert"]')).toBeNull();
      expect(status(el)).toContain('Reação usada.');
    });
  });

  describe("Repreensão Infernal when the aggressor's bonus is unknown", () => {
    it('says so and rolls with no bonus', () => {
      const { el } = setup([
        reactionWindow({
          id: 'save',
          kind: ReactionKind.HELLISH_REBUKE,
          secondStep: true,
          reactorLabel: 'Mirta',
          reactorIsPlayer: false,
          prompt: {
            case: 'hellishRebukeSave',
            value: {
              aggressorLabel: 'Hobgoblin',
              saveDc: 12,
              saveBonus: 4,
              bonusKnown: false,
              diceCount: 2,
            },
          } as never,
        }),
      ]);
      expect(plain(el.querySelector('.card__title')?.textContent)).toBe(
        'Repreensão Infernal da Mirta',
      );
      expect(plain(el.textContent)).toContain(
        'Destreza do Hobgoblin: desconhecida · o teste é seu',
      );
    });
  });

  describe('the concentration save of an NPC', () => {
    it('with advantage asks for two d20 and says why', async () => {
      const w = concentrationWindow('c', {}, { mode: 'advantage', modeSourcesPt: ['Bênção'] });
      const { fixture, el } = setup([w]);
      expect(plain(el.textContent)).toContain('tem vantagem neste teste (Bênção)');
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      expect(el.querySelectorAll('input[type="text"]')).toHaveLength(2);
      type(el, [7, 15]);
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({
        kind: 'typed',
        face: 7,
        second: 15,
      });
    });

    it('with disadvantage keeps the lower of the two and writes the bonus with its sign', async () => {
      const w = concentrationWindow('c', {}, { mode: 'disadvantage', saveBonus: -2 });
      const { fixture, el } = setup([w]);
      expect(plain(el.textContent)).toContain('Rolar no app usa d20 − 2');
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      type(el, [3, 12]);
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toMatchObject({
        kind: 'typed',
        face: 3,
      });
    });

    it('sends the typed d20 of a normal save', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, { saveBonus: 3 })]);
      expect(plain(el.textContent)).toContain('Rolar no app usa d20 + 3');
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      type(el, [11]);
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({ kind: 'typed', face: 11 });
    });

    it('reads the save and says whether the concentration was kept', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, {})]);
      api.resolveConcentrationSave.mockResolvedValue({
        encounter: encounter({ revision: 9 } as never),
        result: { save: rolled, dc: 10, kept: false, spellNamePt: 'Voo' },
      });
      button(el, 'Rolar o teste').click();
      await settle(fixture);
      expect(status(el)).toContain('contra CD 10: Mago 2 perdeu a concentração em Voo.');
      expect(animator.play).toHaveBeenCalledTimes(1);
    });

    it('says "Respondido." when the answer carries no result and the master did not keep', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, {})]);
      button(el, 'Rolar o teste').click();
      await settle(fixture);
      expect(status(el)).toContain('Respondido.');
    });

    it('asks for the d4 of an effect when the server says the roll takes them, and holds the answer until typed', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, {})]);
      api.resolveConcentrationSave.mockRejectedValueOnce(
        new ConnectError('the roll takes 1 more die(s)', Code.InvalidArgument),
      );
      button(el, 'Rolar o teste').click();
      await settle(fixture);
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
        'Esta rolagem leva mais um d4',
      );
      expect(el.querySelector('app-extra-dice')).not.toBeNull();
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      typeLast(el, 11);
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave).toHaveBeenCalledTimes(1);
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
        'Digite o resultado do d4 antes de confirmar.',
      );
    });

    it('shows the refusal of any other kind', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, {})]);
      api.resolveConcentrationSave.mockRejectedValueOnce(new Error('boom'));
      button(el, 'Rolar o teste').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
      expect(el.querySelector('app-extra-dice')).toBeNull();
    });

    it('answers once while the answer is in the air', async () => {
      const { fixture, el } = setup([concentrationWindow('c', {}, {})]);
      button(el, 'Rolar o teste').click();
      button(el, 'Manter a concentração').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave).toHaveBeenCalledTimes(1);
    });
  });

  describe('the concentration save of a player', () => {
    it('keeps the concentration for them, and says whose', async () => {
      const w = concentrationWindow('c', { reactorLabel: 'Sálvia', reactorIsPlayer: true }, {});
      const { fixture, el } = setup([w]);
      button(el, 'Manter a concentração por Sálvia').click();
      await settle(fixture);
      expect(status(el)).toContain('Concentração mantida da Sálvia.');
    });
  });

  describe("an effect's saving throw", () => {
    it('is the same card for a player, who left it to the master', () => {
      const { el } = setup([
        effectSaveWindow(
          { phase: 1, handedToMaster: true },
          { reactorLabel: 'Brisa', reactorIsPlayer: true },
        ),
      ]);
      expect(plain(el.querySelector('.card__title')?.textContent)).toBe(
        'Teste de resistência de Sabedoria de Brisa · Riso Histérico · CD 13',
      );
      expect(plain(el.textContent)).toContain('O jogador de Brisa deixou a rolagem com você.');
      expect(plain(el.textContent)).toContain('Sabedoria: −1');
    });

    it('says the bonus is unknown and rolls with none', () => {
      const { el } = setup([effectSaveWindow({ bonusKnown: false, mode: 'advantage' })]);
      expect(plain(el.textContent)).toContain('Bônus desconhecido · advantage');
    });

    it('with disadvantage the hint says the lower die counts', () => {
      const { fixture, el } = setup([effectSaveWindow({ mode: 'disadvantage' })]);
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      expect(el.textContent).toContain('Com desvantagem conta o menor');
    });

    it('with advantage rolls in the app all the same and tells the save', async () => {
      const { fixture, el } = setup([effectSaveWindow({ mode: 'advantage' })]);
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(effectsApi.rollEffectSave.mock.calls[0][3]).toEqual({ kind: 'app' });
      expect(status(el)).toContain('Teste de resistência do Goblin 1 resolvido.');
    });

    it('says the save was skipped', async () => {
      const { fixture, el } = setup([effectSaveWindow({ skippable: true })]);
      button(el, 'Pular o teste').click();
      fixture.detectChanges();
      button(el, 'Não pular').click();
      fixture.detectChanges();
      expect(button(el, 'Pular o teste')).toBeDefined();
      button(el, 'Pular o teste').click();
      fixture.detectChanges();
      button(el, 'Sim, pular o teste').click();
      await settle(fixture);
      expect(status(el)).toContain('Teste pulado do Goblin 1.');
    });

    it('asks for the d4 of an effect, and refuses to send until they are typed', async () => {
      const { fixture, el } = setup([effectSaveWindow({})]);
      effectsApi.rollEffectSave.mockRejectedValueOnce(
        new ConnectError('the roll takes 2 more die(s)', Code.InvalidArgument),
      );
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
        'Esta rolagem leva mais 2 d4',
      );
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      typeLast(el, 9);
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(effectsApi.rollEffectSave).toHaveBeenCalledTimes(1);
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
        'Digite o resultado de cada dado extra antes de confirmar.',
      );
    });

    it('shows the refusal of any other kind', async () => {
      const { fixture, el } = setup([effectSaveWindow({})]);
      effectsApi.rollEffectSave.mockRejectedValueOnce(new Error('boom'));
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
    });

    it('answers once while the answer is in the air', async () => {
      const { fixture, el } = setup([effectSaveWindow({})]);
      button(el, 'Rolar no app').click();
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(effectsApi.rollEffectSave).toHaveBeenCalledTimes(1);
    });

    it('animates the save the app rolled', async () => {
      const { fixture, el } = setup([effectSaveWindow({})]);
      effectsApi.rollEffectSave.mockResolvedValue({
        encounter: encounter({ revision: 9 } as never),
        result: {
          autoFail: false,
          skipped: false,
          physical: false,
          d20: 14,
          d20Faces: [14],
          modifier: -1,
          total: 13,
          extraDice: [],
          saved: true,
        },
      });
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(animator.play).toHaveBeenCalledTimes(1);
    });
  });
});
