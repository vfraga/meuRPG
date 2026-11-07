import { create } from '@bufbuild/protobuf';

import { TableOptionSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menu } from './content-testing-menu';
import {
  draftToOption,
  emptyOption,
  missingOffering,
  optionPaths,
  optionToDraft,
} from './option-draft';

const stored = () =>
  create(TableOptionSchema, {
    namePt: 'Olhar do Corvo',
    descPt: ['Primeiro.', 'Segundo.'],
    set: 'invocation',
    prerequisite: {
      classLevel: { classKey: 'class:warlock', level: 5 },
      requiresOptionKeys: ['option:agonizing-blast'],
      requiresPact: 'feature:pact-of-the-tome',
      requiresSpellKey: 'spell:comprehend-languages',
    },
    effects: [{ type: 'note', textPt: 'Enxerga pelos olhos de um corvo.' }],
  });

describe('the option form (MR-025)', () => {
  it('turns a stored option into a draft and back to the same body', () => {
    const draft = optionToDraft(stored());
    expect(draft.feature.text).toBe('Primeiro.\n\nSegundo.');
    expect(draft.set).toBe('invocation');
    expect(draft.classKey).toBe('class:warlock');
    expect(draft.classLevel).toBe('5');
    expect(draftToOption(draft, menu())).toEqual({
      namePt: 'Olhar do Corvo',
      descPt: ['Primeiro.', 'Segundo.'],
      set: 'invocation',
      offeredBy: '',
      prerequisite: {
        classLevel: { classKey: 'class:warlock', level: 5 },
        requiresOptionKeys: ['option:agonizing-blast'],
        requiresPact: 'feature:pact-of-the-tome',
        requiresSpellKey: 'spell:comprehend-languages',
      },
      effects: [{ type: 'note', textPt: 'Enxerga pelos olhos de um corvo.' }],
    });
  });

  it('keeps the offering feature of an option of a table feature through an edit', () => {
    const draft = optionToDraft({
      ...stored(),
      set: 'table_feature',
      offeredBy: 'feature:postura@mesa',
    });
    expect(draft.offeredBy).toBe('feature:postura@mesa');
    expect(draftToOption(draft, menu()).offeredBy).toBe('feature:postura@mesa');
  });

  it('sends a prerequisite that asks nothing as empty, with no class level', () => {
    const init = draftToOption(emptyOption('fighting_style'), menu());
    expect(init.set).toBe('fighting_style');
    expect(init.prerequisite).toEqual({
      requiresOptionKeys: [],
      requiresPact: '',
      requiresSpellKey: '',
    });
    expect(init.effects).toEqual([]);
  });

  it('sends a half-filled class level as typed, so the server names the field', () => {
    const d = { ...emptyOption('invocation'), classKey: 'class:warlock' };
    expect(draftToOption(d, menu()).prerequisite?.classLevel).toEqual({
      classKey: 'class:warlock',
      level: 0,
    });
    const e = { ...emptyOption('invocation'), classLevel: '5' };
    expect(draftToOption(e, menu()).prerequisite?.classLevel).toEqual({ classKey: '', level: 5 });
  });

  it('lists the inputs the editor draws, with the fields of each effect type', () => {
    const paths = optionPaths(optionToDraft(stored()), menu());
    expect(paths).toContain('table_option.set');
    expect(paths).toContain('table_option.prerequisite.class_level.level');
    expect(paths).toContain('table_option.prerequisite.requires_option_keys');
    expect(paths).toContain('table_option.effects[0].type');
    expect(paths).toContain('table_option.effects[0].text_pt');
  });

  it('keeps the offering feature of a table feature option and drops it for any other set', () => {
    const own = create(TableOptionSchema, {
      namePt: 'Golpe',
      set: 'table_feature',
      offeredBy: 'feature:x@mesa',
    });
    const draft = optionToDraft(own);
    expect(draft.offeredBy).toBe('feature:x@mesa');
    expect(draftToOption(draft, menu()).offeredBy).toBe('feature:x@mesa');
    expect(draftToOption({ ...draft, set: 'invocation' }, menu()).offeredBy).toBe('');
  });

  it('draws the offering feature input and asks for it only for the table feature set', () => {
    const own = { ...emptyOption('table_feature') };
    expect(optionPaths(own, menu())).toContain('table_option.offered_by');
    expect(optionPaths(emptyOption('invocation'), menu())).not.toContain('table_option.offered_by');
    expect(missingOffering(own)).toBe(true);
    expect(missingOffering({ ...own, offeredBy: 'feature:x@mesa' })).toBe(false);
    expect(missingOffering(emptyOption('invocation'))).toBe(false);
  });
});
