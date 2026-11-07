import { CreatureSource } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  Ability,
  type Creature,
  type CreatureSummary,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots, tieNumbers, tight } from '../format/text';
import { metersText } from '../units';

/** How the table says where a creature came from, under its name: "Familiar de Pensantus". */
export function sourcePhrase(source: CreatureSource, ownerName: string): string {
  switch (source) {
    case CreatureSource.FAMILIAR:
      return `Familiar de ${ownerName}`;
    case CreatureSource.ANIMATE_DEAD:
      return `Animado por ${ownerName}`;
    case CreatureSource.CONJURE_ANIMALS:
    case CreatureSource.TABLE:
      return `Conjurado por ${ownerName}`;
    case CreatureSource.MASTER:
      return 'Dado pelo mestre';
    default:
      return `Criatura de ${ownerName}`;
  }
}

/** A table creature's key ends with "@mesa" (the entry's own); the server sends no flag for it. */
export function isTableCreature(monsterKey: string): boolean {
  return monsterKey.endsWith('@mesa');
}

/** The creature's page says whose numbers a table creature has: the owner's sheet, read when it arrived; "do dono" while the name is not known. */
export function tableCreatureNote(ownerName: string): string {
  const owner = ownerName.trim();
  return `Os números vêm da ficha ${owner ? `de ${owner}` : 'do dono'} quando ele chegou`;
}

/** The same note in the choice list of the one who summons: the numbers are their own. */
export const TABLE_CREATURE_PICK_NOTE = 'Os números vêm da sua ficha quando ele chega.';

/** The short origin of the master's list: "familiar", "dado pelo mestre". */
export function sourceShort(source: CreatureSource): string {
  switch (source) {
    case CreatureSource.FAMILIAR:
      return 'familiar';
    case CreatureSource.ANIMATE_DEAD:
      return 'animado';
    case CreatureSource.CONJURE_ANIMALS:
    case CreatureSource.TABLE:
      return 'conjurado';
    default:
      return 'dado pelo mestre';
  }
}

/** The speeds as the card and the form list say them: "3 m, voo 15 m", "nada 9 m", "1,5 m, voo 18 m". */
export function speedsText(
  c: Pick<
    Creature,
    'speedWalkFt' | 'speedFlyFt' | 'speedSwimFt' | 'speedClimbFt' | 'speedBurrowFt'
  >,
): string {
  const parts: string[] = [];
  if (c.speedWalkFt > 0) {
    parts.push(metersText(c.speedWalkFt));
  }
  const others: [number, string][] = [
    [c.speedClimbFt, 'escala'],
    [c.speedSwimFt, 'nada'],
    [c.speedFlyFt, 'voo'],
    [c.speedBurrowFt, 'escava'],
  ];
  for (const [ft, word] of others) {
    if (ft > 0) {
      parts.push(tight(`${word} ${metersText(ft)}`));
    }
  }
  return parts.join(', ') || '0 m';
}

/** The challenge rating as the list says it: "ND 1/8". */
export function challengeText(cr: string): string {
  return `ND ${cr}`;
}

/** A form's line in a choice list: "Miúdo · 1 PV · 6 m, escala 6 m". */
export function formSubtitle(c: Creature): string {
  return joinDots([c.summary?.sizePt ?? '', tight(`${c.hitPoints} PV`), speedsText(c)]);
}

/** A catalog row's line: "Médio · fera · ND 1/8", with the AC and hit points when the stat block is known. */
export function summarySubtitle(s: CreatureSummary, c?: Creature): string {
  const parts = [s.sizePt, s.typePt, challengeText(s.challengeRating)];
  if (c) {
    parts.push(tieNumbers(`CA ${c.armorClass}`), tieNumbers(`PV ${c.hitPoints}`));
  }
  return joinDots(parts);
}

/** An ability modifier with the true minus sign, as the sheet writes it: "+2", "−4". */
export function signed(n: number): string {
  return n >= 0 ? `+${n}` : `−${-n}`;
}

/** "2 de 40", the counter of a name field. */
export function nameCounter(length: number, max: number): string {
  return `${length} de ${max}`;
}

/** The longest name a creature may have (`RenameCreature`, `GiveCreature`). */
export const CREATURE_NAME_MAX = 40;

const ABILITY_PT: Partial<Record<Ability, string>> = {
  [Ability.STRENGTH]: 'Força',
  [Ability.DEXTERITY]: 'Destreza',
  [Ability.CONSTITUTION]: 'Constituição',
  [Ability.INTELLIGENCE]: 'Inteligência',
  [Ability.WISDOM]: 'Sabedoria',
  [Ability.CHARISMA]: 'Carisma',
};

/** A beast's line in Wild Shape's list: "Médio · ND 1/4 · 12 m · CA 13 · PV 11"; only the book's summary until its stat block is read. */
export function beastLine(s: CreatureSummary, c?: Creature): string {
  const parts = [s.sizePt, challengeText(s.challengeRating)];
  if (c) {
    parts.push(speedsText(c), tieNumbers(`CA ${c.armorClass}`), tieNumbers(`PV ${c.hitPoints}`));
  }
  return joinDots(parts);
}

/** One attack of a stat block as a line: its name, the numbers the book gives and the book's own text (English). */
export interface AttackLine {
  readonly name: string;
  /** The name is the SRD's English one (the content has no Portuguese name for it): the page marks it `lang="en"`. */
  readonly english: boolean;
  /** "+4 · 2d4 + 2 perfurante · Força CD 11": the numbers are the book's. */
  readonly detail: string;
  /** The SRD's whole text of the action ("... or be knocked prone."), English, never translated here. */
  readonly text: string;
}

/** What a beast's attacks do, from its stat block: each with its name (Portuguese when the content has it) and the book's text. Empty when it has none. */
export function beastAttacks(c: Creature): readonly AttackLine[] {
  return c.actions
    .filter((a) => a.hasAttack)
    .map((a) => {
      const damage = a.damage
        .map((d) =>
          `${d.dice
            .replace(/([+-])/g, ' $1 ')
            .replace(/\s+/g, ' ')
            .trim()} ${d.damageTypePt}`.trim(),
        )
        .join(' + ');
      // "Força CD 11" as one block: a line never breaks inside it.
      const save = a.save ? ` · ${ABILITY_PT[a.save.ability] ?? ''}\u00a0CD\u00a0${a.save.dc}` : '';
      return {
        name: a.namePt || a.name,
        english: !a.namePt,
        detail: tight(joinDots([signed(a.attackBonus), damage]) + save),
        text: a.text,
      };
    });
}
