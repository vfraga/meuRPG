import { signal } from '@angular/core';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { AttunementEntryVm } from '../../core/characters/attunement';
import type { ConsumableKind } from '../../core/characters/consumable-vm';
import {
  type ConsumableNotice,
  type ConsumableTargetVm,
  goneNotice,
  targetRows,
  type TurnBlock,
  useNotice,
} from '../../core/characters/consumables';
import type { UseConsumableResponse } from '../../../gen/meurpg/play/v1/charges_pb';
import { effectivePreference } from '../../core/campaigns/dice-labels';
import {
  type ChargesClient,
  type ConsumableRoll,
  type TableDice,
} from '../../core/play/charges-client';
import {
  type ConsumableErrorContext,
  type ConsumableRefusal,
  consumableNeedsReload,
  consumableRefusal,
  consumableTargetsRefusal,
} from '../../core/play/charges-errors';
import type {
  Aftermath,
  TargetsOutcome,
  UseAnswer,
  UseConsumableData,
  UseConsumableRequest,
} from './use-consumable-sheet';

/** The character whose consumables are used, as the page that owns the use holds them. */
export interface ConsumableOwner {
  readonly id: string;
  readonly name: string;
  /** The master acts for this character: the sheet reads "Usar por Tor". */
  readonly byMaster: boolean;
  /** The magic items the character holds, one entry for each unit. */
  readonly entries: readonly AttunementEntryVm[];
}

/** What the use of the consumables needs of the page that owns it: the character's sheet, or its combat. */
export interface ConsumableUseHost {
  readonly client: () => ChargesClient;
  readonly campaignId: () => string;
  /** The character as the page holds it. */
  readonly owner: () => ConsumableOwner | null;
  /**
   * Whether a combat is running, when the page knows (the combat's "Itens" does); `null` on the sheet, which learns it from
   * the targets read (`ListConsumableTargetsResponse.in_combat`) and says nothing of the cost until then.
   */
  readonly inCombat: () => boolean | null;
  /** What the page knows keeps a potion from the Action (the turn is not the character's, or the Action is spent); `null` where it does not know, as on the sheet. */
  readonly turnBlock: () => TurnBlock | null;
  /** Reads what the page holds of the character again without the loading state; `true` when the new read is on screen. */
  readonly reload: () => Promise<boolean>;
  /** The table's dice and the player's choice (RN-18), read once from the campaign. */
  readonly dice: () => Promise<TableDice>;
  /** Puts the focus on the row of the consumable (its "Usar") once the page has drawn it again. */
  readonly focusConsumable: (key: string) => void;
  readonly openUse: (data: UseConsumableData) => Observable<boolean | undefined>;
}

/** The unit a sheet is open on: it changes when the unit is gone and another one of the same item is left. */
interface Session {
  entry: AttunementEntryVm;
  /** The refusal of a gone unit whose sheet was not read again: a later read of it closes the sheet with the same words. */
  unread?: ConsumableRefusal;
}

/** A refusal as the notice of the page, for a sheet that closes under it. */
function refusalNotice(refusal: ConsumableRefusal): ConsumableNotice {
  return { tone: 'neutral', icon: 'info', headline: refusal.lead, detail: refusal.text, text: [] };
}

/**
 * The potions, oils and scrolls of a character: "Usar" opens the use sheet (`UseConsumableSheet`) and every call goes to
 * `ChargesService`. What each entry is and costs is the server's (`AttunementEntryVm.consumable`); an entry without it is no
 * consumable and has no row. The page reads the character again after a use, since the unit is gone, and after every failure
 * but a call that never left: the sheet then follows what the page now holds (the count, the unit, the receivers). Works on a
 * locked sheet, for the owner and for the master, who acts for the character ("Usar por Tor").
 */
export class ConsumableUse {
  /** What the last use said, for the live region under the magic items or the combat's items. */
  readonly result = signal<ConsumableNotice | null>(null);
  /** A sheet is opening or open: a second tap on "Usar" opens nothing. */
  private opening = false;

  constructor(private readonly host: ConsumableUseHost) {}

  /** "Usar" on the row of a consumable: `entry` is the unit the row sends (one entry of the group). */
  openUse(entry: AttunementEntryVm): void {
    const vm = this.host.owner();
    const consumable = entry.consumable;
    if (!vm || !consumable || this.opening) {
      return;
    }
    this.opening = true;
    this.result.set(null);
    const kind = consumable.kind;
    const session: Session = { entry };
    const done = () => {
      this.opening = false;
    };
    this.host
      .dice()
      .then((dice) => {
        this.host
          .openUse({
            itemName: entry.namePt,
            kind,
            consumable,
            effectPt: entry.effectPt ?? '',
            left: Math.max(1, this.unitsOf(vm, entry.key)),
            actingFor: vm.byMaster ? vm.name : null,
            inCombat: this.host.inCombat(),
            blockedBy: consumable.costsAction ? this.host.turnBlock() : null,
            canApp: dice.mode !== DiceMode.PHYSICAL,
            canType: dice.mode !== DiceMode.APP,
            preferApp: effectivePreference(dice.mode, dice.preference) === DicePreference.APP,
            loadTargets: () => this.loadTargets(session, vm, kind),
            reread: () => this.reread(session, kind),
            use: (request, key) => this.use(session, vm, kind, request, key),
          })
          .subscribe({
            next: () => this.host.focusConsumable(session.entry.key),
            error: done,
            complete: done,
          });
      })
      .catch(done);
  }

  private unitsOf(vm: ConsumableOwner, key: string): number {
    return vm.entries.filter((e) => e.key === key).length;
  }

  /**
   * What the page holds now of the unit a refusal was about, once it has been read again: the units left and, when the unit
   * the sheet was open on is no longer there, the one that takes its place, or that the sheet closes with a notice when no
   * unit is left. Nothing is said of a read that is not on screen.
   */
  private async aftermath(
    session: Session,
    kind: ConsumableKind,
    refusal: ConsumableRefusal,
  ): Promise<Aftermath> {
    const onScreen = await this.host.reload().catch(() => false);
    const vm = this.host.owner();
    if (!onScreen || !vm) {
      // The unit is gone and the sheet still holds it: the sheet may not offer it until a read lands.
      if (refusal.cause === 'item-gone') {
        session.unread = refusal;
        return { unread: true };
      }
      return {};
    }
    session.unread = undefined;
    const units = vm.entries.filter((e) => e.key === session.entry.key);
    if (units.length === 0) {
      this.result.set(refusal.cause === 'item-gone' ? goneNotice(kind) : refusalNotice(refusal));
      return { left: 0, close: true };
    }
    if (units.some((e) => e.id === session.entry.id)) {
      return { left: units.length };
    }
    session.entry = units[0];
    return { left: units.length, rebound: true };
  }

  /** Reads the sheet again for a unit that is gone and was not read: where it stands now, or still unread. */
  private async reread(session: Session, kind: ConsumableKind): Promise<Aftermath> {
    return session.unread ? this.aftermath(session, kind, session.unread) : {};
  }

  private context(
    vm: ConsumableOwner,
    kind: ConsumableKind,
    target?: ConsumableTargetVm | null,
    typedDice?: boolean,
  ): ConsumableErrorContext {
    return {
      kind,
      userName: vm.name,
      targetName: target?.name,
      targetId: target?.id,
      actingFor: vm.byMaster ? vm.name : undefined,
      typedDice,
    };
  }

  private async loadTargets(
    session: Session,
    vm: ConsumableOwner,
    kind: ConsumableKind,
    again = true,
  ): Promise<TargetsOutcome> {
    try {
      const list = await this.host
        .client()
        .listConsumableTargets(this.host.campaignId(), vm.id, session.entry.id);
      return { targets: targetRows(list.targets), inCombat: list.inCombat, error: null };
    } catch (err) {
      const error = consumableTargetsRefusal(err, this.context(vm, kind));
      if (!consumableNeedsReload(err)) {
        return { targets: [], inCombat: null, error };
      }
      const after = await this.aftermath(session, kind, error);
      if (after.rebound && again) {
        // The unit was used: the list is read again for the one that takes its place.
        const next = await this.loadTargets(session, vm, kind, false);
        return { ...next, left: after.left, rebound: true };
      }
      return { targets: [], inCombat: null, error, ...after };
    }
  }

  private async use(
    session: Session,
    vm: ConsumableOwner,
    kind: ConsumableKind,
    request: UseConsumableRequest,
    key: string,
  ): Promise<UseAnswer> {
    const target: ConsumableTargetVm | null = request.target;
    const roll: ConsumableRoll = request.roll;
    let res: UseConsumableResponse;
    try {
      res = await this.host
        .client()
        .useConsumable(
          this.host.campaignId(),
          vm.id,
          session.entry.id,
          target?.id ?? '',
          roll,
          key,
        );
    } catch (err) {
      // Only the call's own failure may say nothing was spent.
      const refusal = consumableRefusal(err, this.context(vm, kind, target, 'faces' in roll));
      if (!consumableNeedsReload(err)) {
        return { refusal };
      }
      return { refusal, ...(await this.aftermath(session, kind, refusal)) };
    }
    // The unit was spent. The notice follows what the page holds once it has read the character again, never a count made up
    // here; a read that fails leaves the count out and the unit still used.
    const onScreen = await this.host.reload().catch(() => false);
    const now = this.host.owner();
    const entry = session.entry;
    this.result.set(
      useNotice(res, {
        itemName: entry.namePt,
        kind,
        effectPt: entry.effectPt ?? '',
        heal: entry.consumable?.heal ?? null,
        userName: vm.name,
        receiverName: target?.name ?? vm.name,
        gift: target !== null,
        woke: target?.down === true,
        left: onScreen && now ? this.unitsOf(now, entry.key) : null,
      }),
    );
    return { refusal: null };
  }
}
