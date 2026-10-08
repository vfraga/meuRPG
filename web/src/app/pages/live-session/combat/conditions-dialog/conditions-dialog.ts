import { Component, computed, effect, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { CONDITIONS, sameKeys } from '../../../../core/combat/conditions';
import { SheetFrame } from '../sheet-frame/sheet-frame';
import { injectSheet } from '../sheet-host';

/** What the page hands "Condições…". */
export interface ConditionsData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatant: Combatant;
  readonly state: CombatState;
}

/**
 * "Condições de Goblin 2" (RN-22, E6-29): the master marks the SRD's 15
 * conditions on a combatant as labels (the app applies no effect, it only
 * reminds the table) and ends the concentration it holds. A dialog from a
 * tablet up, a bottom sheet on a phone; the boxes are in three columns (two on a
 * phone), each row 44px, and the sheet scrolls inside itself with "Cancelar"
 * and "Salvar condições" always in reach. "Encerrar concentração" acts at once
 * (it takes the mark off the combatant), "Salvar condições" only saves the
 * boxes. The players see the conditions of whoever they see.
 */
@Component({
  selector: 'app-conditions-dialog',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  template: `
    <app-sheet-frame
      [title]="'Condições de ' + current().label"
      [phone]="inSheet"
      (closed)="close()"
    >
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      <p class="note">Só rótulos: o app não aplica os efeitos. Os jogadores veem as condições de quem eles veem.</p>
      <fieldset class="conds">
        <legend class="cap">Condições</legend>
        <div class="grid">
          @for (c of all; track c.key) {
            <label class="cond">
              <input type="checkbox" [checked]="chosen().has(c.key)" (change)="toggle(c.key)" />
              <span class="cond__box" aria-hidden="true">
                @if (chosen().has(c.key)) {
                  <mat-icon>check</mat-icon>
                }
              </span>
              <span class="cond__name">{{ c.name }}</span>
            </label>
          }
        </div>
      </fieldset>
      <section class="conc" aria-labelledby="conc-t">
        <span class="cap" id="conc-t">Concentração</span>
        <div class="conc__row">
          @if (current().concentrationSpell) {
            <span>Concentrado em <b>{{ spell() }}</b></span>
            <button mat-button type="button" class="conc__end" [disabled]="busy()" (click)="endConcentration()">
              Encerrar concentração
            </button>
          } @else {
            <span class="muted">Não está concentrado em nenhuma magia.</span>
          }
        </div>
      </section>
      <div foot>
        <div class="pair">
          <button mat-stroked-button type="button" class="pair__btn" [disabled]="busy()" (click)="close()">Cancelar</button>
          <button
            mat-flat-button
            type="button"
            class="pair__btn"
            [disabled]="busy() || !changed()"
            disabledInteractive
            (click)="save()"
          >
            Salvar condições
          </button>
        </div>
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './conditions-dialog.scss',
})
export class ConditionsDialog {
  private readonly api = inject(CombatClient);
  private readonly sheet = injectSheet<ConditionsData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly all = CONDITIONS;

  /** What the master marked and unmarked here: their changes, not a copy of the list. */
  private readonly added = signal<ReadonlySet<string>>(new Set());
  private readonly removed = signal<ReadonlySet<string>>(new Set());
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  /** The combatant as the combat has it now (the concentration may end meanwhile). */
  protected readonly current = computed(
    () =>
      this.data.state.encounter()?.combatants.find((c) => c.id === this.data.combatant.id) ??
      this.data.combatant,
  );
  /** The boxes: the conditions the combat has now, with the master's changes on top. */
  protected readonly chosen = computed<ReadonlySet<string>>(() => {
    const out = new Set(this.current().conditions);
    for (const k of this.removed()) {
      out.delete(k);
    }
    for (const k of this.added()) {
      out.add(k);
    }
    return out;
  });
  protected readonly spell = computed(() => this.current().concentrationSpellNamePt);
  protected readonly changed = computed(
    () => !sameKeys([...this.chosen()], this.current().conditions),
  );
  private readonly key = new ActionKey();

  constructor() {
    // A request in the air cannot be dismissed (Esc, the backdrop, ✕, Cancelar): its answer is always shown.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected toggle(key: string): void {
    const on = !this.chosen().has(key);
    const added = new Set(this.added());
    const removed = new Set(this.removed());
    added.delete(key);
    removed.delete(key);
    (on ? added : removed).add(key);
    this.added.set(added);
    this.removed.set(removed);
  }

  /** The keys to save: the ones already marked keep their order, the new ones follow. */
  private keys(): string[] {
    const chosen = this.chosen();
    const kept = this.current().conditions.filter((k) => chosen.has(k));
    return [
      ...kept,
      ...CONDITIONS.map((c) => c.key).filter((k) => chosen.has(k) && !kept.includes(k)),
    ];
  }

  protected save(): Promise<void> {
    return this.send({ keys: this.keys() }, true);
  }

  protected endConcentration(): Promise<void> {
    return this.send({ endConcentration: true }, false);
  }

  private async send(
    change: { keys?: string[]; endConcentration?: boolean },
    closes: boolean,
  ): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const key = this.key.keyFor([this.data.combatant.id, change]);
      this.data.state.apply(
        await this.api.setConditions(
          this.data.campaignId,
          this.data.encounterId,
          this.data.combatant.id,
          change,
          key,
        ),
      );
      this.key.renew();
      if (closes) {
        this.sheet.close(true);
      }
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'mudar as condições'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(false);
  }
}
