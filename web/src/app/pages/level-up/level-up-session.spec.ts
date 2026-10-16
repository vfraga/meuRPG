import { create, type MessageInitShape } from '@bufbuild/protobuf';

import { DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterSchema,
  LevelUpDiceRule,
  LevelUpHitPointsRule,
  LevelUpOptionsSchema,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { PactMagicSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { LevelUpDraft } from '../../core/levelup/levelup-draft';
import {
  SKILLS,
  SPELLS,
  WIZARD_KEYS,
  pensantus,
  wizardOptions,
} from '../../core/levelup/levelup-testing';
import type { LevelUpClient } from '../../core/levelup/levelup-client';
import { LevelUpSession, newSpellsTitle } from './level-up-session';

describe('LevelUpSession', () => {
  const client = { rollHitPoints: vi.fn(), preview: vi.fn() };

  function session(options: LevelUpOptions = wizardOptions(), preference = DicePreference.APP) {
    const character = create(CharacterSchema, {
      id: 'ch-1',
      name: 'Pensantus',
      revision: 3,
      derived: pensantus(),
    });
    const draft = new LevelUpDraft(options, WIZARD_KEYS, { spells: SPELLS, skills: SKILLS });
    return new LevelUpSession(
      'camp-1',
      character,
      options,
      draft,
      client as unknown as LevelUpClient,
      preference,
      () => undefined,
    );
  }
  const opts = (over: MessageInitShape<typeof LevelUpOptionsSchema>) => wizardOptions(over);

  afterEach(() => {
    vi.useRealTimers();
  });

  const give = (title: string, sub: string, tag: 'chosen' | 'choose' | 'auto') => ({
    title,
    sub,
    tag,
  });

  it('lists what the level gives in the order of the steps: chosen, still to choose, automatic', () => {
    expect(session().gives()).toEqual([
      give('Incremento no Valor de Habilidade', 'Passo 2', 'choose'),
      give('Truque novo e 2 magias para o livro', 'Passo 4', 'choose'),
      give('Magias preparadas: até 8', 'Feito no passo 4', 'chosen'),
      give('Espaços de 2º nível', 'Entra sozinho · 2 → 3', 'auto'),
      give('Dados de vida', 'Entra sozinho · 3d6 → 3d6', 'auto'),
    ]);
  });

  it('names every kind of choice and automatic change a level can bring, and skips what the flow already covers', () => {
    const s = session(
      opts({
        abilityScoreImprovement: false,
        subclassDue: true,
        skillChoices: 2,
        expertiseChoices: 1,
        cantrips: 2,
        spells: 0,
        prepares: false,
        featureChoices: [
          { feature: { key: 'feature:style', namePt: 'Estilo' }, choose: 1, options: [] },
          { choose: 1, options: [] },
        ],
        masterAdds: [{ key: 'feature:x', namePt: 'Coisa' }],
        pactMagicBefore: create(PactMagicSchema, { count: 1, slotLevel: 1 }),
        pactMagicAfter: create(PactMagicSchema, { count: 2, slotLevel: 1 }),
        proficiencyBonusAfter: 3,
        spellSlotsBefore: [4],
        spellSlotsAfter: [4, 2],
        newFeatures: [
          { key: 'feature:extra', namePt: 'Extra' },
          { key: 'feature:subclass-1', namePt: 'Sub' },
          { key: 'feature:style', namePt: 'Estilo' },
          { key: 'feature:x', namePt: 'Coisa' },
        ],
      }),
    );
    expect(s.gives()).toEqual([
      give('Subclasse', 'Passo 3', 'choose'),
      give('Estilo', 'Passo 3', 'choose'),
      give('Característica', 'Passo 3', 'choose'),
      give('2 perícias novas', 'Passo 3', 'choose'),
      give('Especialização', 'Passo 3', 'choose'),
      give('2 truques novos', 'Passo 4', 'choose'),
      give('Espaços de 2º nível', 'Entra sozinho · 0 → 2', 'auto'),
      give(
        'Espaços do pacto',
        'Entra sozinho · 1\u00a0espaço\u00a0de\u00a01º\u00a0nível → 2\u00a0espaços\u00a0de\u00a01º\u00a0nível',
        'auto',
      ),
      give('Bônus de proficiência', 'Entra sozinho · +2 → +3', 'auto'),
      give('Dados de vida', 'Entra sozinho · 3d6 → 3d6', 'auto'),
      give('Extra', 'Característica do nível', 'auto'),
    ]);
    expect(s.masterAdds()).toBe('Coisa');
  });

  it('says one new skill and one new cantrip in the singular', () => {
    const titles = session(opts({ skillChoices: 1, spells: 1 }))
      .gives()
      .map((g) => g.title);
    expect(titles).toEqual([
      'Incremento no Valor de Habilidade',
      'Perícia nova',
      'Truque novo e 1 magia para o livro',
      'Magias preparadas: até 8',
      'Espaços de 2º nível',
      'Dados de vida',
    ]);
  });

  it('words the level and the Constituição as the page shows them', () => {
    const s = session();
    expect(s.levelWords).toBe('o nível 4');
    expect(s.withCon(4)).toBe('4 + Constituição +3');
    expect(newSpellsTitle(0, false)).toBe('');
    expect(newSpellsTitle(1, false)).toBe('1 magia conhecida');
    expect(newSpellsTitle(2, false)).toBe('2 magias conhecidas');
    expect(newSpellsTitle(1, true)).toBe('1 magia para o livro');
    expect(newSpellsTitle(2, true)).toBe('2 magias para o livro');
  });

  it('offers the die as the dice rule and the campaign preference allow (RN-18)', () => {
    const only = (rule: LevelUpDiceRule, pref = DicePreference.APP) =>
      session(opts({ diceRule: rule }), pref);
    const app = only(LevelUpDiceRule.FORCED_IN_APP);
    expect([app.canApp, app.canType, app.preferApp]).toEqual([true, false, true]);
    const physical = only(LevelUpDiceRule.FORCED_PHYSICAL);
    expect([physical.canApp, physical.canType, physical.preferApp]).toEqual([false, true, false]);
    const free = only(LevelUpDiceRule.PLAYER_CHOOSES, DicePreference.PHYSICAL);
    expect([free.canApp, free.canType, free.preferApp]).toEqual([true, true, false]);
  });

  it('leaves the hit points to the table when its rule fixes them, and takes a kept roll back on "rolar"', () => {
    expect(session().hpFixed).toBeNull();
    expect(session(opts({ hitPointsRule: LevelUpHitPointsRule.AVERAGE_ONLY })).hpFixed).toBe(
      'average',
    );
    const rolled = session(
      opts({ hitPointsRule: LevelUpHitPointsRule.ROLL_ONLY, keptHitPointRoll: 5 }),
    );
    expect(rolled.hpFixed).toBe('roll');
    expect(rolled.draft.rolled()).toEqual({ kind: 'app', value: 5 });
  });

  it('refuses the app roll when a die was kept for another class, and says which', () => {
    const s = session(
      opts({
        keptHitPointRoll: 4,
        keptHitPointRollClassKey: 'class:cleric',
        classKey: 'class:wizard',
      }),
    );
    expect(s.rollOtherClass()).toContain('O dado deste nível já foi rolado para outra classe (4)');
    void s.rollInApp();
    expect(client.rollHitPoints).not.toHaveBeenCalled();
  });
});
