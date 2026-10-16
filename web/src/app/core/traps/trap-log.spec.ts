import { create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  PendingDamageStatus,
  SaveOutcome,
  TrapFiringSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { TrapActivitySchema, TrapSearchSkill } from '../../../gen/meurpg/play/v1/traps_pb';
import { activityLines, fallNote, trapLogText } from './trap-log';

const firing = create(TrapFiringSchema, {
  pointId: 'p',
  name: 'Fosso escondido',
  caught: [
    {
      targetId: 'c1',
      targetLabel: 'Toren',
      characterId: 'char-toren',
      saves: [{ outcome: SaveOutcome.FAILED }],
      damages: [{ amount: 7, damageTypePt: 'concussão', status: PendingDamageStatus.ROLLED }],
    },
    { targetId: 'c2', targetLabel: 'Goblin 1', attacks: [{ outcome: AttackOutcome.MISS }] },
  ],
});

describe('trap log', () => {
  it('writes the public line of a firing without any DC', () => {
    const text = trapLogText(firing, false);
    expect(text).toContain('A armadilha Fosso escondido foi disparada: Toren caiu');
    expect(text).toContain('7 de concussão, esperando o mestre aplicar');
    expect(trapLogText(firing, true)).toContain('esperando você aplicar');
    expect(text).toContain('falhou no teste');
    expect(text).not.toMatch(/CD/);
    expect(trapLogText(create(TrapFiringSchema, { name: 'X' }), false)).toBe(
      'A armadilha X foi disparada. Ninguém estava na área.',
    );
  });

  it('tells the player what happened to their own character and whether the damage waits', () => {
    const note = fallNote(firing, 'c1')!;
    expect(note.title).toBe('Você caiu na armadilha Fosso escondido.');
    expect(note.detail).toContain('7 de concussão');
    expect(note.detail).not.toContain('esperando');
    expect(note.waiting).toBe(true);
    expect(fallNote(firing, 'other')).toBeNull();
  });

  it('names a creature of the player that a trap caught on its own part', () => {
    const note = fallNote(firing, 'c1', 'Lobo atroz 1')!;
    expect(note.title).toBe('O Lobo atroz 1 caiu na armadilha Fosso escondido.');
    expect(note.waiting).toBe(true);
    expect(fallNote(firing, 'c1', 'Nanquim')!.title).toBe(
      'O Nanquim caiu na armadilha Fosso escondido.',
    );
    expect(fallNote(firing, 'c1', 'Cobra 2')!.title).toBe(
      'A Cobra 2 caiu na armadilha Fosso escondido.',
    );
  });

  it('matches the caught by combatant: a familiar and its owner each get their own line', () => {
    const both = create(TrapFiringSchema, {
      name: 'Fosso',
      caught: [
        {
          targetId: 'owner',
          targetLabel: 'Pensantus',
          characterId: 'p',
          damages: [{ amount: 4, damageTypePt: 'concussão', status: PendingDamageStatus.ROLLED }],
        },
        {
          targetId: 'raven',
          targetLabel: 'Nanquim',
          characterId: 'p',
          damages: [{ amount: 1, damageTypePt: 'concussão', status: PendingDamageStatus.APPLIED }],
        },
      ],
    });
    expect(fallNote(both, 'owner')!.detail).toContain('4 de concussão');
    expect(fallNote(both, 'raven')!.detail).toContain('1 de concussão');
    expect(fallNote(both, 'raven')!.waiting).toBe(false);
  });

  it('writes the search and notice lines of the activity', () => {
    const search = create(TrapActivitySchema, {
      id: 's',
      search: {
        characterName: 'Brisa',
        skill: TrapSearchSkill.INVESTIGATION,
        roll: { diceCount: 1, diceSides: 20, faces: [13], modifier: 4, total: 17 },
        foundNames: ['Fosso escondido'],
      },
    });
    const [line] = activityLines(search, true);
    expect(line.actor).toBe('Brisa');
    expect(line.text).toContain('procurou armadilhas (Investigação)');
    expect(line.text).toContain('achou a armadilha Fosso escondido');
    const none = create(TrapActivitySchema, {
      id: 'n',
      search: {
        characterName: 'Pensantus',
        skill: TrapSearchSkill.PERCEPTION,
        roll: { diceCount: 1, diceSides: 20, faces: [6], total: 6 },
      },
    });
    expect(activityLines(none, false)[0].text).toContain('não achou nada');
    // A search with another skill names it, and a roll Talento Confiável raised shows the account.
    const other = create(TrapActivitySchema, {
      id: 'o',
      search: {
        characterName: 'Pensantus',
        skill: TrapSearchSkill.OTHER,
        otherSkillKey: 'skill:arcana',
        roll: { diceCount: 1, diceSides: 20, faces: [12], modifier: 8, total: 20 },
        foundNames: ['Estátua que cospe fogo'],
      },
    });
    expect(activityLines(other, true)[0].text).toBe(
      ' procurou armadilhas (Arcanismo): 1d20 (12) + 8 = 20, e achou a armadilha Estátua que cospe fogo',
    );
    const raised = create(TrapActivitySchema, {
      id: 'r',
      search: {
        characterName: 'Brisa',
        skill: TrapSearchSkill.OTHER,
        otherSkillKey: 'skill:acrobatics',
        roll: {
          diceCount: 1,
          diceSides: 20,
          faces: [6],
          modifier: 9,
          total: 19,
          treatedAs: 10,
          treatedAsSource: 'feature:reliable-talent',
        },
      },
    });
    expect(activityLines(raised, true)[0].text).toBe(
      ' procurou armadilhas (Acrobacia): d20: 6 → 10 (Talento Confiável) + 9 = 19, e não achou nada',
    );
    const notice = create(TrapActivitySchema, {
      id: 'q',
      notice: { characterNames: ['Sálvia'], trapName: 'Fosso escondido' },
    });
    expect(activityLines(notice, false)[0]).toMatchObject({
      actor: 'Sálvia',
      text: ' notou a armadilha Fosso escondido ao passar',
    });
  });

  it('says the d20 the halfling’s Lucky rolled again beside the search’s roll', () => {
    const lucky = create(TrapActivitySchema, {
      id: 'l',
      search: {
        characterName: 'Pipo',
        skill: TrapSearchSkill.INVESTIGATION,
        roll: {
          diceCount: 1,
          diceSides: 20,
          faces: [14],
          modifier: 4,
          total: 18,
          rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }],
        },
      },
    });
    expect(activityLines(lucky, true)[0].text).toBe(
      ' procurou armadilhas (Investigação): 1d20 (14) + 4 = 18; Sortudo: 1 → 14, e não achou nada',
    );
  });

  it('says the save the app rolled again for a halfling beside its dice, and nothing without the dice', () => {
    const save = (withRoll: boolean) =>
      create(TrapFiringSchema, {
        name: 'Estátua de Fogo',
        caught: [
          {
            targetId: 'b',
            targetLabel: 'Brisa',
            saves: [
              {
                outcome: SaveOutcome.SAVED,
                dc: 12,
                roll: withRoll
                  ? {
                      diceCount: 1,
                      diceSides: 20,
                      faces: [14],
                      modifier: 4,
                      total: 18,
                      rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }],
                    }
                  : undefined,
              },
            ],
          },
        ],
      });
    expect(trapLogText(save(true), true)).toContain(
      'Brisa caiu (passou no teste (1d20 (14) + 4 = 18 contra CD 12; Sortudo: 1 → 14))',
    );
    expect(trapLogText(save(false), false)).not.toContain('Sortudo');
  });
});
