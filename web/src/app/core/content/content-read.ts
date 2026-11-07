import type {
  TableBackground,
  TableClassFeature,
  TableEntry,
  TableFeat,
  TableFeature,
  TableOption,
  TableRace,
  TableSpell,
  TableSubrace,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readClass, readSubclass } from './content-read-class';
import {
  type EntryRead,
  type NameOf,
  type ParentView,
  type ReadRow,
  abilitiesOf,
  bonusesOf,
  featureItem,
  metres,
  names,
} from './content-read-parts';
import { bonusText } from './feature-draft';
import { summonLine } from './summon-names';
import { entryEffectsPart } from './content-read-rolls';
import { spellToDraft } from './spell-draft';
import { previewRows } from './spell-preview';
import { featChoiceRows, featOptionItems } from './content-read-feat';
import { prerequisiteSupport, SIZE_WORDS } from './feat-text';
import { featureKeyName } from './class-feature-text';
import { readCreature } from './content-read-creature';
import { type OfferedByName, optionPrerequisiteParts, optionSetName } from './option-text';
import { parentTraitsOf, replacedNames } from './subrace-traits';

function raceAbilitiesRow(r: TableRace, nameOf: NameOf): ReadRow {
  const choice =
    r.choiceBonuses.length > 0
      ? `${r.choiceBonuses.map((n) => `+${n}`).join(' e ')} à escolha`
      : '';
  const fixed = bonusText(bonusesOf(r.abilityBonuses), abilitiesOf(nameOf));
  return {
    label: 'Habilidades',
    value: [fixed === 'Nenhum' ? '' : fixed, choice].filter((x) => x).join(', ') || 'Nenhum',
  };
}

function readRace(r: TableRace, nameOf: NameOf): EntryRead {
  const rows: ReadRow[] = [
    raceAbilitiesRow(r, nameOf),
    { label: 'Tamanho', value: SIZE_WORDS[r.size] ?? r.size },
    { label: 'Deslocamento', value: metres(r.speedFt) },
  ];
  if (r.darkvisionFt > 0) rows.push({ label: 'Visão no escuro', value: metres(r.darkvisionFt) });
  const langs = [
    names(r.languages, nameOf),
    r.languageChoices > 0 ? `${r.languageChoices} à escolha` : '',
  ]
    .filter((x) => x)
    .join(', ');
  if (langs) rows.push({ label: 'Idiomas', value: langs });
  const sections =
    r.traits.length > 0
      ? [{ title: 'Traços', items: r.traits.map((t) => featureItem(t, nameOf)) }]
      : [];
  return { rows, sections, text: [] };
}

function subraceTraitSections(
  s: TableSubrace,
  nameOf: NameOf,
  parent: ParentView,
): EntryRead['sections'] {
  const raceName = nameOf(s.raceKey);
  const own = s.traits.map((t) => featureItem(t, nameOf));
  const kept = parent.traits
    .filter((t) => !s.replacesTraitKeys.includes(t.key))
    .map((t) => featureItem(t, nameOf));
  if (parent.split) {
    return [
      ...(kept.length > 0 ? [{ title: `Traços de ${raceName}`, items: kept }] : []),
      ...(own.length > 0 ? [{ title: 'Traços da sub-raça', items: own }] : []),
    ];
  }
  const all = [...own, ...kept.map((i) => ({ ...i, tag: `(de ${raceName})` }))];
  return all.length > 0 ? [{ title: 'Traços', items: all }] : [];
}

function readSubrace(s: TableSubrace, nameOf: NameOf, parent: ParentView): EntryRead {
  const rows: ReadRow[] = [];
  if (s.raceKey) rows.push({ label: 'Raça', value: nameOf(s.raceKey) });
  const bonuses = bonusText(bonusesOf(s.abilityBonuses), abilitiesOf(nameOf));
  rows.push({
    label: 'Habilidades',
    value: !s.replacesAbilityBonuses
      ? bonuses
      : bonuses === 'Nenhum'
        ? 'Nenhum; os da raça não valem'
        : `${bonuses}, no lugar da raça`,
  });
  // Only a trait the reader can name is listed (RN-23).
  const replaced = replacedNames(parentTraitsOf(parent.traits, nameOf), s.replacesTraitKeys);
  if (!parent.split && replaced.length > 0) {
    rows.push({ label: 'Substitui', value: replaced.join(', ') });
  }
  return { rows, sections: subraceTraitSections(s, nameOf, parent), text: [] };
}

function readBackground(b: TableBackground, nameOf: NameOf): EntryRead {
  const rows: ReadRow[] = [{ label: 'Perícias', value: names(b.skills, nameOf) || '—' }];
  if (b.tools.length > 0) rows.push({ label: 'Ferramentas', value: names(b.tools, nameOf) });
  if (b.languageChoices > 0)
    rows.push({ label: 'Idiomas à escolha', value: String(b.languageChoices) });
  if (b.equipmentPt) rows.push({ label: 'Equipamento', value: b.equipmentPt });
  const sections =
    b.feature && b.feature.namePt
      ? [{ title: 'Característica', items: [featureItem(b.feature, nameOf)] }]
      : [];
  return { rows, sections, text: [] };
}

function readFeat(
  f: TableFeat,
  nameOf: NameOf,
  optionSide?: (o: TableFeature) => string,
): EntryRead {
  const asks = prerequisiteSupport(f.prerequisite, nameOf);
  const rows: ReadRow[] = [{ label: 'Pré-requisito', value: asks || 'Nenhum' }];
  for (const e of f.effects) {
    if (e.type === 'ability_increase' && e.count > 0) {
      const from = e.from.map((k) => nameOf(`ability:${k}`));
      rows.push({
        label: 'Aumento de habilidade',
        value: `+${e.value} em ${e.count === 1 ? 'uma' : e.count} ${e.count === 1 ? 'habilidade' : 'habilidades'} à escolha: ${from.join(', ')}`,
      });
    }
  }
  rows.push(...featChoiceRows(f));
  const options = featOptionItems(f, nameOf, optionSide);
  return {
    rows,
    sections: [],
    text: f.descPt,
    ...entryEffectsPart(f.effects, nameOf),
    ...(options.length > 0 ? { options } : {}),
  };
}

function readOption(o: TableOption, nameOf: NameOf, offeredByName: OfferedByName): EntryRead {
  const asks = optionPrerequisiteParts(o.prerequisite, nameOf);
  const rows: ReadRow[] = [
    { label: 'Conjunto', value: optionSetName(o.set) },
    ...(o.offeredBy === '' ? [] : [{ label: 'Oferecida por', value: offeredByName(o.offeredBy) }]),
    { label: 'Pré-requisito', value: asks.length > 0 ? asks.join(', ') : 'Nenhum' },
  ];
  return { rows, sections: [], text: o.descPt, ...entryEffectsPart(o.effects, nameOf) };
}

function readClassFeature(f: TableClassFeature, nameOf: NameOf): EntryRead {
  const rows: ReadRow[] = [
    { label: 'Classe', value: nameOf(f.classKey) },
    { label: 'Nível', value: `${f.level}º` },
    {
      label: 'Substitui',
      value: f.replacesFeatureKey === '' ? 'Nada' : featureKeyName(f.replacesFeatureKey, nameOf),
    },
  ];
  const summon = summonLine(f.summon, nameOf);
  return {
    rows,
    sections: [],
    text: summon ? [...f.descPt, summon] : f.descPt,
    ...entryEffectsPart(f.effects, nameOf),
  };
}

function readSpell(s: TableSpell, nameOf: NameOf): EntryRead {
  const rows: ReadRow[] = [...previewRows(spellToDraft(s), nameOf)];
  const classes = names(s.classKeys, nameOf);
  if (classes) rows.push({ label: 'Classes', value: classes });
  const summon = summonLine(s.summon, nameOf);
  return {
    rows,
    sections: [],
    text: [...s.descPt, ...s.higherLevelPt, ...(summon ? [summon] : [])],
  };
}

export function readEntry(
  entry: TableEntry,
  nameOf: NameOf,
  subclassLevelOf: (classKey: string) => number = () => 0,
  parentOf: (raceKey: string) => ParentView = () => ({ traits: [] }),
  offeredByName: OfferedByName = (key) => featureKeyName(key, nameOf),
  optionSide?: (o: TableFeature) => string,
): EntryRead {
  const body = entry.body;
  switch (body.case) {
    case 'tableRace':
      return readRace(body.value, nameOf);
    case 'tableSubrace':
      return readSubrace(body.value, nameOf, parentOf(body.value.raceKey));
    case 'tableBackground':
      return readBackground(body.value, nameOf);
    case 'tableFeat':
      return readFeat(body.value, nameOf, optionSide);
    case 'tableOption':
      return readOption(body.value, nameOf, offeredByName);
    case 'tableClassFeature':
      return readClassFeature(body.value, nameOf);
    case 'tableSpell':
      return readSpell(body.value, nameOf);
    case 'tableCreature':
      return readCreature(body.value, nameOf);
    case 'tableClass':
      return readClass(body.value, nameOf);
    case 'tableSubclass':
      return readSubclass(body.value, nameOf, subclassLevelOf);
    default:
      return { rows: [], sections: [], text: [] };
  }
}
