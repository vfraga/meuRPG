import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  type Signal,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Observable } from 'rxjs';

import type { PreviewRevivifyResponse } from '../../../../../gen/meurpg/play/v1/revivify_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { describeConnectError } from '../../../../core/connect/connect-errors';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  DENIED_TEXT,
  DIAMONDS_HINT,
  DIAMONDS_NOTE,
  NOBODY_TEXT,
  NO_SLOT_TEXT,
  REVIVIFY_LEVEL,
  REFUSED_TEXT,
  type ResultLines,
  type RevivifyStep,
  canCast,
  costLine,
  requestOutcome,
  resultLines,
  sheetSubtitle,
  stepMarks,
  targetRows,
} from '../../../../core/revivify/revivify-flow';
import { RevivifyClient } from '../../../../core/revivify/revivify-client';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the Revivificar sheet. */
export interface RevivifySheetData {
  readonly campaignId: string;
  /** The caster's name and class line ("Clérigo 5"; empty when the sheet is not read yet). */
  readonly casterName: string;
  readonly classes: string;
  /** In a combat: the combat, the combatant that casts and the page's copy of the combat the answer goes to. `null` outside one. */
  readonly combat: {
    readonly encounterId: string;
    readonly casterId: string;
    readonly round: number;
    readonly state: CombatState;
  } | null;
  /** Outside a combat: the character that casts. */
  readonly casterCharacterId: string;
  /** Goes up when the casts outside a combat may have changed (the stream's hint): the sheet reads them again. */
  readonly reload: Signal<number>;
}

/** The Revivificar sheet: a bottom sheet on a phone, a dialog from a tablet up. */
export function openRevivifySheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: RevivifySheetData,
): Observable<boolean | undefined> {
  return openSheet<RevivifySheet, RevivifySheetData, boolean>(dialog, bottomSheet, RevivifySheet, {
    data,
    ariaLabel: 'Revivificar',
    labelledBy: 'sheet-t',
    restoreFocus: true,
  });
}

/**
 * "Revivificar" (SRD 5.1, Revivify), the player's cast in three steps: **Alvo** (who died nearby and the spell can
 * still reach: nothing else is listed, and no reason for who is missing), **Confirmar** (the slot and the action it
 * costs, and the diamonds, which the caster ticks: the app only reminds, the table checks) and **Resultado**. In a
 * combat the cast is `CombatService.CastSpell` and the result is read at once, with "Encerrar turno". Outside one
 * the app does not count the time: the cast is a request that waits for the master ("Esperando o mestre", nothing
 * spent), and the answer comes back by the stream's hint, as "voltou à vida" or only "O mestre disse que não dá.".
 * A refused target is always the same sentence. Focus goes to the title at each step, so a screen reader reads where
 * it is; the results are `role="status"`.
 */
@Component({
  selector: 'app-revivify-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './revivify-sheet.html',
  styleUrl: './revivify-sheet.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RevivifySheet {
  private readonly combatApi = inject(CombatClient);
  private readonly api = inject(RevivifyClient);
  private readonly sheet = injectSheet<RevivifySheetData, boolean>();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly inCombat = this.data.combat !== null;

  protected readonly nobody = NOBODY_TEXT;
  protected readonly noSlot = NO_SLOT_TEXT;
  protected readonly note = DIAMONDS_NOTE;
  protected readonly hint = DIAMONDS_HINT;
  protected readonly deniedText = DENIED_TEXT;

  protected readonly step = signal<RevivifyStep>('target');
  protected readonly preview = signal<PreviewRevivifyResponse | null>(null);
  protected readonly loading = signal(true);
  protected readonly chosen = signal('');
  protected readonly diamonds = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** Outside a combat: the request that waits for the master, until he answers. */
  protected readonly requestId = signal('');
  protected readonly denied = signal(false);
  protected readonly lines = signal<ResultLines | null>(null);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));

  protected readonly marks = computed(() => stepMarks(this.step()));
  protected readonly rows = computed(() => targetRows(this.preview()?.targets ?? []));
  protected readonly waitsMaster = computed(() => this.rows().some((r) => r.waitsMaster));
  protected readonly hasTarget = computed(() => this.chosen() !== '');
  protected readonly target = computed(() => this.rows().find((r) => r.id === this.chosen()));
  protected readonly slot = computed(() => this.preview()?.slot);
  protected readonly free = computed(() => this.preview()?.slotsFree ?? 0);
  protected readonly ready = computed(() => canCast(this.chosen(), !!this.slot(), this.diamonds()));
  protected readonly waiting = computed(
    () => this.step() === 'result' && !this.denied() && this.lines() === null,
  );
  protected readonly cost = computed(() =>
    costLine(this.slot()?.level ?? REVIVIFY_LEVEL, this.free(), this.inCombat),
  );
  protected readonly title = computed(() =>
    this.step() === 'confirm' && this.target()
      ? `Revivificar em ${this.target()?.name}`
      : 'Revivificar',
  );
  protected readonly subtitle = computed(() =>
    this.step() === 'confirm'
      ? `${this.target()?.name ?? ''} volta à vida com 1 PV.`
      : this.step() === 'result'
        ? 'Resultado'
        : sheetSubtitle(this.data.casterName, this.data.classes),
  );

  /** One key for the cast, kept across a retry of the same target and slot. */
  private readonly keys = new ActionKey();
  private seq = 0;

  constructor() {
    void this.readPreview();
    // Outside a combat the master's answer comes as a hint: read the casts again while waiting.
    effect(() => {
      this.data.reload();
      untracked(() => {
        if (this.waiting()) {
          void this.readAnswer();
        }
      });
    });
    // Each step takes the focus on its title, so the screen reader says where the person is.
    effect(() => {
      this.step();
      this.waiting();
      this.denied();
      untracked(() =>
        afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>('h2')?.focus(), {
          injector: this.injector,
        }),
      );
    });
  }

  private async readPreview(): Promise<void> {
    const seq = ++this.seq;
    this.loading.set(true);
    try {
      const c = this.data.combat;
      const res = await this.api.preview(
        this.data.campaignId,
        c
          ? { encounterId: c.encounterId, casterId: c.casterId }
          : { casterCharacterId: this.data.casterCharacterId },
      );
      if (seq !== this.seq) {
        return;
      }
      this.preview.set(res);
      const still = res.targets.some((t) => t.targetId === this.chosen());
      this.chosen.set(still ? this.chosen() : (res.targets.at(0)?.targetId ?? ''));
    } catch (err) {
      if (seq === this.seq) {
        this.error.set(describeConnectError(err, {}));
      }
    } finally {
      if (seq === this.seq) {
        this.loading.set(false);
      }
    }
  }

  protected pick(id: string): void {
    this.chosen.set(id);
    this.error.set('');
  }

  protected next(): void {
    if (this.hasTarget()) {
      this.error.set('');
      this.step.set('confirm');
    }
  }

  protected previous(): void {
    if (!this.busy()) {
      this.error.set('');
      this.step.set('target');
    }
  }

  protected async cast(): Promise<void> {
    const slot = this.slot();
    const target = this.target();
    if (!this.ready() || this.busy() || !slot || !target) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const key = this.keys.keyFor({ target: target.id, slot: [slot.level, slot.pact] });
      const c = this.data.combat;
      if (c) {
        const res = await this.combatApi.casts.castRevivify(
          this.data.campaignId,
          c.encounterId,
          c.casterId,
          { level: slot.level, pact: slot.pact },
          target.id,
          key,
        );
        c.state.apply(res.encounter);
        this.lines.set(resultLines(target.name, slot.level, this.free() - 1, this.free(), true));
      } else {
        const req = await this.api.request(
          this.data.campaignId,
          this.data.casterCharacterId,
          target.id,
          { level: slot.level, pact: slot.pact },
          key,
        );
        this.requestId.set(req?.id ?? '');
      }
      this.keys.renew();
      this.step.set('result');
      if (!c) {
        void this.readAnswer();
      }
    } catch (err) {
      this.fail(err);
    } finally {
      this.busy.set(false);
    }
  }

  /** The master's answer (or the same answer again): CONFIRMED shows the result, DENIED only his sentence. */
  private async readAnswer(): Promise<void> {
    const id = this.requestId();
    if (!id) {
      return;
    }
    try {
      const { outcome, request } = requestOutcome(await this.api.list(this.data.campaignId), id);
      if (outcome === 'denied') {
        this.denied.set(true);
      } else if (outcome === 'confirmed' && request) {
        const left = request.slotsLeft;
        this.lines.set(
          resultLines(
            request.targetName,
            request.slot?.level ?? this.slot()?.level ?? REVIVIFY_LEVEL,
            left,
            left + 1,
            false,
          ),
        );
      }
    } catch {
      // Keep waiting: the next hint or connection reads again.
    }
  }

  /** A refused target is always the same sentence; the list is read again. Anything else says what failed. */
  private fail(err: unknown): void {
    const connectErr = ConnectError.from(err, Code.Unavailable);
    if (connectErr.code === Code.FailedPrecondition && !encounterBlocked(err)) {
      this.error.set(REFUSED_TEXT);
      this.step.set('target');
      void this.readPreview();
      return;
    }
    this.error.set(
      this.inCombat
        ? combatErrorMessage(err, 'conjurar Revivificar')
        : describeConnectError(err, {}),
    );
  }

  protected async endTurn(): Promise<void> {
    const c = this.data.combat;
    if (!c || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      c.state.apply(
        await this.combatApi.turn.endTurn(
          this.data.campaignId,
          c.encounterId,
          c.casterId,
          false,
          c.round,
        ),
      );
      this.sheet.close(true);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'encerrar o turno'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    this.sheet.close(this.lines() !== null);
  }
}
