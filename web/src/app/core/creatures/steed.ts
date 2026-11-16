import { Code, ConnectError } from '@connectrpc/connect';

import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import type {
  CharacterCreature,
  SummonSpellOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { sessionClosed } from '../combat/combat-errors';
import { circleLabel } from '../combat/combat-grid';
import { joinDots, tieNumbers } from '../format/text';
import { metersText } from '../units';
import { creatureErrorMessage } from './creature-errors';
import { CREATURE_TYPES } from './creature-types';

/** Convocar Montaria's key: the summon whose creature is the character's bonded steed (#588). */
export const FIND_STEED = 'spell:find-steed';

/** The warning of a form the caster cannot ride (`SummonOption.too_small_for_you`): the cast still works. */
export const STEED_TOO_SMALL = 'Pequeno demais para você montar';

/** "Tipo do corcel": the types the server lets the cast pick, with the app's words (Celestial, Fada, Ínfero). */
export function steedTypes(
  spell: SummonSpellOptions,
): readonly { readonly value: string; readonly label: string }[] {
  return spell.typeChoices.map((value) => ({
    value,
    label: CREATURE_TYPES.find((t) => t.value === value)?.label ?? value,
  }));
}

/** "celestial", "fada", "ínfero" at the end of a steed's subtitle; `''` for no type, or one the app has no word for. */
export function steedTypeWord(type: string): string {
  return CREATURE_TYPES.find((t) => t.value === type)?.label.toLowerCase() ?? '';
}

/** What "Como funciona" folds: the spell's notes, then the least Intelligence the steed has. */
export function steedHowLines(spell: SummonSpellOptions): readonly string[] {
  return [
    ...spell.notes,
    ...(spell.minInt > 0 ? [`A Inteligência dele é no mínimo ${spell.minInt}.`] : []),
  ];
}

/** The cast button: "Convocar Estrela" for the bonded steed, "Convocar Cavalo de guerra" for a form, or the plain verb. */
export function steedCastVerb(bondedName: string, formName: string): string {
  return `Convocar ${bondedName || formName || 'a montaria'}`;
}

/** What the cast spends and when it can be made: it takes 10 minutes, so never in a combat. */
export function steedCostLine(level: number): string {
  return `Gasta um espaço de ${circleLabel(level)} ou maior. Só durante uma sessão, fora de combate.`;
}

/** Under the fixed row of a bonded steed ("Mesmo corcel"): board 588-22's words. */
export function steedSameLine(name: string): string {
  return `${name} volta como era: a mesma forma, o mesmo tipo, e os PV cheios. Para outra forma ou outro tipo, liberte ${name} antes, no bloco Criaturas.`;
}

/** The end of the sheet's subtitle: "Magia de 2º nível · 10 minutos · um corcel vinculado". */
export function steedSubtitleTail(bonded: boolean): string {
  return bonded ? 'o seu corcel vinculado' : 'um corcel vinculado';
}

/** The slot, folded from the start (the steed is one creature): "Espaço de 2º nível · 2 livres". */
export function steedSetupLine(level: number, free: number): string {
  return joinDots([
    tieNumbers(`Espaço de ${circleLabel(level)}`),
    `${free} ${free === 1 ? 'livre' : 'livres'}`,
  ]);
}

/** A form's line in the steed's list: "Grande · 18 m · CA 11 · PV 19". */
export function steedFormLine(block: Creature): string {
  return joinDots([
    block.summary?.sizePt ?? '',
    tieNumbers(metersText(block.speedWalkFt)),
    tieNumbers(`CA ${block.armorClass}`),
    tieNumbers(`PV ${block.hitPoints}`),
  ]);
}

/** The fixed row's second line: "Cavalo de guerra · celestial · Grande · 18 m · CA 11 · PV 19"; without the stat block, what the steed's own row carries. */
export function steedFixedLine(steed: CharacterCreature, block: Creature | null): string {
  return joinDots(
    [
      steed.monsterNamePt,
      steedTypeWord(steed.creatureType),
      ...(block
        ? [steedFormLine(block)]
        : [tieNumbers(`CA ${steed.armorClass}`), tieNumbers(`PV ${steed.hitPointsMax}`)]),
    ].filter((part) => part !== ''),
  );
}

/** A refused cast of Convocar Montaria, in board 588-26's words: the two refusals of a ten-minute casting, the choice a bonded steed does not take, and a call that did not arrive. */
export function steedCastError(err: unknown, bondedName = ''): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.FailedPrecondition) {
    if (sessionClosed(err)) {
      return 'Agora não há sessão aberta.';
    }
    const reason = connectErr.findDetails(EncounterBlockedSchema)[0]?.reason;
    if (reason === EncounterBlockedReason.SUMMON_IN_COMBAT) {
      return 'Leva 10 minutos: não dá durante um combate.';
    }
    if (reason === EncounterBlockedReason.SUMMON_CHOICE_INVALID && bondedName) {
      return `${bondedName} já tem forma e tipo. Para escolher outros, liberte ${bondedName} antes.`;
    }
  }
  if (connectErr.code === Code.Unavailable || connectErr.code === Code.DeadlineExceeded) {
    return 'Não deu para convocar agora. Nada foi gasto. Tente de novo.';
  }
  return creatureErrorMessage(err, 'cast');
}
