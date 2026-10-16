import type { ChoiceSelection } from '../../shared/choice-groups/choice-groups';
import type { CharacterFormValue } from './character-editor.types';
import { EditorPicks } from './editor-picks';

const NO_GIVEN = new Map<string, string>();

describe('EditorPicks', () => {
  it('picks and drops a skill, and takes the expertise of a skill it drops', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.toggleSkill('skill:arcana');
    picks.toggleSkill('skill:history');
    picks.toggleExpertise('skill:arcana');
    expect([...picks.selectedSkills()]).toEqual(['skill:arcana', 'skill:history']);
    expect([...picks.expertiseSkills()]).toEqual(['skill:arcana']);
    picks.toggleSkill('skill:arcana');
    expect([...picks.selectedSkills()]).toEqual(['skill:history']);
    expect([...picks.expertiseSkills()]).toEqual([]);
  });

  it('leaves the expertise alone when a skill without expertise is dropped', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.toggleSkill('skill:arcana');
    picks.toggleSkill('skill:history');
    picks.toggleExpertise('skill:history');
    picks.toggleSkill('skill:arcana');
    expect([...picks.selectedSkills()]).toEqual(['skill:history']);
    expect([...picks.expertiseSkills()]).toEqual(['skill:history']);
  });

  it('gives expertise to a skill that is given, and takes it off with a second click', () => {
    const picks = new EditorPicks(() => new Map([['skill:insight', 'Sábio']]));
    picks.toggleExpertise('skill:insight');
    expect([...picks.expertiseSkills()]).toEqual(['skill:insight']);
    picks.toggleExpertise('skill:insight');
    expect([...picks.expertiseSkills()]).toEqual([]);
  });

  it('does not take a skill the race or the background already gives', () => {
    const picks = new EditorPicks(() => new Map([['skill:insight', 'Sábio']]));
    picks.toggleSkill('skill:insight');
    expect([...picks.selectedSkills()]).toEqual([]);
  });

  it('gives expertise only to a skill that is proficient', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.toggleExpertise('skill:arcana');
    expect([...picks.expertiseSkills()]).toEqual([]);
  });

  it('caps the custom background at two skills and two tools or languages', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    ['a', 'b', 'c'].forEach(picks.toggleCustomBackgroundSkill);
    expect([...picks.customBackgroundSkills()]).toEqual(['a', 'b']);
    picks.toggleCustomBackgroundSkill('a');
    expect([...picks.customBackgroundSkills()]).toEqual(['b']);
    picks.setCustomProficiencies(['x', 'y', 'z']);
    expect(picks.customBackgroundProficiencies()).toEqual(['x', 'y']);
  });

  it('adds, changes and removes the classes after the first', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.addClass();
    picks.addClass();
    picks.changeExtraClass(1, { classKey: 'class:wizard', level: 3 });
    expect(picks.extraClasses()).toEqual([
      { classKey: '', level: 1, subclassKey: '', customSubclassName: '' },
      { classKey: 'class:wizard', level: 3, subclassKey: '', customSubclassName: '' },
    ]);
    // Block 1 on screen is the first extra class.
    picks.removeClass(1);
    expect(picks.extraClasses().map((c) => c.classKey)).toEqual(['class:wizard']);
  });

  it('gives a feat once, and takes it off with the slot of the improvement it replaced', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.addFeat('feat:alert');
    picks.addFeat('feat:alert');
    picks.addFeat('feat:lucky');
    picks.featSlots.set({ 'feat:alert': 'slot-1', 'feat:lucky': 'slot-2' });
    picks.removeFeat('feat:alert');
    expect(picks.featKeys()).toEqual(['feat:lucky']);
    expect(picks.featSlots()).toEqual({ 'feat:lucky': 'slot-2' });
  });

  it('follows a pick on the Escolhas step and keeps its free text', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    const choice = {
      key: 'choice:style',
      picked: [],
      options: [{ key: 'dueling', storedKey: 'feature:dueling' }],
    };
    picks.pickChoice({ choice, optionKeys: ['dueling'] } as unknown as ChoiceSelection);
    expect(picks.featureChoiceKeys()).toEqual(['feature:dueling']);
    picks.editChoiceText({ choice: { key: 'choice:enemy' }, n: 0, text: 'orcs' } as never);
    expect(Object.values(picks.featureChoiceText())).toContain('orcs');
  });

  it('loads a saved sheet and forgets it again', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.load({
      skillProficiencies: ['skill:arcana'],
      customBackgroundSkills: ['skill:a', 'skill:b'],
      customBackgroundProficiencies: ['tool:x'],
      extraClasses: [
        { classKey: 'class:rogue', level: 2, subclassKey: '', customSubclassName: '' },
      ],
      expertiseSkillKeys: ['skill:arcana'],
      hitPointsRolls: [4],
      featureChoiceKeys: ['feature:a'],
      featureChoiceText: { k: 't' },
      featKeys: ['feat:alert'],
      featSlots: { 'feat:alert': 's' },
    } as unknown as CharacterFormValue);
    expect(picks.hitPointsRolls()).toEqual([4]);
    expect(picks.featKeys()).toEqual(['feat:alert']);
    picks.preparedMaxByClass.set({ 'class:wizard': 4 });
    picks.hasCuttingWords.set(true);
    picks.reset();
    expect(picks.selectedSkills().size).toBe(0);
    expect(picks.extraClasses()).toEqual([]);
    expect(picks.featSlots()).toEqual({});
    expect(picks.preparedMaxByClass()).toEqual({});
    expect(picks.hasCuttingWords()).toBe(false);
  });

  it('loads a saved sheet that has no custom background skills or feats', () => {
    const picks = new EditorPicks(() => NO_GIVEN);
    picks.load({
      skillProficiencies: [],
      customBackgroundSkills: null,
      customBackgroundProficiencies: [],
      extraClasses: [],
      expertiseSkillKeys: [],
      hitPointsRolls: [],
      featureChoiceKeys: [],
      featureChoiceText: {},
    } as unknown as CharacterFormValue);
    expect(picks.customBackgroundSkills().size).toBe(0);
    expect(picks.featKeys()).toEqual([]);
  });
});
