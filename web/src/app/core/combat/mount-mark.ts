import type { Combatant, Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { theName } from '../format/article';
import { isPlayer } from './combat-view';

/** `Combatant.mount_kind` and `SetCombatantMountRequest.kind`: `''` takes the mark off. */
export type MountKind = '' | 'controlled' | 'independent';

export interface MountMarkOption {
  readonly kind: MountKind;
  readonly label: string;
  /** What the mark means, in one line. */
  readonly hint: string;
  readonly disabled: boolean;
}

/** The master's "Montaria" select of a row. */
export interface MountMarkView {
  readonly current: MountKind;
  readonly options: readonly MountMarkOption[];
  /** The line under the select: what the current mark means, naming the rider it carries. */
  readonly hint: string;
  /** Why "Nenhuma" is off: "Só depois que Tor desmontar."; `''` when it is not. */
  readonly why: string;
  /** With no mark yet: what marking does ("Com a marca, o Mastim aceita um cavaleiro…"); `''` once it is marked. */
  readonly note: string;
}

/** The mark is for an NPC or a creature: a player's character is never a marked mount. */
export function canMarkMount(c: Combatant): boolean {
  return !isPlayer(c);
}

function hintOf(kind: MountKind, rider: string): string {
  if (kind === 'controlled') {
    return `Age logo depois de ${rider || 'quem monta'} e só usa Disparada, Desengajar e Esquivar.`;
  }
  return kind === 'independent'
    ? `Mantém a própria vez e leva ${rider || 'quem monta'}.`
    : 'Não aceita cavaleiro.';
}

function noteOf(label: string): string {
  return `Com a marca, ${theName(label)} aceita um cavaleiro, que monta na própria vez. Você monta qualquer criatura.`;
}

/** `null` for a combatant that takes no mark, or is not in the combat. The server decides every refusal. */
export function mountMarkView(e: Encounter, id: string): MountMarkView | null {
  const c = e.combatants.find((x) => x.id === id);
  if (!c || !canMarkMount(c)) {
    return null;
  }
  const rider = c.riderId ? (e.combatants.find((x) => x.id === c.riderId)?.label ?? '') : '';
  const current: MountKind =
    c.mountKind === 'controlled' || c.mountKind === 'independent' ? c.mountKind : '';
  const option = (kind: MountKind, label: string): MountMarkOption => ({
    kind,
    label,
    hint: hintOf(kind, ''),
    // The mark does not come off a mount that carries a rider (MOUNT_TAKEN).
    disabled: kind === '' && !!c.riderId,
  });
  return {
    current,
    options: [
      option('', 'Nenhuma'),
      option('controlled', 'Adestrada'),
      option('independent', 'Independente'),
    ],
    hint: hintOf(current, rider),
    why: c.riderId ? `Só depois que ${rider || 'o cavaleiro'} desmontar.` : '',
    note: current === '' ? noteOf(c.label) : '',
  };
}
