import {
  ActionEconomy,
  SpellDamageChoice,
  type SpellDetails,
  type SpellOption,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { needsAbility } from '../effects/ability-choice';

/**
 * The spells that ask the caster for a choice when they are cast (#857): an ability (Enhance Ability), the creatures of a summon
 * (Conjure Animals) or a damage type (Flame Strike). The cast sheet asks them with its pickers and sends `ability_key`, `summon` and
 * `damage_type_key`; the release of a readied spell does not send them yet, so the Ready sheet leaves these spells off. Each kind is
 * known the way the cast sheet knows it: the ability by `needsAbility`, the damage type by the spell's details (`damage_choice` with
 * two or more damages), the summon by the character's summon options (`GetSummonOptions`). No spell key is listed here. A revival
 * (Revivify) is not here: nothing in the spell details or the turn options says a spell revives, so the server refuses it at Ready
 * (`READY_SPELL_NOT_RELEASABLE`) and the sheet shows that sentence.
 */

/** What a spell asks when it is cast. */
export type CastChoice = 'ability' | 'summon' | 'damage';

/** The words after "ela pede" in the card of a spell the window cannot release: what it asks of the caster. */
export const CHOICE_ASKS: Readonly<Record<CastChoice, string>> = {
  ability: 'a habilidade',
  summon: 'as criaturas',
  damage: 'o tipo de dano',
};

/** Whether the spell lets the caster pick its damage type: a choice among two or more damages (as the cast sheet's picker is shown). */
export function asksDamageType(d: SpellDetails | null): boolean {
  return !!d && d.damageChoice !== SpellDamageChoice.UNSPECIFIED && d.damage.length > 1;
}

/** What the spell asks when it is cast, `null` when nothing. `summons` holds the keys of the spells that summon. */
export function castChoiceOf(
  key: string,
  details: SpellDetails | null,
  summons: ReadonlySet<string>,
): CastChoice | null {
  if (needsAbility(key)) {
    return 'ability';
  }
  if (summons.has(key)) {
    return 'summon';
  }
  return asksDamageType(details) ? 'damage' : null;
}

/** What the sheet reads to tell the choices: the details of a spell (`GetSpellDetails`) and the keys of the spells that summon. */
export interface ChoiceReads {
  details(key: string): Promise<SpellDetails | null>;
  summons(): Promise<ReadonlySet<string>>;
}

/**
 * The choice each spell the Ready sheet could offer asks (only the ones that ask are in the map). A read that fails leaves the spell
 * as it is: Ready itself refuses, with its reason, a spell whose release cannot carry what it needs.
 */
export async function readyChoices(
  spells: readonly SpellOption[],
  reads: ChoiceReads,
): Promise<ReadonlyMap<string, CastChoice>> {
  const held = spells.filter(
    (o) => o.spell && !o.grant && o.enabled && o.economy === ActionEconomy.ACTION,
  );
  const summons = await reads.summons().catch(() => new Set<string>());
  const found = await Promise.all(
    held.map(async (o) => {
      const key = o.spell!.key;
      const details = needsAbility(key) ? null : await reads.details(key).catch(() => null);
      return [key, castChoiceOf(key, details, summons)] as const;
    }),
  );
  return new Map(found.filter((f): f is [string, CastChoice] => f[1] !== null));
}
