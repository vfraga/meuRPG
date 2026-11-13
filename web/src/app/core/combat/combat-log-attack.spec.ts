import { DamageStepKind } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { damageNotes } from './combat-log-attack';

describe('damageNotes for a damage reduction step', () => {
  const damage = (labelPt: string, ignored = false) =>
    ({
      parts: [],
      rolled: [],
      steps: [{ kind: DamageStepKind.REDUCTION, labelPt, before: 8, after: 5, ignored }],
    }) as never;

  it('writes the step with the label the server sent', () => {
    expect(damageNotes(damage('Redução de dano de arma (Couro grosso)'))).toContain(
      'Redução de dano de arma (Couro grosso): 8 → 5',
    );
  });

  it('names the step by its kind when the label is empty, and says the master left it out', () => {
    expect(damageNotes(damage('', true))).toContain(
      'redução de dano: 8 → 5 (o mestre deixou de fora)',
    );
  });
});
