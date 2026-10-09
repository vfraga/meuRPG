import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { Coins } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { InventoryView } from '../../../../gen/meurpg/characters/v1/inventory_service_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { InventoryClient } from '../../../core/inventory/inventory-client';
import { itemErrorMessage } from '../../../core/inventory/inventory-errors';
import { COIN_FIELDS, COIN_MAX, type CoinKey } from '../../../core/inventory/inventory-labels';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

export interface CoinsSheetData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly characterName: string;
  readonly coins: Coins | undefined;
}

/**
 * "Editar moedas": the five coins of the purse, each a whole number from 0 to the cap (SRD 5.1
 * "Coins": 50 coins weigh a pound, the app counts and does not weigh). The owner and the master edit;
 * the history says "<name> ajustou as moedas". The server saves the numbers as typed (`SetCoins`).
 */
@Component({
  selector: 'app-coins-sheet',
  imports: [FormsModule, MatButtonModule, MatFormFieldModule, MatIconModule, MatInputModule, SheetFrame],
  template: `
    <app-sheet-frame title="Editar moedas" titleId="sheet-t" [subtitle]="sub" [phone]="handle.inSheet" (closed)="handle.close()">
      <form id="coins-form" class="body" (ngSubmit)="save()">
        <fieldset class="fields">
          <legend class="mr-visually-hidden">Moedas de {{ data.characterName }}</legend>
          @for (c of fields; track c.key) {
            <mat-form-field appearance="outline" subscriptSizing="dynamic">
              <mat-label>{{ c.label }}</mat-label>
              <input
                matInput
                type="number"
                inputmode="numeric"
                min="0"
                [max]="max"
                autocomplete="off"
                [name]="c.key"
                [ngModel]="values()[c.key]"
                (ngModelChange)="set(c.key, $event)"
              />
            </mat-form-field>
          }
        </fieldset>
        <p class="why">Cada campo é um número inteiro, de 0 a {{ maxText }}. O registro mostra “{{ data.characterName }} ajustou as moedas”.</p>
        @if (error()) {
          <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
        }
      </form>
      <div foot class="foot">
        <button type="submit" form="coins-form" matButton="filled" class="big" [disabled]="busy()" [class.mr-button--off]="!valid()" disabledInteractive>Salvar</button>
        <button type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="handle.close()">Cancelar</button>
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './item-sheets.scss',
})
export class CoinsSheet {
  protected readonly handle = injectSheet<CoinsSheetData, InventoryView>();
  protected readonly data = this.handle.data;
  private readonly client = inject(InventoryClient);
  private readonly key = new ActionKey();

  protected readonly fields = COIN_FIELDS;
  protected readonly max = COIN_MAX;
  protected readonly maxText = COIN_MAX.toLocaleString('pt-BR');
  protected readonly sub = `${this.data.characterName} · só o dono e o mestre editam`;
  protected readonly values = signal<Record<CoinKey, number | null>>({
    copper: this.data.coins?.copper ?? 0,
    silver: this.data.coins?.silver ?? 0,
    electrum: this.data.coins?.electrum ?? 0,
    gold: this.data.coins?.gold ?? 0,
    platinum: this.data.coins?.platinum ?? 0,
  });
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly valid = computed(() =>
    Object.values(this.values()).every(
      (v) => v !== null && Number.isInteger(v) && v >= 0 && v <= COIN_MAX,
    ),
  );

  protected set(key: CoinKey, value: number | null): void {
    this.values.update((v) => ({ ...v, [key]: value }));
  }

  protected async save(): Promise<void> {
    if (!this.valid()) {
      this.error.set(`Escreva números inteiros de 0 a ${this.maxText} em cada moeda.`);
      return;
    }
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.handle.lock(true);
    this.error.set('');
    const v = this.values() as Record<CoinKey, number>;
    try {
      const view = await this.client.setCoins(
        this.data.campaignId,
        this.data.characterId,
        v,
        this.key.keyFor(v),
      );
      this.key.renew();
      this.handle.close(view);
    } catch (err) {
      this.error.set(itemErrorMessage(err, 'coins'));
    } finally {
      this.busy.set(false);
      this.handle.lock(false);
    }
  }
}
