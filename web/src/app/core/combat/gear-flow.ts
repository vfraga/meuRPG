import { effect, signal, untracked } from '@angular/core';

import type { GearClient, GearVm } from '../characters/gear';
import { combatErrorMessage } from './combat-errors';

/** What `GearFlow` reads from the screen it serves. */
export interface GearFlowContext {
  readonly client: Pick<GearClient, 'read' | 'set'>;
  readonly campaignId: () => string;
  /** The character of the screen's player; empty when the screen has none (the master's, a creature's). */
  readonly characterId: () => string;
  /** What of the encounter the gear shows or spends (the own combatant's turn economy): the gear is read again only
   * when it changes, so an attack of someone else costs no read. */
  readonly signature: () => string;
}

/** The turn's economy as the gear rows need it. */
export interface GearTurn {
  readonly objectInteractionUsed: boolean;
  readonly actionUsed: boolean;
  /** An extra action (Velocidade) is free and may be "Usar um objeto" (the server counts it when the action is used). */
  readonly extraObject: boolean;
}

/** What a change of this piece costs now (SRD 5.1, "Other Activity on Your Turn"): the first weapon of the turn is the
 * free object interaction, everything else is an action (the extra one, when only it is left). The server charges it and
 * has the last word; the word only tells beforehand. */
export function gearCost(piece: GearVm, turn: GearTurn): string {
  if (piece.kind === 'weapon' && !turn.objectInteractionUsed) {
    return 'grátis';
  }
  return turn.actionUsed && turn.extraObject ? 'ação extra' : 'ação';
}

/** Why a change cannot be made now, before the click; empty when it can. */
export function gearBlocked(piece: GearVm, turn: GearTurn): string {
  if (gearCost(piece, turn) !== 'grátis' && turn.actionUsed && !turn.extraObject) {
    return 'Você já usou a ação deste turno.';
  }
  return '';
}

/** What the rule line of the combat's fold says (SRD 5.1, "Other Activity on Your Turn" and "Armor"). */
export const GEAR_RULE =
  '1ª arma do turno: grátis; depois, a ação. Escudo: ação. Armadura: só fora do combate.';

/** The armor class the shield would give back: the two-handed weapon in hand, stowed (the server's number); null when
 * the shield counts. */
export function shieldBackAc(gear: readonly GearVm[], shieldIdle: boolean): number | null {
  if (!shieldIdle) {
    return null;
  }
  const hand = gear.find((g) => g.kind === 'weapon' && g.equipped && g.twoHanded);
  return hand && hand.armorClassIfToggled > 0 ? hand.armorClassIfToggled : null;
}

/** The warning before a drawing that changes the armor class: a two-handed weapon in the pack, the shield in hand. */
export function twoHandedWarnings(
  gear: readonly GearVm[],
  armorClass: number | null,
): { key: string; text: string }[] {
  if (armorClass === null || !gear.some((g) => g.kind === 'shield' && g.equipped)) {
    return [];
  }
  return gear
    .filter((g) => g.kind === 'weapon' && g.twoHanded && !g.equipped)
    .filter((g) => g.armorClassIfToggled > 0 && g.armorClassIfToggled !== armorClass)
    .map((g) => ({
      key: g.key,
      text: `Sacar ${g.namePt} (duas mãos) deixa o escudo de lado: CA ${armorClass} → ${g.armorClassIfToggled}.`,
    }));
}

/**
 * The player's gear in a combat: the list the server derives, "Equipar" / "Guardar" and what the server answers. The
 * economy it spends (the free object interaction, the action) is the combat's: the stream brings it back. The gear is read
 * when the screen's character is known and again when the own turn's economy changes or the fold opens (`setOpen`),
 * never for the others' moves and never while the fold is closed; a read that fails says so (`readFailed`) and
 * `retry()` asks again.
 */
export class GearFlow {
  readonly gear = signal<readonly GearVm[]>([]);
  readonly armorClass = signal<number | null>(null);
  readonly shieldIdle = signal(false);
  readonly busyKey = signal<string | null>(null);
  readonly notice = signal('');
  readonly error = signal('');
  /** The last read did not come back: what is shown (if anything) may be old. */
  readonly readFailed = signal(false);
  private readonly open = signal(true);
  private loadedFor = '';
  private seq = 0;

  /** Built where Angular can run an effect (a field of the screen). */
  constructor(private readonly ctx: GearFlowContext) {
    effect(() => {
      ctx.characterId();
      ctx.signature();
      if (this.open()) {
        untracked(() => void this.load());
      }
    });
  }

  /** The fold's open state: a closed fold reads nothing; opening it reads what moved meanwhile. */
  setOpen(open: boolean): void {
    this.open.set(open);
  }

  /** Reads the gear of the screen's character; a change in flight reads it again itself. */
  async load(): Promise<void> {
    const characterId = this.ctx.characterId();
    if (!characterId) {
      return;
    }
    if (this.loadedFor !== characterId) {
      this.loadedFor = characterId;
      this.gear.set([]);
      this.armorClass.set(null);
      this.shieldIdle.set(false);
      this.notice.set('');
      this.error.set('');
    }
    if (this.busyKey() === null) {
      await this.read(characterId);
    }
  }

  /** "Tentar de novo": the read that failed, asked again. */
  async retry(): Promise<void> {
    this.error.set('');
    await this.load();
  }

  private async read(characterId: string): Promise<boolean> {
    const seq = ++this.seq;
    try {
      const sheet = await this.ctx.client.read(this.ctx.campaignId(), characterId);
      if (seq !== this.seq || characterId !== this.ctx.characterId()) {
        return false; // a newer read, or another character, owns the screen
      }
      this.gear.set(sheet.gear);
      this.armorClass.set(sheet.armorClass);
      this.shieldIdle.set(sheet.shieldIdle);
      this.readFailed.set(false);
      return true;
    } catch {
      if (seq === this.seq) {
        this.readFailed.set(true); // keeps what is shown; the fold offers "Tentar de novo"
      }
      return false;
    }
  }

  /** "Equipar" / "Guardar": the server's answer is the refusal in words, or the new list. `true` when it changed. */
  async toggle(key: string, equipped: boolean): Promise<boolean> {
    const characterId = this.ctx.characterId();
    if (!characterId || this.busyKey() !== null) {
      return false;
    }
    const name = this.gear().find((g) => g.key === key)?.namePt ?? '';
    this.busyKey.set(key);
    this.error.set('');
    this.notice.set('');
    try {
      await this.ctx.client.set(this.ctx.campaignId(), characterId, key, equipped);
      const fresh = await this.read(characterId);
      if (characterId !== this.ctx.characterId()) {
        return false;
      }
      if (!fresh) {
        this.error.set(
          'A troca foi feita, mas não deu para ler o equipamento de novo. Tente de novo em instantes.',
        );
        return true;
      }
      const ac = this.armorClass();
      this.notice.set(
        `${equipped ? 'Sacado' : 'Guardado'}: ${name}.${ac === null ? '' : ` Classe de armadura ${ac}.`}`,
      );
      return true;
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'mudar o equipamento'));
      return false;
    } finally {
      this.busyKey.set(null);
    }
  }
}
