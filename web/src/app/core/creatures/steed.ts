import { Code, ConnectError } from '@connectrpc/connect';

import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { sessionClosed } from '../combat/combat-errors';
import { circleLabel } from '../combat/combat-grid';
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

/** Under the fixed row of a bonded steed ("Mesmo corcel"). */
export function steedSameLine(name: string): string {
  return `${name} volta como era, com todos os pontos de vida. Para outra forma ou outro tipo, liberte ${name} antes, no bloco Criaturas.`;
}

/** A refused cast of Convocar Montaria: the two refusals of a ten-minute casting in its own words, the rest as any summon. */
export function steedCastError(err: unknown): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.FailedPrecondition) {
    if (sessionClosed(err)) {
      return 'Agora não há sessão aberta.';
    }
    const reason = connectErr.findDetails(EncounterBlockedSchema)[0]?.reason;
    if (reason === EncounterBlockedReason.SUMMON_IN_COMBAT) {
      return 'Leva 10 minutos: não dá durante um combate.';
    }
  }
  return creatureErrorMessage(err, 'cast');
}
