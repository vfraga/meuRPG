import { computed, effect, signal, untracked } from '@angular/core';

import {
  type Combatant,
  type Encounter,
  EncounterStatus,
  type GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatCalls } from '../../../core/combat/combat-calls';
import { type CombatClient, type MoveResult } from '../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import type { Square } from '../../../core/combat/combat-grid';
import { masterReach } from '../../../core/combat/combat-derived';
import type { CombatState } from '../../../core/combat/combat-state';
import { currentCombatant } from '../../../core/combat/combat-view';
import { article } from '../../../core/combat/combat-log';
import { acts } from '../../../core/combat/joint-turn';
import { MoveOptionsState } from '../../../core/combat/move-options-state';
import { ofThe } from '../../../core/combat/move-plan';
import { ActionKey } from '../../../core/connect/idempotency';
import { MoveSaves } from '../../../core/maps/move-saves';
import { metersFixed } from '../../../core/units';
import type { Reach, TokenDrop } from '../../../shared/combat-map/combat-map';
import type { JumpRequest } from './move-page/move-page';

/** What `MoveFlow` reads from the screen it serves. */
export interface MoveFlowContext {
  readonly api: Pick<CombatClient, 'move' | 'moveOptions'>;
  readonly calls: Pick<CombatCalls, 'busy' | 'error' | 'refreshAfter'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  readonly state: () => CombatState;
  readonly own: () => Combatant | null;
  readonly isMaster: () => boolean;
  readonly active: () => boolean;
  /** The combat is played without a map: moving is "Gastar movimento", a sheet that asks how far. */
  readonly theatre: () => boolean;
  /** The player's `GetTurnOptions` and each creature's, for the jumps a mover has. */
  readonly options: () => GetTurnOptionsResponse | null;
  readonly creatureOptions: () => ReadonlyMap<string, GetTurnOptionsResponse>;
  /** Spends a standard action of a combatant of the player's (the "Mover" page's Disengage). */
  readonly takeAction: (id: string, key: string) => Promise<boolean>;
  /** "Gastar movimento": the sheet for the player's character (no id) or one of their creatures. */
  readonly openSpend: (id?: string) => void;
}

/**
 * Moving on the combat screen: the player's "Mover" page (for the character or one of its creatures: the options the server
 * reads, the confirm, the jump with its key, the notes), the master's drag of a token and the reach it shows. One flow
 * for all of it because they share the page's state (`moving`, the mover, the notes) and the call that sends a move.
 * Build it in an injection context (a field initializer): it keeps two effects, the page closing with the turn and the
 * options asked again whenever the combat changes.
 */
export class MoveFlow {
  /** The creature whose "Mover" page is open (`''`: the character's own). */
  readonly moverId = signal('');
  /** Where the player dropped their token on the main map: the "Mover" page starts there (the drop never moves). */
  readonly dropStart = signal<Square | null>(null);
  /** A refusal of the move on the "Mover" page (TOO_FAR, SQUARE_OCCUPIED). */
  readonly moveError = signal('');
  /** "Você parou antes: algo bloqueou o caminho.": what the last move said when it stopped short. */
  readonly moveNote = signal('');
  /** The last move stopped before a locked door: the "Mover" page stays open and says so (RN-26). */
  readonly lockedDoor = signal(false);
  /** The master's "Movimento forçado" box: it is for one drag, and goes off when the drag is taken. */
  readonly forcedMove = signal(false);
  /** What the last forced drag did, for the live region; cleared by the next drag. */
  readonly forcedNote = signal('');
  /** The "Saltar" page's "Distância" is on: the options are the long jump's (and its warning), not the walk's. */
  readonly jump = signal<{ readonly runningStart: boolean } | null>(null);
  /** The master's "Mostrar o alcance": the reach of whoever is on turn, drawn on the map. */
  readonly reachOn = signal(false);
  /** Where the combatant of the move page can go (`GetMoveOptions`): the player's own, or whoever the master's reach is on for. */
  readonly options = new MoveOptionsState();

  /** Whoever the "Mover" page is for: one of the creatures, or the character. */
  readonly mover = computed(() => {
    const e = this.ctx.encounter();
    const id = this.moverId();
    // A creature that left the combat is not the character: nothing moves, and the page closes (the effect below says so).
    return e ? (id ? (e.combatants.find((c) => c.id === id) ?? null) : this.ctx.own()) : null;
  });
  private readonly moverActs = computed(() => {
    const e = this.ctx.encounter();
    const who = this.mover();
    return !!e && !!who && acts(e, who);
  });
  /** "Desengajar" can still be taken here: the action is free and it was not. */
  readonly canDisengage = computed(() => {
    const who = this.mover();
    return !!who && !who.actionUsed && !who.disengaged;
  });
  /** How far and how high the mover can jump: the creature's own options, or the character's. */
  readonly jumps = computed(() =>
    this.moverId()
      ? this.ctx.creatureOptions().get(this.moverId())?.options?.jumps
      : this.ctx.options()?.options?.jumps,
  );
  /** The reach the master's map draws, as the server said it. */
  readonly masterReach = computed<Reach | null>(() => {
    const e = this.ctx.encounter();
    const who = e ? currentCombatant(e) : null;
    return this.ctx.isMaster() ? masterReach(who, this.reachOn(), this.options.data()) : null;
  });
  readonly reachSwitch = computed(() => {
    const e = this.ctx.encounter();
    const who = e ? currentCombatant(e) : null;
    return this.ctx.isMaster() && who?.placed
      ? { name: ofThe([who.label]), on: this.reachOn() }
      : null;
  });
  /** Whose reach the server is asked for: the player's own on the "Mover" page, the master's on turn with the reach on. */
  private readonly subject = computed(() => {
    const e = this.ctx.encounter();
    if (!e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    if (this.ctx.isMaster()) {
      const who = currentCombatant(e);
      return this.reachOn() && who?.placed ? who.id : null;
    }
    const who = this.mover();
    return this.ctx.state().moving() && this.moverActs() && who?.placed ? who.id : null;
  });

  /** The name of the creature the page was opened for, to say it left. */
  private moverName = '';
  /** One save in flight per combatant (see `MoveSaves`). */
  private readonly saves = new MoveSaves();
  /** The key of a jump: the same jump again is a retry of it. */
  private readonly jumpKey = new ActionKey();

  constructor(private readonly ctx: MoveFlowContext) {
    // "Mover" belongs to the player's turn: when it ends, the page closes.
    effect(() => {
      if (!this.moverActs() || !this.ctx.active()) {
        untracked(() => this.closeWithTurn());
      }
      if (!this.ctx.active()) {
        this.ctx.state().mapOpen.set(false);
      }
    });
    // Where the one on the "Mover" page (or the master's reach) can go: asked again whenever the combat changes.
    effect(() => {
      const e = this.ctx.encounter();
      const who = this.subject();
      const campaignId = this.ctx.campaignId();
      if (!e || !who) {
        untracked(() => this.options.clear());
        return;
      }
      void e.revision;
      // Only while the "Saltar" page is the one open: the master's reach and a closed page read the walk's.
      const jump = this.ctx.state().moving() ? this.jump() : null;
      untracked(
        () => void this.options.load(this.ctx.api, campaignId, e.id, who, jump ?? undefined),
      );
    });
  }

  /** The page closes with the turn; a creature that left the combat is told. */
  private closeWithTurn(): void {
    if (this.moverId() && this.ctx.active() && !this.mover() && this.moverName) {
      this.moveNote.set(
        `${article(this.moverName) === 'a' ? 'A' : 'O'} ${this.moverName} saiu do combate.`,
      );
    }
    this.ctx.state().moving.set(false);
    this.moverId.set('');
  }

  /** A token dropped on a square (the master's drag, or a player's inside their reach). The token is where it was dropped at
   * once; if the server refuses, it goes back and says why. */
  async drop(drop: TokenDrop): Promise<void> {
    const e = this.ctx.encounter();
    const c = e?.combatants.find((x) => x.id === drop.id);
    if (!e || !c) {
      return;
    }
    const from: Square = { col: c.col, row: c.row };
    if (c.placed && from.col === drop.col && from.row === drop.row) {
      return;
    }
    // A player's drop picks the square on the "Mover" page: the same warnings, the same
    // question about a trap, the same confirm. It never moves by itself.
    if (!this.ctx.isMaster()) {
      this.moveError.set('');
      this.dropStart.set({ col: drop.col, row: drop.row });
      this.ctx.state().moving.set(true);
      return;
    }
    await this.dragToken(e, c, from, drop);
  }

  /** The master's drag: the token moves on the screen at once and the server decides (reach, walls, creatures). */
  private async dragToken(
    e: Encounter,
    c: Combatant,
    from: Square,
    drop: TokenDrop,
  ): Promise<void> {
    const { calls } = this.ctx;
    calls.error.set('');
    // The box is for this drag only; a refusal gives it back.
    const forced = this.forcedMove();
    this.forcedMove.set(false);
    this.forcedNote.set('');
    this.ctx
      .state()
      .applyMove({ encounterId: e.id, combatantId: c.id, col: drop.col, row: drop.row });
    // One save at a time per combatant: `MoveSaves` is about positions, and
    // here a square's column and row stand in for x and y.
    await this.saves.move(
      `${e.id}/${c.id}`,
      { xBp: from.col, yBp: from.row },
      { xBp: drop.col, yBp: drop.row },
      {
        // The server decides every move (reach, walls, creatures): a refusal puts the token back.
        save: async (to) => {
          const res = await this.ctx.api.move(
            this.ctx.campaignId(),
            e.id,
            c.id,
            to.xBp,
            to.yBp,
            undefined,
            undefined,
            forced,
          );
          this.ctx.state().apply(res.encounter);
          if (forced) {
            this.forcedNote.set(
              `${c.label} foi mov${article(c.label) === 'a' ? 'ida' : 'ido'} à força. Ninguém recebeu oferta de ataque de oportunidade.`,
            );
          }
          this.sayStopped(res);
        },
        failed: (saved, err) => {
          if (forced) {
            this.forcedMove.set(true);
          }
          this.ctx.state().applyMove({
            encounterId: e.id,
            combatantId: c.id,
            col: saved.xBp,
            row: saved.yBp,
          });
          calls.error.set(combatErrorMessage(err, 'mover o token'));
          void calls.refreshAfter(err);
        },
      },
    );
  }

  /** What stopped the master's drag short, in the page's notice. */
  private sayStopped(res: MoveResult): void {
    if (res.lockedDoor) {
      // The master walks through closed doors but a locked one stops them too (RN-26).
      this.ctx.calls.error.set(
        'Uma porta trancada parou o movimento. Destranque a porta (toque nela no mapa) e mova de novo.',
      );
    } else if (res.stoppedEarly) {
      // A barred or secret door stops it too: the token stands where it stopped.
      this.ctx.calls.error.set(
        'Uma grade levadiça ou uma porta secreta parou o movimento. Abra a porta (toque nela no mapa) e mova de novo.',
      );
    }
  }

  /** "Mover para cá" on the "Mover" page. */
  async confirmMove(to: Square): Promise<void> {
    const own = this.mover();
    if (!own) {
      return;
    }
    this.moveError.set('');
    const ok = await this.runMove((e) =>
      this.ctx.api.move(this.ctx.campaignId(), e.id, own.id, to.col, to.row),
    );
    this.afterMove(ok);
  }

  /** "Saltar para cá" or "Saltar 1,8 m para cima": the same call, as a jump. The new movement
   * used, less what was used before, is what the jump cost: the server's own numbers. */
  async confirmJump(req: JumpRequest): Promise<void> {
    const own = this.mover();
    if (!own) {
      return;
    }
    const before = own.movementUsedDft;
    this.moveError.set('');
    // "Saltar e desengajar": the action is spent first, so the jump leaves the reach with no opportunity attack.
    if (req.kind === 'long' && req.disengage && !(await this.disengage())) {
      return;
    }
    const ok = await this.runMove((e) => this.sendJump(e, own, req));
    if (ok) {
      this.jumpKey.renew();
    }
    // A jump that stopped short already said so (runMove's note): that is not "saltou".
    if (ok && !this.moveNote()) {
      const spent = (this.mover()?.movementUsedDft ?? before) - before;
      this.moveNote.set(
        req.kind === 'long'
          ? `Você saltou ${metersFixed(spent / 10)}.`
          : `Você saltou ${metersFixed(spent / 10)} para cima.`,
      );
    }
    this.afterMove(ok);
  }

  /** The jump's call. The same jump again (a lost answer) is a retry: it keeps its key and is not charged twice. */
  private sendJump(e: Encounter, own: Combatant, req: JumpRequest): Promise<MoveResult> {
    const key = this.jumpKey.keyFor([e.id, own.id, req]);
    const campaignId = this.ctx.campaignId();
    return req.kind === 'long'
      ? this.ctx.api.move(
          campaignId,
          e.id,
          own.id,
          req.square.col,
          req.square.row,
          { kind: 'long' },
          key,
        )
      : this.ctx.api.move(
          campaignId,
          e.id,
          own.id,
          own.col,
          own.row,
          { kind: 'high', heightDft: req.heightDft },
          key,
        );
  }

  private afterMove(ok: boolean): void {
    if (ok && this.lockedDoor()) {
      // A locked door stopped the move: the page stays, with the notice (E10-05 9). The token already stands where it stopped.
      return;
    }
    if (ok) {
      this.ctx.state().moving.set(false);
    } else {
      // The refusal's own words, next to the map where the person looks.
      this.moveError.set(this.ctx.calls.error());
      this.ctx.calls.error.set('');
    }
  }

  /** Runs a move: applies the combat it answers, and says when it stopped short. */
  private async runMove(call: (e: Encounter) => Promise<MoveResult>): Promise<boolean> {
    const e = this.ctx.encounter();
    const { calls } = this.ctx;
    if (!e || calls.busy()) {
      return false;
    }
    calls.busy.set(true);
    calls.error.set('');
    this.moveNote.set('');
    this.lockedDoor.set(false);
    try {
      const res = await call(e);
      this.ctx.state().apply(res.encounter);
      if (res.lockedDoor) {
        this.lockedDoor.set(true);
      } else if (res.stoppedEarly) {
        // Never what stopped it: the player did not see it.
        this.moveNote.set('Você parou antes: algo bloqueou o caminho.');
      }
      return true;
    } catch (err) {
      calls.error.set(combatErrorMessage(err, 'mover'));
      await calls.refreshAfter(err);
      return false;
    } finally {
      calls.busy.set(false);
    }
  }

  /** "Desengajar (gasta a ação)" on the "Mover" page: the warning goes once it is taken. */
  disengage(): Promise<boolean> {
    const who = this.mover();
    return who ? this.ctx.takeAction(who.id, 'standard:disengage') : Promise.resolve(false);
  }

  closeFullPage(): void {
    this.lockedDoor.set(false);
    this.moverId.set('');
    this.dropStart.set(null);
    this.ctx.state().moving.set(false);
    this.ctx.state().mapOpen.set(false);
  }

  /** "Mover": the page of the player's character, or "Gastar movimento" in a combat without a map. */
  open(): void {
    if (this.ctx.theatre()) {
      this.ctx.openSpend();
      return;
    }
    this.lockedDoor.set(false);
    this.moverId.set('');
    this.dropStart.set(null);
    this.moveError.set('');
    this.ctx.state().moving.set(true);
  }

  /** "Mover o Lobo atroz 1": the same page, for the creature. */
  openCreature(id: string): void {
    if (this.ctx.theatre()) {
      this.ctx.openSpend(id);
      return;
    }
    this.moverName = this.ctx.encounter()?.combatants.find((c) => c.id === id)?.label ?? '';
    this.moverId.set(id);
    this.lockedDoor.set(false);
    this.dropStart.set(null);
    this.moveError.set('');
    this.ctx.state().moving.set(true);
  }
}
