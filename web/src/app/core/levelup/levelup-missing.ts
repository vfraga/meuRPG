import {
  LevelUpSpellsKind,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { Ability, FeatOption } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { AbilityKey } from '../characters/characters.types';
import { type ChoiceDrafts, missingIn } from './levelup-choices';
import type { HpCard, Rolled } from './levelup-draft-types';
import { type Missing, type PickItem, type Totals, needText } from './levelup-flow';
import { LIST } from './levelup-summary-rows';

/** What the draft knows that the list of missing picks is made from: every member is a signal or a plain value of the draft. */
export interface MissingSource {
  readonly options: LevelUpOptions;
  readonly effective: () => LevelUpOptions;
  readonly totals: () => Totals;
  readonly taking: () => boolean;
  readonly feat: () => FeatOption | undefined;
  readonly featAbilities: () => readonly Ability[];
  readonly featAsked: () => number;
  readonly abilityKeys: () => readonly AbilityKey[];
  readonly abilityAsked: () => number;
  readonly hpCard: () => HpCard;
  readonly rolled: () => Rolled | null;
  readonly lateDrafts: () => ChoiceDrafts;
  readonly newDrafts: () => ChoiceDrafts;
  readonly subclassKey: () => string;
  readonly features: () => ReadonlySet<string>;
  readonly skills: () => ReadonlySet<string>;
  readonly skillsAsked: () => number;
  readonly skillItems: () => readonly PickItem[];
  readonly instrument: () => string;
  readonly instrumentAsked: () => number;
  readonly expertise: () => ReadonlySet<string>;
  readonly expertiseAsked: () => number;
  readonly expertiseItems: () => readonly PickItem[];
  readonly cantrips: () => ReadonlySet<string>;
  readonly cantripsAsked: () => number;
  readonly cantripItems: () => readonly PickItem[];
  readonly spells: () => ReadonlySet<string>;
  readonly spellsAsked: () => number;
  readonly spellItems: () => readonly PickItem[];
  readonly prepared: () => ReadonlySet<string>;
  readonly preparedAsked: () => number;
  /** The swap: its groups with half a swap chosen are a missing choice. */
  readonly swap: { readonly halfGroups: () => readonly { readonly namePt: string }[] };
}

/** The ability step: the feat, or the abilities of the increase, that are still to pick. */
function abilityMissing(d: MissingSource): Missing[] {
  const miss = (text: string): Missing[] => [{ step: 'abilities', id: 'abilities', text }];
  if (d.taking()) {
    if (d.feat() === undefined) {
      return miss('Falta escolher o talento.');
    }
    const n = d.featAsked() - d.featAbilities().length;
    return n > 0 ? miss(needText(n, 'habilidade', 'habilidades')) : [];
  }
  const n = d.abilityAsked() - d.abilityKeys().length;
  return d.options.abilityScoreImprovement && n > 0
    ? miss(needText(n, 'habilidade', 'habilidades'))
    : [];
}

/** The choices of earlier levels come first (the server refuses the level without them), then the new level's own. */
function openChoicesMissing(d: MissingSource): Missing[] {
  const out: Missing[] = [];
  const late = missingIn(d.options.lateChoices, d.lateDrafts());
  if (late.count > 0) {
    out.push({
      step: 'picks',
      id: 'late',
      text: needText(late.count, 'escolha que ficou para trás', 'escolhas que ficaram para trás'),
    });
  }
  const fresh = missingIn(d.options.newChoices, d.newDrafts());
  if (fresh.count > 0) {
    out.push({
      step: 'picks',
      id: 'new',
      text: needText(fresh.count, 'escolha do nível', 'escolhas do nível'),
    });
  }
  return out;
}

/** The options of each feature choice (a fighting style, the metamagic) that are still to pick. */
function featureMissing(d: MissingSource): Missing[] {
  return d.totals().featureChoices.flatMap((f, i) => {
    const picked = f.options.filter((opt) => d.features().has(opt.key)).length;
    if (picked >= f.choose) {
      return [];
    }
    const name = f.feature?.namePt ?? 'característica';
    return [
      {
        step: 'picks' as const,
        id: `feature-${i}`,
        text: needText(f.choose - picked, `opção de ${name}`, `opções de ${name}`),
      },
    ];
  });
}

/** A list with fewer rows than the level asks cannot be completed: the message says so (the server would refuse less). */
function lack(asked: number, items: readonly PickItem[], what: string): string {
  return items.length < asked
    ? `A lista só traz ${items.length} ${what} e o nível pede ${asked}. Peça ao mestre para ajustar a ficha.`
    : '';
}

/** One counted pick (skills, cantrips, spells...): its step, how many are asked and what to say for the ones still missing. */
interface Counted {
  readonly step: Missing['step'];
  readonly id: string;
  readonly picked: ReadonlySet<string>;
  readonly asked: number;
  readonly text: (n: number) => string;
}

function countedMissing(c: Counted): Missing[] {
  const n = Math.max(0, c.asked - c.picked.size);
  return n > 0 ? [{ step: c.step, id: c.id, text: c.text(n) }] : [];
}

function skillsMissing(d: MissingSource): Missing[] {
  return countedMissing({
    step: 'picks',
    id: 'skills',
    picked: d.skills(),
    asked: d.skillsAsked(),
    text: (n) =>
      lack(d.skillsAsked(), d.skillItems(), 'perícias') || needText(n, 'perícia', 'perícias'),
  });
}

function expertiseMissing(d: MissingSource): Missing[] {
  return countedMissing({
    step: 'picks',
    id: 'expertise',
    picked: d.expertise(),
    asked: d.expertiseAsked(),
    text: (n) =>
      lack(d.expertiseAsked(), d.expertiseItems(), 'perícias para a especialização') ||
      needText(n, 'especialização', 'especializações'),
  });
}

/** The spell step: the cantrips, the new spells (for the book or the known list) and the ones to prepare. */
function spellsMissing(d: MissingSource): Missing[] {
  const where =
    d.effective().spellsKind === LevelUpSpellsKind.SPELLBOOK
      ? 'para o livro'
      : 'para as magias conhecidas';
  return [
    ...countedMissing({
      step: 'spells',
      id: 'cantrips',
      picked: d.cantrips(),
      asked: d.cantripsAsked(),
      text: (n) =>
        lack(d.cantripsAsked(), d.cantripItems(), 'truques') || needText(n, 'truque', 'truques'),
    }),
    ...countedMissing({
      step: 'spells',
      id: 'spells',
      picked: d.spells(),
      asked: d.spellsAsked(),
      text: (n) =>
        lack(d.spellsAsked(), d.spellItems(), 'magias') ||
        needText(n, 'magia', 'magias').replace(/\.$/, ` ${where}.`),
    }),
    ...countedMissing({
      step: 'spells',
      id: 'prepared',
      picked: d.prepared(),
      asked: d.preparedAsked(),
      text: (n) => needText(n, 'magia', 'magias', 'preparar'),
    }),
  ];
}

/** A swap with only one of its two picks is a choice left half done; the panel is in the ability step or, without it, in "Escolhas". */
function swapMissing(d: MissingSource): Missing[] {
  const names = d.swap.halfGroups().map((g) => g.namePt);
  return names.length > 0
    ? [
        {
          step: d.options.abilityScoreImprovement ? 'abilities' : 'picks',
          id: 'swap',
          text: `Falta escolher o que trocar em ${LIST.format(names)}.`,
        },
      ]
    : [];
}

/** Every choice still to make, in step order, each with the picker that holds it. */
export function missingChoices(d: MissingSource): Missing[] {
  const o = d.options;
  return [
    ...abilityMissing(d),
    ...(d.hpCard() === 'roll' && d.rolled() === null
      ? [{ step: 'hp' as const, id: 'hp', text: 'Falta rolar o dado de vida.' }]
      : []),
    ...openChoicesMissing(d),
    ...(o.subclassDue && d.subclassKey() === ''
      ? [{ step: 'picks' as const, id: 'subclass', text: 'Falta escolher a subclasse.' }]
      : []),
    ...featureMissing(d),
    ...skillsMissing(d),
    ...(d.instrumentAsked() > 0 && d.instrument() === ''
      ? [{ step: 'picks' as const, id: 'instrument', text: 'Falta escolher o instrumento.' }]
      : []),
    ...expertiseMissing(d),
    ...spellsMissing(d),
    ...swapMissing(d),
  ];
}
