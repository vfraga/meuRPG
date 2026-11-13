import { create } from '@bufbuild/protobuf';

import { CombatantKind, CombatantSide } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ContestKind,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestTurnStateSchema,
  ContestWaitFor,
  ContestWinner,
  RollModeKind,
  RollNoteSchema,
  ShoveBlockedReason,
  ShoveOutcome,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { CreatureSize } from '../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from './combat-testing';
import { checkRoll, contestView, skillOption } from './contest-testing';
import {
  contestNotes,
  contestSteps,
  countedFace,
  defenderTitle,
  defenderVerdict,
  escapeIntro,
  escapeVerdict,
  holdLine,
  initiatorVerdict,
  named,
  noteLines,
  ofNamed,
  pairOf,
  proneLine,
  pushLine,
  refusalParts,
  rollFormula,
  sheetTitle,
  sheetTitleWith,
  signed,
  sizeWord,
  skillChoices,
  skillLine,
  skillName,
  waitDetail,
  waitTitle,
  whoIs,
} from './contest-view';

const plain = (s: string) => s.replace(/\u00a0/g, ' ');
const hob = whoIs(combatant({ id: 'h', label: 'Hobgoblin' }));
const brisa = whoIs(combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER }));

describe('the words of the contests (W7-X)', () => {
  it('names a player bare and a creature with its article', () => {
    expect(named(hob)).toBe('o Hobgoblin');
    expect(named(brisa)).toBe('Brisa');
    expect(ofNamed(hob)).toBe('do Hobgoblin');
    expect(ofNamed(brisa)).toBe('de Brisa');
    expect(ofNamed({ label: 'Cobra constritora', player: false })).toBe('da Cobra constritora');
  });

  it('titles the sheets as the boards do', () => {
    expect(sheetTitle(ContestPurpose.GRAPPLE)).toBe('Agarrar');
    expect(sheetTitle(ContestPurpose.SHOVE)).toBe('Empurrar');
    expect(sheetTitle(ContestPurpose.ESCAPE)).toBe('Escapar');
    expect(sheetTitleWith(ContestPurpose.GRAPPLE, 'o Hobgoblin')).toBe('Agarrar o Hobgoblin');
    expect(sheetTitleWith(ContestPurpose.GRAPPLE, 'Brisa')).toBe('Agarrar Brisa');
    expect(defenderTitle('Hobgoblin', ContestPurpose.GRAPPLE)).toBe('Hobgoblin tenta agarrar você');
    expect(defenderTitle('Toren', ContestPurpose.SHOVE)).toBe('Toren tenta empurrar você');
  });

  it('writes the skills with their ability, and a modifier with a true minus', () => {
    expect(skillLine(ContestSkill.ATHLETICS)).toBe('Força (Atletismo)');
    expect(skillLine(ContestSkill.ACROBATICS)).toBe('Destreza (Acrobacia)');
    expect(skillName(ContestSkill.ACROBATICS)).toBe('Acrobacia');
    expect(signed(7)).toBe('+7');
    expect(signed(0)).toBe('+0');
    expect(signed(-1)).toBe('−1');
  });

  it('puts the skill that rolls best first, as the board draws the choice', () => {
    const rows = skillChoices([
      skillOption({ skill: ContestSkill.ATHLETICS, modifier: 0 }),
      skillOption({ skill: ContestSkill.ACROBATICS, modifier: 7 }),
    ]);
    expect(rows.map((r) => r.skill)).toEqual([ContestSkill.ACROBATICS, ContestSkill.ATHLETICS]);
  });

  it('reads the size in words', () => {
    expect(sizeWord(CreatureSize.MEDIUM)).toBe('Médio');
    expect(sizeWord(CreatureSize.HUGE)).toBe('Enorme');
    expect(sizeWord(CreatureSize.UNSPECIFIED)).toBe('');
  });

  it('builds the stepper', () => {
    expect(contestSteps(['Alvo', 'Disputa', 'Resultado'], 1).map((s) => s.state)).toEqual([
      'done',
      'current',
      'todo',
    ]);
  });

  describe('a roll', () => {
    it('shows the d20 that counts and the formula, never more than the player rolled', () => {
      const roll = checkRoll();
      expect(countedFace(roll)).toBe(15);
      expect(rollFormula(roll)).toBe('1d20 (15) + 5');
      expect(rollFormula(checkRoll({ modifier: -1, faces: [9], total: 8 }))).toBe('1d20 (9) − 1');
      expect(rollFormula(checkRoll({ modifier: 0, faces: [9], total: 9 }))).toBe('1d20 (9)');
    });

    it('counts the higher d20 with advantage and the lower with disadvantage, and marks one of a pair', () => {
      const adv = checkRoll({ faces: [8, 15], mode: RollModeKind.ADVANTAGE, total: 20 });
      const dis = checkRoll({ faces: [8, 15], mode: RollModeKind.DISADVANTAGE, total: 13 });
      expect(countedFace(adv)).toBe(15);
      expect(countedFace(dis)).toBe(8);
      expect(pairOf(adv)).toEqual([
        { value: 8, counts: false },
        { value: 15, counts: true },
      ]);
      expect(pairOf(checkRoll())).toEqual([]);
      // Two equal dice: one counts, the other does not.
      expect(
        pairOf(checkRoll({ faces: [12, 12], mode: RollModeKind.ADVANTAGE })).map((d) => d.counts),
      ).toEqual([true, false]);
    });

    it('says the circumstances behind the mode, one sentence each', () => {
      const roll = checkRoll({
        notes: [
          create(RollNoteSchema, { kind: 'help', labelPt: 'Ajuda de Orla', advantage: true }),
          create(RollNoteSchema, { kind: 'poisoned', labelPt: 'Envenenado', advantage: false }),
        ],
      });
      expect(noteLines(roll)).toEqual(['Vantagem: Ajuda de Orla', 'Desvantagem: Envenenado']);
    });

    it('says a bonus an effect adds by its label alone, never as a disadvantage', () => {
      const roll = checkRoll({
        notes: [
          create(RollNoteSchema, {
            kind: 'effect_bonus',
            labelPt: 'Marca de Nanquim +3',
            advantage: false,
          }),
        ],
      });
      expect(noteLines(roll)).toEqual(['Marca de Nanquim +3']);
    });
  });

  describe('the wait', () => {
    it('says the combat’s own wait, and for a player the name', () => {
      const e = encounter({
        reactionWait: {
          titlePt: 'Esperando o mestre',
          detailPt: 'O turno continua quando ele responder.',
        },
        combatants: [combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER })],
      } as never);
      expect(waitTitle(e, contestView())).toBe('Esperando o mestre');
      const bare = encounter({
        combatants: [combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER })],
      });
      expect(
        waitTitle(
          bare,
          contestView({ waitingFor: ContestWaitFor.PLAYER, waitingCombatantId: 'b' }),
        ),
      ).toBe('Esperando Brisa');
      expect(waitTitle(bare, contestView())).toBe('Esperando o mestre');
    });

    it('says who answers and what they do', () => {
      const e = encounter({
        combatants: [
          combatant({ id: 'h', label: 'Hobgoblin' }),
          combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER }),
        ],
      });
      expect(waitDetail(e, contestView())).toBe(
        'O Hobgoblin escolhe Atletismo ou Acrobacia e rola.',
      );
      expect(waitDetail(e, contestView({ defenderId: 'b' }))).toBe(
        'Ela escolhe Atletismo ou Acrobacia e rola.',
      );
      expect(waitDetail(e, contestView({ defenderId: 'b', deferred: true }))).toBe(
        'Brisa deixou o mestre rolar por ela.',
      );
      expect(waitDetail(e, contestView({ status: ContestStatus.AWAITING_OUTCOME }))).toBe(
        'Você escolhe o que fazer.',
      );
      expect(waitDetail(null, undefined)).toBe('');
    });
  });

  describe('what the one that started reads once it is decided', () => {
    it('a won grapple says the target is Agarrado, by its gender, and how it is held', () => {
      const won = contestView({ status: ContestStatus.RESOLVED, winner: ContestWinner.INITIATOR });
      expect(initiatorVerdict(won, hob, 'Toren')).toEqual({
        lead: 'Você venceu a disputa.',
        rest: 'O Hobgoblin está Agarrado.',
        won: true,
      });
      expect(initiatorVerdict(won, brisa, 'Toren')?.rest).toBe('Brisa está Agarrada.');
      expect(holdLine('Hobgoblin')).toBe(
        'Você o segura enquanto quiser (solte sem gastar ação). Se você se mover, leva-o junto, com o deslocamento pela metade.',
      );
      expect(holdLine('Brisa')).toContain('Você a segura');
      expect(holdLine('Brisa')).toContain('leva-a junto');
    });

    it('a lost or tied grapple says only that it did not work, never a total', () => {
      const lost = contestView({ status: ContestStatus.RESOLVED, winner: ContestWinner.DEFENDER });
      expect(initiatorVerdict(lost, hob, 'Toren')).toEqual({
        lead: 'Você não conseguiu agarrar o Hobgoblin.',
        rest: '',
        won: false,
      });
      const tie = contestView({ status: ContestStatus.RESOLVED, winner: ContestWinner.TIE });
      expect(initiatorVerdict(tie, hob, 'Toren')?.lead).toBe('Empate: nada muda.');
      expect(initiatorVerdict(tie, hob, 'Toren')?.rest).toBe(
        'Você não conseguiu agarrar o Hobgoblin.',
      );
      const shove = contestView({
        purpose: ContestPurpose.SHOVE,
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.DEFENDER,
      });
      expect(initiatorVerdict(shove, hob, 'Toren')?.lead).toBe(
        'Você não conseguiu empurrar o Hobgoblin.',
      );
    });

    it('says nothing while the contest waits, and that the master closed one', () => {
      expect(initiatorVerdict(contestView(), hob, 'Toren')).toBeNull();
      expect(
        initiatorVerdict(contestView({ status: ContestStatus.CLOSED }), hob, 'Toren')?.lead,
      ).toBe('O mestre encerrou a disputa.');
    });

    it('a won shove says what was done', () => {
      const done = (shoveOutcome: ShoveOutcome) =>
        initiatorVerdict(
          contestView({
            purpose: ContestPurpose.SHOVE,
            status: ContestStatus.RESOLVED,
            winner: ContestWinner.INITIATOR,
            shoveOutcome,
          }),
          hob,
          'Toren',
        )?.rest;
      expect(done(ShoveOutcome.PRONE)).toBe('O Hobgoblin está Derrubado.');
      expect(plain(done(ShoveOutcome.PUSH) ?? '')).toBe('Você empurrou o Hobgoblin 1,5 m.');
      expect(done(ShoveOutcome.STAYS)).toBe('O Hobgoblin não saiu do lugar.');
    });

    it('an escape says "Você se soltou", "Você escapou" against a fixed DC, or "Continua agarrada"', () => {
      const won = contestView({
        purpose: ContestPurpose.ESCAPE,
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
      });
      expect(escapeVerdict(won).lead).toBe('Você se soltou.');
      expect(escapeVerdict({ ...won, kind: ContestKind.ESCAPE_DC }).lead).toBe('Você escapou.');
      const lost = { ...won, winner: ContestWinner.DEFENDER };
      expect(escapeVerdict(lost, 'Brisa')).toEqual({
        lead: 'Continua agarrada.',
        rest: 'A ação foi gasta.',
        won: false,
      });
      expect(escapeVerdict(lost, 'Toren').lead).toBe('Continua agarrado.');
    });

    it('the escape’s sentence names whom the test is against, with no number', () => {
      expect(escapeIntro(hob, false)).toBe(
        'Você usa a ação. Escolha Atletismo ou Acrobacia: o teste é contra o Atletismo do Hobgoblin.',
      );
      expect(escapeIntro({ label: 'Cobra constritora gigante', player: false }, true)).toBe(
        'Você usa a ação: um teste de Atletismo ou Acrobacia contra a força da Cobra constritora gigante.',
      );
    });
  });

  describe('what the target reads', () => {
    it('a lost grapple: the one that held, and the condition', () => {
      const lost = contestView({
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
      });
      expect(defenderVerdict(lost, hob, 'Brisa')).toEqual({
        lead: 'Você perdeu a disputa.',
        rest: 'O Hobgoblin agarrou você: você está Agarrada, com deslocamento 0.',
        won: false,
      });
    });

    it('a won defence says the grapple did not work', () => {
      const won = contestView({ status: ContestStatus.RESOLVED, winner: ContestWinner.DEFENDER });
      expect(defenderVerdict(won, brisa, 'Toren')).toEqual({
        lead: 'Você venceu a disputa.',
        rest: 'Brisa não conseguiu agarrar você.',
        won: true,
      });
      expect(defenderVerdict({ ...won, winner: ContestWinner.TIE }, hob, 'Brisa')?.lead).toBe(
        'Empate: nada muda.',
      );
    });

    it('a lost shove says what the winner did, and waits while they choose', () => {
      const shove = contestView({
        purpose: ContestPurpose.SHOVE,
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
        shoveOutcome: ShoveOutcome.PRONE,
      });
      expect(defenderVerdict(shove, hob, 'Brisa')?.rest).toBe('O Hobgoblin derrubou você.');
      expect(
        defenderVerdict({ ...shove, shoveOutcome: ShoveOutcome.STAYS }, hob, 'Brisa')?.rest,
      ).toBe('O Hobgoblin não tirou você do lugar.');
      expect(
        defenderVerdict(contestView({ status: ContestStatus.AWAITING_OUTCOME }), hob, 'Brisa')
          ?.lead,
      ).toBe('Você perdeu a disputa.');
      expect(defenderVerdict(contestView(), hob, 'Brisa')).toBeNull();
    });
  });

  describe('the shove’s choice', () => {
    it('says what each outcome does, and why a push is blocked', () => {
      expect(plain(proneLine(hob))).toBe(
        'O Hobgoblin fica Derrubado: só rasteja. Ataque corpo a corpo contra ele a até 1,5 m tem vantagem; de mais longe, desvantagem.',
      );
      expect(pushLine(ShoveBlockedReason.UNSPECIFIED)).toBe('Para longe de você, em linha reta.');
      expect(pushLine(ShoveBlockedReason.WALL)).toBe(
        'Para longe de você. Há uma parede na casa de trás: ele não sai do lugar.',
      );
      expect(pushLine(ShoveBlockedReason.CREATURE)).toContain('uma criatura');
    });
  });

  describe('the states of the turn', () => {
    it('says "Escondida" without saying from whom, and "Surpresa"', () => {
      const hidden = create(ContestTurnStateSchema, { hidden: true });
      expect(contestNotes(hidden, 'Brisa')).toEqual([
        { tag: 'Escondida', icon: 'visibility', tone: 'success', text: 'Você está escondida.' },
      ]);
      expect(contestNotes(hidden, 'Toren')[0].text).toBe('Você está escondido.');
      const surprised = create(ContestTurnStateSchema, { surprised: true });
      expect(contestNotes(surprised, 'Nael')[0]).toMatchObject({
        tag: 'Surpresa',
        text: 'Você está surpreso neste turno. Você não se move, não age e não reage até o fim dele.',
      });
      expect(contestNotes(surprised, 'Brisa')[0].text).toContain('Você está surpresa neste turno.');
      expect(contestNotes(undefined, 'Brisa')).toEqual([]);
      expect(contestNotes(create(ContestTurnStateSchema), 'Brisa')).toEqual([]);
    });

    it('splits the master’s refusal at its colon', () => {
      expect(refusalParts('Alguém vê você claramente: não dá para se esconder agora.')).toEqual({
        lead: 'Alguém vê você claramente:',
        rest: 'não dá para se esconder agora.',
      });
      expect(refusalParts('Sem lugar para isso.')).toEqual({
        lead: '',
        rest: 'Sem lugar para isso.',
      });
    });
  });

  it('knows the side of a combatant it is not given (a creature of the other side)', () => {
    expect(whoIs(undefined)).toEqual({ label: 'alguém', player: true });
    expect(whoIs(combatant({ id: 'x', label: 'Goblin 1', side: CombatantSide.ENEMY }))).toEqual({
      label: 'Goblin 1',
      player: false,
    });
  });
});
