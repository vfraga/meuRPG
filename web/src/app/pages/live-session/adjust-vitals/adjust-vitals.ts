import { Component, ElementRef, computed, effect, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MAT_BOTTOM_SHEET_DATA, MatBottomSheetRef } from '@angular/material/bottom-sheet';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { AuthService } from '../../../core/auth/auth.service';
import { LiveSessionSource, VitalsChange } from '../live-session.types';
import { slotLevelLabel, usedWords, whoSeesTheChange } from '../vitals';
import { VitalsStepper } from '../vitals-stepper/vitals-stepper';
import {
  AdjustVitalsData,
  AdjustVitalsResult,
  MAX_TEMPORARY_HP,
  VitalsDraft,
  changeBetween,
  draftFrom,
  sameChange,
} from './adjust-vitals.types';

type SaveState =
  | { status: 'idle' }
  | { status: 'saving' }
  | { status: 'error'; title: string; detail: string }
  | { status: 'ended' };

/**
 * "Ajustar Brisa" (RN-02, artboard E5-05): the master's correction of a
 * character's hit points, temporary hit points, spell slots, hit dice,
 * resource uses and a beast form's hit points during the session. One component in two containers: a bottom sheet on
 * a phone, a dialog of about 440px from a tablet up (the page picks).
 *
 * Every number is absolute ("9", not "−5"), and "Salvar ajuste" sends one
 * `AdjustCharacterVitals` with only what changed, under one idempotency key
 * (a UUID) per save: a retry after a failure sends the same key, so it
 * can't apply twice. It is the screen's one filled button.
 */
@Component({
  selector: 'app-adjust-vitals',
  imports: [MatButtonModule, MatIconModule, VitalsStepper],
  templateUrl: './adjust-vitals.html',
  styleUrl: './adjust-vitals.scss',
})
export class AdjustVitals {
  private readonly source = inject(LiveSessionSource);
  private readonly auth = inject(AuthService);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly dialogRef = inject<MatDialogRef<AdjustVitals, AdjustVitalsResult>>(
    MatDialogRef,
    {
      optional: true,
    },
  );
  private readonly sheetRef = inject<MatBottomSheetRef<AdjustVitals, AdjustVitalsResult>>(
    MatBottomSheetRef,
    { optional: true },
  );
  protected readonly data: AdjustVitalsData =
    inject<AdjustVitalsData | null>(MAT_DIALOG_DATA, { optional: true }) ??
    inject<AdjustVitalsData>(MAT_BOTTOM_SHEET_DATA);

  protected readonly v = this.data.vitals;
  protected readonly inSheet = this.sheetRef !== null;
  protected readonly maxTemporary = MAX_TEMPORARY_HP;
  protected readonly slotLevelLabel = slotLevelLabel;
  protected readonly note = whoSeesTheChange(this.data.playerName, this.data.vitals.name);

  private readonly initial = draftFrom(this.v);
  protected readonly hp = signal(this.initial.hitPointsCurrent);
  protected readonly temp = signal(this.initial.hitPointsTemporary);
  protected readonly hitDice = signal(this.initial.hitDiceUsed);
  protected readonly pact = signal(this.initial.pactSlotsUsed ?? 0);
  protected readonly slots = this.v.spellSlots.map((s) => ({
    level: s.level,
    total: s.total,
    used: signal(s.used),
  }));

  protected readonly resources = (this.v.resources ?? [])
    .filter((r) => r.total > 0)
    .map((r) => ({
      key: r.key,
      name: r.namePt || 'Recurso',
      total: r.total,
      used: signal(r.used),
    }));
  protected readonly beast = this.v.wildShape ?? null;
  protected readonly beastHp = signal(this.initial.wildShapeHitPoints ?? 0);

  protected readonly hitDiceHint = computed(
    () => `${this.v.hitDice}, ${usedWords(this.hitDice(), this.v.hitDiceTotal)}`,
  );

  protected readonly saveState = signal<SaveState>({ status: 'idle' });
  /** The save is in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileSaving = effect(() => {
    const saving = this.saveState().status === 'saving';
    if (this.dialogRef) {
      this.dialogRef.disableClose = saving;
    }
    if (this.sheetRef) {
      this.sheetRef.disableClose = saving;
    }
  });

  /** The idempotency key of the last correction tried, with its numbers:
   * trying the same numbers again reuses it (a retry); new numbers get a
   * new key (a new correction). */
  private lastTry: { key: string; change: VitalsChange } | null = null;

  protected readonly hpLabel = (step: number) =>
    step < 0 ? `Tirar ${-step} PV` : `Somar ${step} PV`;
  protected readonly tempLabel = (step: number) =>
    step < 0 ? 'Tirar 1 PV temporário' : 'Somar 1 PV temporário';
  protected readonly diceLabel = (step: number) =>
    step < 0 ? 'Devolver 1 dado de vida' : 'Usar 1 dado de vida';
  protected slotLabel(level: number): (step: number) => string {
    const name = slotLevelLabel(level);
    return (step) => (step < 0 ? `Devolver 1 espaço de ${name}` : `Usar 1 espaço de ${name}`);
  }
  protected readonly pactLabel = (step: number) =>
    step < 0 ? 'Devolver 1 espaço de pacto' : 'Usar 1 espaço de pacto';

  protected resourceLabel(name: string): (step: number) => string {
    return (step) => (step < 0 ? `Devolver 1 uso de ${name}` : `Usar 1 uso de ${name}`);
  }
  protected readonly beastLabel = (step: number) =>
    step < 0 ? 'Tirar 1 PV da fera' : 'Somar 1 PV da fera';

  protected usedWords = usedWords;

  private draft(): VitalsDraft {
    return {
      hitPointsCurrent: this.hp(),
      hitPointsTemporary: this.temp(),
      slotsUsed: Object.fromEntries(this.slots.map((s) => [s.level, s.used()])),
      pactSlotsUsed: this.v.pactSlots ? this.pact() : null,
      hitDiceUsed: this.hitDice(),
      resourcesUsed: Object.fromEntries(this.resources.map((r) => [r.key, r.used()])),
      wildShapeHitPoints: this.beast ? this.beastHp() : null,
    };
  }

  private allValid(): boolean {
    const whole = (n: number, max: number) => Number.isInteger(n) && n >= 0 && n <= max;
    return (
      whole(this.hp(), this.v.hitPointsMax) &&
      whole(this.temp(), MAX_TEMPORARY_HP) &&
      whole(this.hitDice(), this.v.hitDiceTotal) &&
      this.slots.every((s) => whole(s.used(), s.total)) &&
      this.resources.every((r) => whole(r.used(), r.total)) &&
      (!this.beast || whole(this.beastHp(), this.beast.hitPointsMax)) &&
      (!this.v.pactSlots || whole(this.pact(), this.v.pactSlots.total))
    );
  }

  protected async save(): Promise<void> {
    if (this.saveState().status === 'saving') {
      return;
    }
    if (!this.allValid()) {
      // Each field already says what's allowed; take the person there.
      this.host.nativeElement.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
      return;
    }
    const change = changeBetween(this.v, this.draft());
    if (!change) {
      this.close(undefined);
      return;
    }
    if (!this.lastTry || !sameChange(this.lastTry.change, change)) {
      this.lastTry = { key: crypto.randomUUID(), change };
    }
    this.saveState.set({ status: 'saving' });
    try {
      const vitals = await this.source.adjustVitals(
        this.data.campaignId,
        this.v.characterId,
        this.lastTry.key,
        change,
      );
      this.close({ kind: 'saved', vitals });
    } catch (err) {
      this.saveState.set(this.describe(err));
    }
  }

  private describe(err: unknown): SaveState {
    switch (this.source.classifyError(err)) {
      case 'no-session':
        return { status: 'ended' };
      case 'invalid':
        return {
          status: 'error',
          title: 'Algum número passou do limite da ficha.',
          detail: 'A ficha pode ter mudado. Feche, abra "Ajustar" de novo e confira os números.',
        };
      case 'no-access':
        return {
          status: 'error',
          title: `${this.v.name} não está mais no grupo.`,
          detail: 'O personagem morreu ou saiu da campanha. Feche e confira o grupo.',
        };
      case 'forbidden':
        return {
          status: 'error',
          title: 'Só o mestre da campanha ajusta esses números.',
          detail: '',
        };
      case 'signed-out':
        this.close(undefined);
        this.auth.signIn(location.pathname);
        return { status: 'idle' };
      default:
        return {
          status: 'error',
          title: 'Não foi possível salvar o ajuste.',
          detail: 'Confira a conexão e toque em "Salvar ajuste" de novo.',
        };
    }
  }

  protected close(result: AdjustVitalsResult | undefined): void {
    this.dialogRef?.close(result);
    this.sheetRef?.dismiss(result);
  }

  protected closeEnded(): void {
    this.close({ kind: 'ended' });
  }
}
