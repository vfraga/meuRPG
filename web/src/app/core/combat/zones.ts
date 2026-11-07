import {
  type Combatant,
  type Encounter,
  type SpellZone,
  SpellZoneMovement,
  SpellZoneVisibility,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { joinDots, upFirst } from '../format/text';
import { abilityName } from '../effects/ability-choice';
import { metersText } from '../units';
import { shapeText } from './area-text';
import type { Square } from './combat-grid';

/**
 * What the combat screen says about a spell zone (RN-37): the pure view of `Encounter.zones`, tested without a DOM.
 * The client never infers or fills what the server left out: a zone with `detailsHidden` reads name, area, caster,
 * concentration and occupants, even if a field slipped through, and a zone the server did not send is not drawn.
 */

/** Who reads the zones: the master sees every zone; a player's combatants decide the occupants they may name. */
export interface ZoneViewer {
  readonly master: boolean;
  readonly combatants: readonly Combatant[];
}

export interface ZoneView {
  readonly id: string;
  readonly spellKey: string;
  /** "Raio Lunar": the spell alone, for the map's legend. */
  readonly spellName: string;
  /** "Raio Lunar · Bartolo". */
  readonly label: string;
  /** The pieces of the detail line, in the order the board reads them. */
  readonly pieces: readonly string[];
  /** The pieces joined with a middle dot. */
  readonly detail: string;
  /** "restam 9 rodadas", "restam 59 min"; empty with no count (or hidden). */
  readonly roundsLeft: string;
  /** "Concentração de Bartolo"; empty for a spell that does not need it. */
  readonly concentration: string;
  /** "Aqui: Tor, Jabuti"; empty with nobody the viewer may name. */
  readonly occupants: string;
  /** "Só você vê" for a zone of a hidden caster, else empty. */
  readonly masterOnly: string;
  /** The master's call for this spell (RN-37), one line; empty for a player or a spell with none. */
  readonly reminder: string;
  /** The map draws the outline dashed (the zone of a hidden caster). */
  readonly dashed: boolean;
  readonly detailsHidden: boolean;
  readonly canMove: boolean;
  readonly canEnd: boolean;
  readonly origin: Square | null;
  readonly squares: readonly Square[];
}

/** The master's calls the spells leave to the table: one line each, for the master's card only. */
const CALL_WORDS: Readonly<Record<string, string>> = {
  master_light:
    'Você decide: a luz de uma magia de nível 2 ou menor que toque a Escuridão é dissipada.',
  master_wind: 'Você decide: um vento forte dispersa a Névoa em 1 rodada.',
  master_breath: 'Você decide: quem não precisa respirar passa no teste sem rolar.',
};

const NOTE_WORDS: Readonly<Record<string, string>> = {
  restrains: 'contido',
  difficult_terrain: 'terreno difícil',
  speed_halved: 'velocidade pela metade',
};

/** A round is 6 seconds, so ten rounds are a minute: under ten the zone counts rounds, from ten minutes. */
export function roundsLeftText(rounds: number): string {
  if (rounds <= 0) {
    return '';
  }
  if (rounds === 1) {
    return 'resta 1 rodada';
  }
  return rounds < 10 ? `restam ${rounds} rodadas` : `restam ${Math.floor(rounds / 10)} min`;
}

/** "Cilindro de 1,5 m de raio": the shape of the area in words (the same phrase as the area picker's). */
function shapeWords(zone: SpellZone): string {
  return upFirst(shapeText({ shape: zone.shape, sizeFt: zone.sizeFt, widthFt: 0 }));
}

function shapeAndMove(zone: SpellZone): string {
  const shape = shapeWords(zone);
  return zone.movesPerTurnFt > 0
    ? `${shape}, anda ${metersText(zone.movesPerTurnFt)} por turno`
    : shape;
}

function saveText(zone: SpellZone): string {
  if (zone.saveAbility === '') {
    return '';
  }
  const name = abilityName(zone.saveAbility, zone.saveAbilityNamePt);
  return zone.saveDc === undefined ? name : `${name} CD ${zone.saveDc}`;
}

function damageText(zone: SpellZone): string {
  if (zone.damageDice === '') {
    return '';
  }
  const base = zone.damageTypeNamePt
    ? `${zone.damageDice} ${zone.damageTypeNamePt}`
    : zone.damageDice;
  if (zone.saveAbility === '') {
    return `${base}, sem teste`;
  }
  return zone.halfOnSave ? `${base}, metade se passar` : base;
}

function pieces(zone: SpellZone): string[] {
  if (zone.detailsHidden) {
    return [shapeWords(zone)];
  }
  const notes = zone.notes.flatMap((n) => NOTE_WORDS[n] ?? []);
  return [shapeAndMove(zone), saveText(zone), damageText(zone), ...notes].filter((p) => p !== '');
}

function occupantsText(zone: SpellZone, viewer: ZoneViewer): string {
  const names = zone.occupantIds.flatMap((id) => {
    const c = viewer.combatants.find((x) => x.id === id);
    return c && (viewer.master || !c.hidden) ? [c.label] : [];
  });
  return names.length > 0 ? `Aqui: ${names.join(', ')}` : '';
}

/** The view of a zone for `viewer`. */
export function zoneView(zone: SpellZone, viewer: ZoneViewer): ZoneView {
  const parts = pieces(zone);
  const masterOnly = zone.visibility === SpellZoneVisibility.MASTER_ONLY;
  return {
    id: zone.id,
    spellKey: zone.spellKey,
    spellName: zone.spellNamePt,
    label: joinDots([zone.spellNamePt, zone.ownerLabel]),
    pieces: parts,
    detail: joinDots(parts),
    roundsLeft: zone.detailsHidden ? '' : roundsLeftText(zone.roundsLeft),
    concentration: zone.concentration ? `Concentração de ${zone.ownerLabel}` : '',
    occupants: occupantsText(zone, viewer),
    masterOnly: masterOnly ? 'Só você vê' : '',
    reminder:
      viewer.master && !zone.detailsHidden
        ? (zone.notes.flatMap((n) => CALL_WORDS[n] ?? [])[0] ?? '')
        : '',
    dashed: masterOnly,
    detailsHidden: zone.detailsHidden,
    // The server lets the caster's player move it too; the screen offers it to the master only (RN-37: the
    // master moves any zone, with no turn or action to spend).
    canMove: viewer.master && zone.movement === SpellZoneMovement.ACTION,
    canEnd: viewer.master,
    origin: zone.origin ? { col: zone.origin.col, row: zone.origin.row } : null,
    squares: zone.squares.map((s) => ({ col: s.col, row: s.row })),
  };
}

/** The zones of the combat on screen for `master` or a player, in the order the server sent (the order they were cast). */
export function zoneViews(encounter: Encounter | null, master: boolean): ZoneView[] {
  if (!encounter) {
    return [];
  }
  const viewer: ZoneViewer = { master, combatants: encounter.combatants };
  return encounter.zones.map((z) => zoneView(z, viewer));
}
