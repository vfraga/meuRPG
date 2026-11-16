import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Combatant, SpellTargets } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { TurnOptions } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  READY_FAILED_TEXT,
  READY_TRIGGER_MAX,
  TRIGGER_HINT,
  readyArgs,
  readyAttackRows,
  readyKinds,
  readyMissing,
  readyOtherRows,
  readySpellRows,
  triggerState,
  type ReadyKind,
  type ReadyRow,
} from '../../../../core/combat/ready';
import {
  concentrationNotice,
  readyLegend,
  readyMoveText,
} from '../../../../core/combat/ready-text';
import { ActionKey } from '../../../../core/connect/idempotency';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ResourcePick } from '../../../../shared/resource-pick/resource-pick';

/** What the page hands the Ready sheet. */
export interface ReadySheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The readier: the player's character, or the combatant the master acts as. */
  readonly combatantId: string;
  readonly ownerLabel: string;
  /** The turn as `GetTurnOptions` sends it: the attacks, the spells and the standard actions. */
  readonly options: TurnOptions;
  /** The summary line of each spell, by key (`GetSpellDetails`); none while it is not read. */
  readonly summaries: ReadonlyMap<string, string>;
  /** What `GetTurnOptions` says of who each spell can target: darts, a placed area or several targets cannot be released by the window. */
  readonly spellTargets: readonly SpellTargets[];
  /** A combat with no map: no square to walk to. */
  readonly theatre: boolean;
  /** The readier has no speed now. */
  readonly noSpeed: boolean;
  /** The metres the readied move may go: the speed now, in feet. */
  readonly speedFt: number;
  /** The spell the readier concentrates on now ("Névoa Obscurecente"), or `''`. */
  readonly concentrating: string;
  /** Everyone of the combat, to name in the trigger. */
  readonly combatants: readonly Combatant[];
  readonly state: CombatState;
}

/**
 * "Preparar uma ação" (#582, boards 582-01, 582-02 and 582-09): the sheet of the Ready action (`ActionOption.via_ready_action`),
 * taken with `ReadyService.ReadyAction`. The player picks what to hold (an attack, a spell and the slot, a move, another
 * standard action), whom the trigger names, if anyone, and writes the trigger in their own words (1 to 200 characters,
 * counted). The server decides every refusal: its sentence is written in a red band, the sheet stays open, what was typed
 * stays and nothing was spent. While the call is in the air both buttons stop and the sheet does not close under it. A
 * bottom sheet on a phone and a dialog from a tablet up.
 */
@Component({
  selector: 'app-ready-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ResourcePick, SheetFrame],
  templateUrl: './ready-sheet.html',
  styleUrls: ['../resource-pick/resource-sheet.scss', './ready-sheet.scss'],
})
export class ReadySheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<ReadySheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly max = READY_TRIGGER_MAX;
  protected readonly hint = TRIGGER_HINT;

  protected readonly kinds = readyKinds(this.data.options);
  protected readonly moveText = readyMoveText(this.data.speedFt);
  /** The creatures the trigger may name: everyone in the fight but the readier and the fallen. */
  protected readonly targets = this.data.combatants
    .filter((c) => c.id !== this.data.combatantId && c.state !== CombatantState.DEFEATED)
    .map((c) => ({ id: c.id, label: c.label }));

  protected readonly kind = signal<ReadyKind>(this.kinds.find((k) => !k.blocked)?.key ?? 'attack');
  protected readonly key = signal('');
  protected readonly slotLevel = signal(0);
  protected readonly targetId = signal('');
  protected readonly triggerText = signal('');
  protected readonly touched = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');

  protected readonly rows = computed<readonly ReadyRow[]>(() => {
    switch (this.kind()) {
      case 'attack':
        return readyAttackRows(this.data.options);
      case 'spell':
        return readySpellRows(this.data.options, this.data.spellTargets, this.data.summaries);
      case 'other':
        return readyOtherRows(this.data.options);
      default:
        return [];
    }
  });
  protected readonly row = computed(() => this.rows().find((r) => r.id === this.key()));
  protected readonly slots = computed(() =>
    this.kind() === 'spell' ? (this.row()?.slots ?? []) : [],
  );
  protected readonly kindBlocked = computed(
    () => this.kinds.find((k) => k.key === this.kind())?.blocked ?? '',
  );
  protected readonly listLabel = computed(() => {
    switch (this.kind()) {
      case 'attack':
        return 'Qual ataque';
      case 'spell':
        return 'Qual magia · só as de 1 ação';
      default:
        return 'Qual ação';
    }
  });
  protected readonly trigger = computed(() => triggerState(this.triggerText(), this.touched()));
  private readonly pick = computed(() => ({
    kind: this.kind(),
    key: this.key(),
    slotLevel: this.slotLevel(),
    targetId: this.targetId(),
    trigger: this.triggerText(),
  }));
  protected readonly missing = computed(() =>
    readyMissing(this.pick(), this.row(), this.kindBlocked()),
  );
  protected readonly doLabel = computed(() =>
    this.kind() === 'spell' && this.row() ? `Preparar ${this.row()?.title}` : 'Preparar',
  );
  protected readonly legend = computed(() => readyLegend(this.kind(), this.slotLevel()));
  protected readonly concentration = computed(() =>
    this.kind() === 'spell' && this.row() && !this.row()?.blocked
      ? concentrationNotice(this.data.concentrating, this.row()?.title ?? '')
      : null,
  );

  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly callKey = new ActionKey();

  constructor() {
    this.fill();
  }

  /** The first card that can be held is chosen, as the board shows it, and its lowest slot. */
  private fill(): void {
    const first = this.rows().find((r) => !r.blocked);
    this.key.set(first?.id ?? '');
    this.slotLevel.set(first?.slots[0]?.level ?? 0);
  }

  protected pickKind(kind: ReadyKind): void {
    this.kind.set(kind);
    this.error.set('');
    this.fill();
  }

  protected pickRow(id: string): void {
    this.key.set(id);
    this.slotLevel.set(this.rows().find((r) => r.id === id)?.slots[0]?.level ?? 0);
    this.error.set('');
  }

  protected pickSlot(level: number): void {
    this.slotLevel.set(level);
  }

  protected pickTarget(event: Event): void {
    this.targetId.set((event.target as HTMLSelectElement).value);
  }

  protected typed(event: Event): void {
    this.triggerText.set((event.target as HTMLTextAreaElement).value);
    this.error.set('');
  }

  protected async ready(): Promise<void> {
    this.touched.set(true);
    if (this.busy() || this.missing()) {
      return;
    }
    const args = {
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      combatantId: this.data.combatantId,
      ...readyArgs(this.pick()),
    };
    this.busy.set(true);
    this.error.set('');
    try {
      const enc = await this.api.ready.readyAction(args, this.callKey.keyFor(args));
      this.callKey.renew();
      this.data.state.apply(enc);
      this.busy.set(false);
      this.sheet.close(true);
    } catch (err) {
      this.error.set(this.refusal(err));
      this.busy.set(false);
    }
  }

  /** A refusal the server worded is read as it is; a call that did not get through says nothing was spent and to try again. */
  private refusal(err: unknown): string {
    if (encounterBlocked(err)) {
      return combatErrorMessage(err);
    }
    const code = ConnectError.from(err, Code.Unavailable).code;
    return code === Code.Unavailable || code === Code.Unknown || code === Code.Internal
      ? READY_FAILED_TEXT
      : combatErrorMessage(err, 'preparar a ação');
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close();
  }
}
