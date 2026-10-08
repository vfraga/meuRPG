import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { DiceRoll } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { type DamageDie, CombatClient } from '../../../../core/combat/combat-client';
import { rollFormula } from '../../../../core/combat/combat-dice';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { RollPicker } from '../roll-picker/roll-picker';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { injectSheet } from '../sheet-host';

/** What the page hands the feature sheet. */
export interface FeatureSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatantId: string;
  readonly actionKey: string;
  readonly name: string;
  /** "Ação bônus": the part of the turn it costs. */
  readonly cost: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
}

/**
 * A feature that rolls (Retomar o Fôlego: 1d10 plus the fighter's level, MR-014):
 * the d10 in the two ways of rolling (RN-18) and, once rolled, what it healed.
 * The key is made once, so a tap repeated after a lost answer never spends a
 * second use. A bottom sheet on a phone and a dialog from a tablet up.
 */
@Component({
  selector: 'app-feature-sheet',
  imports: [MatButtonModule, MatIconModule, RollPicker, SheetFrame],
  template: `
    <app-sheet-frame [title]="title()" [subtitle]="data.cost" [phone]="inSheet" (closed)="close()">
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @if (used()) {
        <div class="res" role="status" aria-live="polite">
          <span class="res__sum">{{ healed() }} PV recuperados</span>
          @if (roll(); as r) {
            <span class="res__line">{{ formula() }}{{ r.physical ? ' · dado físico' : '' }}</span>
          }
          <span class="res__line">Sua {{ data.cost.toLowerCase() }} foi usada.</span>
        </div>
      } @else {
        <app-roll-picker
          [canApp]="canApp"
          [canType]="canType"
          [preferApp]="preferApp"
          [label]="'Role 1d10 para ' + data.name"
          hint="Digite o número do dado (1 a 10). O app soma o seu nível de guerreiro."
          totalNote="Dado"
          [min]="1"
          [max]="10"
          [busy]="busy()"
          [appLabel]="'Rolar 1d10 no app'"
          [sticky]="true"
          [(typing)]="typing"
          (app)="use({ inApp: true })"
          (typed)="use({ sum: $event })"
        />
      }
      @if (used()) {
        <div foot>
          <button #back mat-flat-button type="button" class="done" (click)="close()">Voltar à sua vez</button>
        </div>
      }
    </app-sheet-frame>
  `,
  styles: `
    .res {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    .res__sum {
      font-family: var(--mr-font-display);
      font-size: 26px;
      font-weight: 800;
      line-height: 30px;
    }

    .res__line {
      font-size: 15px;
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .done {
      --mat-button-filled-container-height: 48px;
      width: 100%;
    }
  `,
})
export class FeatureSheet {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<FeatureSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly roll = signal<DiceRoll | null>(null);
  protected readonly used = signal(false);
  protected readonly healed = signal(0);
  private readonly key = new ActionKey();
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly title = computed(() =>
    this.typing() ? 'Digite o resultado do dado' : this.data.name,
  );
  protected readonly formula = computed(() => {
    const r = this.roll();
    return r ? `${rollFormula(r)} de cura` : '';
  });

  constructor() {
    effect(() => this.back()?.nativeElement.focus());
    // A use in the air cannot be dismissed: its answer is always shown.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected async use(die: DamageDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.takeAction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.combatantId,
        this.data.actionKey,
        die,
        this.key.keyFor([this.data.actionKey, die]),
      );
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.roll.set(res.roll ?? null);
      this.used.set(true);
      this.healed.set(res.healed ?? res.roll?.total ?? 0);
      this.typing.set(false);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'usar a habilidade'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.used());
  }
}
