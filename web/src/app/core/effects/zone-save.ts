import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import type { EffectSavePrompt } from '../../../gen/meurpg/play/v1/lasting_effects_pb';

/**
 * The words of the saving throw a spell zone asks (Moonbeam, Web, Spirit Guardians): pure, built from the prompt the server sent.
 * The server sends the DC only to the master and the caster's own player, so a number is written only when `dc` is there; the
 * client never fills it.
 */

/** The HEADER of the sheet: what kind of save this is, before the reason. */
export const ZONE_SAVE_HEADER = 'Magia que ficou no mapa';

/** Why the creature is asked, for who answers (`who` empty: "você") or for the master reading about `who`. Empty for a reason the client does not know. */
export function zoneReasonText(reason: string, who: string): string {
  if (reason === 'zone_enter') {
    return who ? `${who} entrou na zona.` : 'Você entrou na zona.';
  }
  if (reason === 'zone_start') {
    return who
      ? `A zona atingiu ${who} no começo do turno.`
      : 'A zona atingiu você no começo do seu turno.';
  }
  if (reason === 'zone_cast') {
    return who
      ? `${who} estava na área quando a magia foi conjurada.`
      : 'Você estava na área quando a magia foi conjurada.';
  }
  if (reason === 'zone_end') {
    return who ? `${who} terminou o turno na zona.` : 'Você terminou o turno na zona.';
  }
  return '';
}

/** "Raio Lunar de Bartolo"; the zone alone when its caster is not among the zones the viewer was sent. */
export function zoneSaveTitle(prompt: EffectSavePrompt, caster: string): string {
  return caster ? `${prompt.sourceNamePt} de ${caster}` : prompt.sourceNamePt;
}

/** The caster of a zone, from the zones the server sent the viewer; empty when the zone is not among them. */
export function zoneCasterLabel(encounter: Encounter | null, zoneId: string): string {
  return encounter?.zones.find((z) => z.id === zoneId)?.ownerLabel ?? '';
}

/** What a failure and a pass do: the damage type and "metade do dano" from the prompt's fields; a zone with no damage (the web)
 * says what its text says, and "Soltar-se gasta uma ação" is a static line the server sends nowhere. */
export function zoneConsequence(prompt: EffectSavePrompt, textAfterAbility: string): string {
  if (prompt.damageTypeNamePt) {
    const pass = prompt.halfOnSave ? 'metade do dano.' : 'não sofre o dano.';
    return `Se falhar: sofre dano ${prompt.damageTypeNamePt}. Se passar: ${pass}`;
  }
  return /contid[oa]/.test(textAfterAbility)
    ? `${textAfterAbility} Soltar-se gasta uma ação.`
    : textAfterAbility;
}

/** The words a zone's save adds to the sheet in place of an effect's: title, subtitle, what a failure does, the DC and the source line. */
export function zoneSheetWords(prompt: EffectSavePrompt, caster: string, textAfterAbility: string) {
  return {
    title: zoneSaveTitle(prompt, caster),
    subtitle: `${ZONE_SAVE_HEADER}. ${zoneReasonText(prompt.zoneReason, '')}`.trim(),
    consequence: zoneConsequence(prompt, textAfterAbility),
    source: caster ? `${prompt.sourceNamePt}, de ${caster}` : '',
  };
}
