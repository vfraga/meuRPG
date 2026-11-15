import { computed, signal } from '@angular/core';
import type {
  Combatant,
  CombatLogEntry,
  Encounter,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ConcentrationWatch,
  type FormEnded,
  FormNotices,
  type LostConcentration,
  formNoticeText,
  lostNoticeText,
} from '../../../core/combat/combat-notices';
import {
  type ReadiedLost,
  ReadiedNotices,
  readiedLostNoticeText,
} from '../../../core/combat/ready-notice';
import type { CombatCalls } from '../../../core/combat/combat-calls';
import type { CombatClient } from '../../../core/combat/combat-client';

/** What `CombatNotices` reads from the screen it serves. */
export interface CombatNoticesContext {
  readonly api: Pick<CombatClient, 'combatants'>;
  readonly calls: Pick<CombatCalls, 'run'>;
  readonly campaignId: () => string;
  readonly ownId: () => string | undefined;
}

/**
 * The notices the player is told when something of theirs ended without them asking: the Wild Shape form (from the
 * combat log's line) and the concentration that held creatures (from the combat's own data). Each stays until it is
 * touched. The screen's effects read the signals they track and hand the values to `readForm` and `readConcentration`.
 */
export class CombatNotices {
  /** What the form's end said, from the combat log's line (the server's number and reason): until it is touched. */
  readonly formEnded = signal<FormEnded | null>(null);
  readonly formNotice = computed(() => {
    const ended = this.formEnded();
    return ended ? formNoticeText(ended) : '';
  });
  /** What the player's own concentration ending took with it: the spell and the creatures that left, until it is touched. */
  readonly lostConcentration = signal<LostConcentration | null>(null);
  readonly lostNotice = computed(() => {
    const lost = this.lostConcentration();
    return lost ? lostNoticeText(lost) : '';
  });
  /** The readied action of the player that was lost (#582), from the combat log's `READIED_LOST` line: until it is touched. */
  readonly readiedLost = signal<ReadiedLost | null>(null);
  readonly readiedNotice = computed(() => {
    const lost = this.readiedLost();
    return lost ? readiedLostNoticeText(lost) : null;
  });
  readonly concentration = new ConcentrationWatch();
  private readonly formNotices = new FormNotices();
  private readonly readiedNotices = new ReadiedNotices();

  constructor(private readonly ctx: CombatNoticesContext) {}

  /** The combat log, read again: a new combat clears both notices, a form that ended on its own is told. */
  readForm(
    encounterId: string,
    ownId: string,
    loaded: boolean,
    entries: readonly CombatLogEntry[],
  ): void {
    const { reset, notice } = this.formNotices.read(encounterId, ownId, loaded, entries);
    const readied = this.readiedNotices.read(encounterId, ownId, loaded, entries);
    if (reset) {
      this.formEnded.set(null);
      this.lostConcentration.set(null);
      this.readiedLost.set(null);
    }
    if (notice) {
      this.formEnded.set(notice);
    }
    if (readied.notice) {
      this.readiedLost.set(readied.notice);
    }
  }

  /** The player's spell and the creatures it holds, read again: a loss is told, an undo takes it away. */
  readConcentration(encounterId: string, spell: string, creatures: readonly Combatant[]): void {
    const out = this.concentration.read(encounterId, spell, creatures, this.lostConcentration());
    if (out !== undefined) {
      this.lostConcentration.set(out);
    }
  }

  /** The player ends their own concentration (the conditions are the master's). */
  endOwnConcentration(): Promise<boolean> {
    const ownId = this.ctx.ownId();
    this.concentration.endedByMe = true;
    return ownId
      ? this.ctx.calls
          .run((e: Encounter) =>
            this.ctx.api.combatants.setConditions(this.ctx.campaignId(), e.id, ownId, {
              endConcentration: true,
            }),
          )
          .then((ok) => {
            if (!ok) {
              this.concentration.endedByMe = false;
            }
            return ok;
          })
      : Promise.resolve(false);
  }
}
