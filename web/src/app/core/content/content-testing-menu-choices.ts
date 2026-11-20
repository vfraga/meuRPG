import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuValueSchema,
  EffectOptionSetSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menuResponse } from './content-testing-menu';
import { EffectMenuVm } from './effect-draft';

/**
 * The menu with the real one's choice effect as RN-33 has it (backend/internal/rules/tablemenu.go): `filter` (kind "choice_filter",
 * list "proficiency_kinds") and `from_feature` (kind "feature_ref"), the choice kinds "tool" and "feature", the lists of the proficiencies
 * of each kind and the SRD features that offer options (the eldritch invocations among them, which a choice never offers).
 */
/** How many proficiencies of each kind the menu lists: the counts the specs read back. */
export const WEAPONS = 37;
export const TOOLS = 20;

const value = (key: string, namePt: string) => create(EffectMenuValueSchema, { key, namePt });

export function choiceSourceMenu(): EffectMenuVm {
  const res = menuResponse();
  const choice = res.types.find((t) => t.type === 'choice')!;
  choice.fields = [
    { name: 'choice', kind: 'choice', required: true, list: 'choice_kinds' },
    { name: 'count', kind: 'number', required: true, min: 1, max: 20 },
    { name: 'from', kind: 'choices' },
    { name: 'filter', kind: 'choice_filter', list: 'proficiency_kinds' },
    { name: 'from_feature', kind: 'feature_ref' },
    { name: 'name', kind: 'text' },
    { name: 'set', kind: 'choice', list: 'feat_option_sets' },
    { name: 'daily_choice', kind: 'yes_no' },
    { name: 'when', kind: 'condition' },
    { name: 'text_pt', kind: 'text' },
  ] as never;
  const kinds = res.lists.find((l) => l.name === 'choice_kinds')!;
  kinds.values.push(value('tool', 'Ferramentas'));
  const proficiencies = (kind: string, n: number) =>
    Array.from({ length: n }, (_, i) => value(`proficiency:${kind}-${i}`, `${kind} ${i}`));
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'proficiency_kinds',
      values: [
        value('weapon', 'Armas'),
        value('tool', 'Ferramentas'),
        value('other', 'Veículos e kits'),
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'tool_choices',
      values: [
        value('proficiency:longsword', 'Espada longa'),
        value('proficiency:cartographers-tools', 'Ferramentas de cartógrafo'),
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'weapon_proficiencies',
      values: proficiencies('weapon', WEAPONS),
    }),
    create(EffectMenuListSchema, {
      name: 'tool_proficiencies',
      values: proficiencies('tool', TOOLS),
    }),
    create(EffectMenuListSchema, {
      name: 'other_proficiencies',
      values: proficiencies('other', 1),
    }),
  );
  res.optionSets.push(
    create(EffectOptionSetSchema, {
      key: 'feature:eldritch-invocations',
      namePt: 'Invocações místicas',
      choose: 0,
      options: [value('option:agonizing-blast', 'Rajada agonizante')],
    }),
  );
  const menu = new EffectMenuVm(res);
  menu.tableFeatures = [
    {
      key: 'feature:class-guardiao-3f9a1c--truques@mesa',
      namePt: 'Truques de lâmina (Guarda do Corvo)',
      count: 3,
    },
  ];
  return menu;
}
