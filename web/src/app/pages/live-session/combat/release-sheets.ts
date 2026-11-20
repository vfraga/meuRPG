import type {
  Encounter,
  ReactionWindow,
  ReadiedAction,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { ReadiedKind } from '../../../../gen/meurpg/play/v1/combat_pb';
import { readiedOfWindow } from '../../../core/combat/reaction-readied';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { MasterAreaCast, type MasterAreaCastData } from './area-cast/master-area-cast';
import { releaseCastData } from './cast-sheet/cast-release';
import { opensOnMap, masterAreaFocus } from './cast-sheet/cast-sheet-data';
import { CastSheet, type CastMapData, type CastSheetData } from './cast-sheet/cast-sheet';
import type { CombatSheetsContext } from './combat-sheets-context';

/** What it says when the map cannot be opened for the release. */
const CANNOT_OPEN = 'Não deu para abrir o mapa. Tente de novo.';

/**
 * "Soltar no mapa", "Mover agora", "Soltar por Pensantus" and "Mover por Tor" (#857, #856): the map opens for the readied action a
 * READIED window holds, in the screen that already does it (the cast sheet, the master's area dialog, the Mover page), in release mode.
 * "Voltar" shows the window again for the player.
 */
export class ReleaseSheets {
  /** The windows whose spell is being read for the map: a second press meanwhile opens nothing. */
  private readonly opening = new Set<string>();

  constructor(
    private readonly ctx: CombatSheetsContext,
    private readonly castMap: () => CastMapData | null,
    /** Shows the readier's window again (the player went back from the map). */
    private readonly reopen: (windowId: string) => void,
  ) {}

  /** The map for the window's held action: a spell's cast sheet, or the Mover page. */
  open(windowId: string): void {
    const e = this.ctx.encounter();
    const w = e?.reactionWindows.find((x) => x.id === windowId);
    const held = e && w ? readiedOfWindow(e, w) : undefined;
    if (!e || !w || !held) {
      return;
    }
    if (held.kind === ReadiedKind.MOVE) {
      this.ctx.releaseMove?.(windowId);
    } else if (held.kind === ReadiedKind.SPELL) {
      void this.spell(e, w, held);
    }
  }

  private async spell(e: Encounter, w: ReactionWindow, held: ReadiedAction): Promise<void> {
    const map = this.castMap();
    const caster = e.combatants.find((c) => c.id === w.reactorId);
    if (!map || !caster) {
      this.ctx.note(CANNOT_OPEN);
      return;
    }
    if (this.opening.has(w.id)) {
      return;
    }
    this.opening.add(w.id);
    try {
      // The held spell's targets come from the readier's own options, whoever releases it.
      const turn = await this.ctx.api.moves.turnOptions(this.ctx.campaignId(), e.id, w.reactorId);
      const data = releaseCastData({
        campaignId: this.ctx.campaignId(),
        encounter: e,
        window: w,
        readied: held,
        caster,
        turn,
        master: this.ctx.isMaster(),
        diceMode: this.ctx.diceMode(),
        preference: this.ctx.preference(),
        state: this.ctx.state(),
        map,
      });
      if (!data) {
        this.ctx.note(CANNOT_OPEN);
        return;
      }
      if (this.ctx.isMaster()) {
        this.masterSheet(data, caster);
      } else {
        this.playerSheet(data, w.id);
      }
    } catch {
      this.ctx.note(CANNOT_OPEN);
    } finally {
      this.opening.delete(w.id);
    }
  }

  /** The player's cast sheet in release mode; closing it without the cast shows the window again. */
  private playerSheet(data: CastSheetData, windowId: string): void {
    openSheet<CastSheet, CastSheetData, boolean>(this.ctx.dialog, this.ctx.bottomSheet, CastSheet, {
      data,
      ariaLabel: `Soltar ${data.name}`,
      labelledBy: 'sheet-t',
      ...(opensOnMap(data) ? { focus: '[role="application"]' } : {}),
    }).subscribe({ complete: () => this.reopen(windowId) });
  }

  /** The master's two-column dialog, with the same band; nothing to show again afterwards (he has the queue). */
  private masterSheet(data: CastSheetData, caster: MasterAreaCastData['caster']): void {
    if (!data.targets || !data.map || !data.release) {
      return;
    }
    const master: MasterAreaCastData = {
      campaignId: data.campaignId,
      encounterId: data.encounterId,
      caster,
      spellKey: data.spellKey,
      name: data.name,
      level: data.level,
      slot: null,
      economy: data.economy,
      targets: data.targets,
      map: data.map,
      state: data.state,
      release: data.release,
    };
    openSheet<MasterAreaCast, MasterAreaCastData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      MasterAreaCast,
      {
        data: master,
        ariaLabel: `Soltar ${data.name} por ${caster.label}`,
        labelledBy: 'sheet-t',
        width: '1100px',
        tall: true,
        ...masterAreaFocus(data.targets),
      },
    ).subscribe();
  }
}
