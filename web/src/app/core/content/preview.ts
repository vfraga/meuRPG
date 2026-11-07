import { create } from '@bufbuild/protobuf';

import {
  TableBackgroundSchema,
  TableClassFeatureSchema,
  TableContentKind,
  TableCreatureSchema,
  TableFeatSchema,
  TableOptionSchema,
  TableEntrySchema,
  TableRaceSchema,
  TableSpellSchema,
  TableSubraceSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readEntry } from './content-read';
import { type EntryRead, type NameOf, type ParentView } from './content-read-parts';
import type { CreatureInit } from './creature-draft';
import type { FeatInit } from './feat-draft';
import type { ClassFeatureInit } from './class-feature-draft';
import type { OptionInit } from './option-draft';
import type { OfferedByName } from './option-text';
import type { BackgroundInit, RaceInit, SubraceInit } from './feature-draft';
import type { SpellInit } from './spell-types';

/**
 * "Como os jogadores veem" (E10-01): the editor's preview is what a player will read, so it is written by the same function
 * (`readEntry`) from the entry the form would send, effects and all. Nothing here is a second way to write a spell or a race.
 */
export type PreviewBody =
  | { readonly case: 'tableSpell'; readonly value: SpellInit }
  | { readonly case: 'tableRace'; readonly value: RaceInit }
  | { readonly case: 'tableSubrace'; readonly value: SubraceInit }
  | { readonly case: 'tableBackground'; readonly value: BackgroundInit }
  | { readonly case: 'tableFeat'; readonly value: FeatInit }
  | { readonly case: 'tableOption'; readonly value: OptionInit }
  | { readonly case: 'tableClassFeature'; readonly value: ClassFeatureInit }
  | { readonly case: 'tableCreature'; readonly value: CreatureInit };

/** `parent` is the race of a subrace: what its traits add to the preview. */
export function previewRead(
  body: PreviewBody,
  nameOf: NameOf,
  parent?: ParentView,
  offeredByName?: OfferedByName,
): EntryRead {
  switch (body.case) {
    case 'tableSpell':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.SPELL,
          body: { case: 'tableSpell', value: create(TableSpellSchema, body.value) },
        }),
        nameOf,
      );
    case 'tableRace':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.RACE,
          body: { case: 'tableRace', value: create(TableRaceSchema, body.value) },
        }),
        nameOf,
      );
    case 'tableSubrace':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.SUBRACE,
          body: { case: 'tableSubrace', value: create(TableSubraceSchema, body.value) },
        }),
        nameOf,
        undefined,
        () => parent ?? { traits: [] },
      );
    case 'tableBackground':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.BACKGROUND,
          body: { case: 'tableBackground', value: create(TableBackgroundSchema, body.value) },
        }),
        nameOf,
      );
    case 'tableFeat':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.FEAT,
          body: { case: 'tableFeat', value: create(TableFeatSchema, body.value) },
        }),
        nameOf,
      );
    case 'tableOption':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.OPTION,
          body: { case: 'tableOption', value: create(TableOptionSchema, body.value) },
        }),
        nameOf,
        () => 0,
        undefined,
        offeredByName,
      );
    case 'tableClassFeature':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.CLASS_FEATURE,
          body: {
            case: 'tableClassFeature',
            value: create(TableClassFeatureSchema, body.value),
          },
        }),
        nameOf,
      );
    case 'tableCreature':
      return readEntry(
        create(TableEntrySchema, {
          kind: TableContentKind.CREATURE,
          body: { case: 'tableCreature', value: create(TableCreatureSchema, body.value) },
        }),
        nameOf,
      );
  }
}
