import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import {
  type CatalogItem,
  type InventoryView,
  ItemGrantSchema,
  ItemKind,
} from '../../../../gen/meurpg/characters/v1/inventory_service_pb';
import { create } from '@bufbuild/protobuf';
import { ActionKey } from '../../../core/connect/idempotency';
import { InventoryClient } from '../../../core/inventory/inventory-client';
import { itemErrorMessage } from '../../../core/inventory/inventory-errors';
import { rarityLabel } from '../../../core/inventory/inventory-labels';
import { SpellsClient } from '../../../core/spells/spells-client';
import { CheckBox } from '../../../shared/check-box/check-box';
import { CountStepper } from '../../../shared/count-stepper/count-stepper';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

export interface AddItemSheetData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly characterName: string;
  /** The master gives any item of the game, and may leave it unidentified; a player adds equipment and text. */
  readonly isMaster: boolean;
}

const SHOWN = 24;

/** Lower case, no accents: how the search compares names. */
function plain(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

/**
 * "Adicionar item": the game's list (the SRD equipment, and for the master its magic items) found by
 * name, or a free-text line. A player adds equipment and text to their own sheet; the master gives
 * anything, picks the base of a generic weapon or armour, the spell of a scroll, and may leave the item
 * unidentified with the look the player will read (RN-10): identical unidentified items never share a
 * line. The server checks every rule again (`GiveItems`).
 */
@Component({
  selector: 'app-add-item-sheet',
  imports: [
    CheckBox,
    CountStepper,
    FormsModule,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    SheetFrame,
  ],
  template: `
    <app-sheet-frame
      [title]="data.isMaster ? 'Dar um item' : 'Adicionar item'"
      titleId="sheet-t"
      [subtitle]="data.isMaster ? 'Para ' + data.characterName : 'Da lista do jogo ou texto livre'"
      [phone]="handle.inSheet"
      height="min(640px, calc(100dvh - 120px))"
      (closed)="handle.close()"
    >
      <div class="body">
        @if (!free()) {
          <mat-form-field appearance="outline" subscriptSizing="dynamic">
            <mat-label>Procurar</mat-label>
            <input matInput name="query" autocomplete="off" [ngModel]="query()" (ngModelChange)="query.set($event)" />
          </mat-form-field>
          @if (loadFailed()) {
            <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>Não deu para ler a lista do jogo. Escreva o item como texto livre ou tente de novo.</p>
            <button type="button" matButton="outlined" (click)="load()">Tentar de novo</button>
          } @else if (!catalog()) {
            <p class="lead" role="status">Carregando a lista…</p>
          } @else {
            <ul class="list" aria-label="Itens da lista">
              @for (c of shown(); track c.key) {
                <li>
                  <button type="button" class="pick" [class.pick--on]="picked()?.key === c.key" [attr.aria-pressed]="picked()?.key === c.key" (click)="pick(c)">
                    <span class="pick__name">{{ c.namePt }}</span>
                    <span class="pick__sub">{{ sub(c) }}</span>
                  </button>
                </li>
              } @empty {
                <li class="lead">Nenhum item com esse nome.</li>
              }
            </ul>
          }
        } @else {
          <mat-form-field appearance="outline" subscriptSizing="dynamic">
            <mat-label>Nome do item</mat-label>
            <input matInput name="free" autocomplete="off" maxlength="200" [ngModel]="freeName()" (ngModelChange)="freeName.set($event)" />
          </mat-form-field>
        }

        @if (picked(); as p) {
          @if (p.isFamily) {
            <p class="lead">Este item tem versões. Escolha uma:</p>
            <ul class="list" aria-label="Versões">
              @for (v of variantsOf(p); track v.key) {
                <li>
                  <button type="button" class="pick" [class.pick--on]="variant()?.key === v.key" [attr.aria-pressed]="variant()?.key === v.key" (click)="variant.set(v)">
                    <span class="pick__name">{{ v.namePt }}</span>
                  </button>
                </li>
              }
            </ul>
          }
          @if (target(); as t) {
            @if (t.baseChoices.length > 0) {
              <mat-form-field appearance="outline" subscriptSizing="dynamic">
                <mat-label>Em qual {{ t.kind === 3 ? 'arma ou armadura' : 'item' }}</mat-label>
                <select matNativeControl name="base" [ngModel]="base()" (ngModelChange)="base.set($event)">
                  <option value="">Escolher</option>
                  @for (b of baseNames(t); track b.key) {
                    <option [value]="b.key">{{ b.name }}</option>
                  }
                </select>
              </mat-form-field>
            }
            @if (t.optionDamageTypes.length > 0) {
              <mat-form-field appearance="outline" subscriptSizing="dynamic">
                <mat-label>Tipo de dano</mat-label>
                <select matNativeControl name="option" [ngModel]="option()" (ngModelChange)="option.set($event)">
                  <option value="">Escolher</option>
                  @for (d of t.optionDamageTypes; track d) {
                    <option [value]="d">{{ damageName(d) }}</option>
                  }
                </select>
              </mat-form-field>
            }
            @if (t.scrollLevel >= 0) {
              <mat-form-field appearance="outline" subscriptSizing="dynamic">
                <mat-label>Magia do pergaminho</mat-label>
                <select matNativeControl name="spell" [ngModel]="spell()" (ngModelChange)="spell.set($event)">
                  <option value="">Escolher</option>
                  @for (s of spells(); track s.key) {
                    <option [value]="s.key">{{ s.name }}</option>
                  }
                </select>
              </mat-form-field>
            }
            @if (data.isMaster) {
              <div class="row">
                <span id="unid-l">Não identificado</span>
                <app-check-box [checked]="unidentified()" label="Dar como não identificado" (toggle)="unidentified.set(!unidentified())" />
              </div>
              @if (unidentified()) {
                <mat-form-field appearance="outline" subscriptSizing="dynamic">
                  <mat-label>Aspecto (o que o jogador vê)</mat-label>
                  <input matInput name="look" autocomplete="off" maxlength="200" [ngModel]="look()" (ngModelChange)="look.set($event)" />
                  <mat-hint>Itens iguais e não identificados nunca se juntam numa pilha, para não revelar que são o mesmo.</mat-hint>
                </mat-form-field>
              }
            }
          }
        }

        @if (canCount()) {
          <div class="row">
            <span id="qty-l">Quantidade</span>
            <app-count-stepper [value]="quantity()" [min]="1" [max]="99" [noun]="free() ? freeName() || 'item' : picked()?.namePt || 'item'" (valueChange)="quantity.set($event)" />
          </div>
        }

        @if (error()) {
          <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
        }
        <button type="button" class="swap" matButton="text" (click)="toggleFree()">{{ free() ? 'Escolher da lista do jogo' : 'Escrever um item que não está na lista' }}</button>
      </div>
      <div foot class="foot">
        <button type="button" matButton="filled" class="big" [disabled]="busy()" [class.mr-button--off]="!ready()" disabledInteractive [attr.aria-describedby]="ready() ? null : 'why'" (click)="add()">
          {{ label() }}
        </button>
        @if (!ready()) {
          <p class="why" id="why">{{ why() }}</p>
        }
        <button type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="handle.close()">Cancelar</button>
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './item-sheets.scss',
  styles: `
    .pick {
      display: flex;
      flex-direction: column;
      width: 100%;
      min-height: 48px;
      box-sizing: border-box;
      padding: var(--mr-space-2) var(--mr-space-3);
      border: 1.5px solid var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
      color: var(--mr-ink);
      text-align: left;
      cursor: pointer;

      &:focus-visible {
        outline: 3px solid var(--mr-focus);
        outline-offset: 2px;
      }
    }

    .pick--on {
      border-color: var(--mr-accent);
      box-shadow: inset 0 0 0 1px var(--mr-accent);
    }

    .pick__name {
      font-weight: 700;
    }

    .pick__sub {
      font-size: 14px;
      color: var(--mr-ink-muted);
    }

    .swap {
      align-self: flex-start;
    }
  `,
})
export class AddItemSheet {
  protected readonly handle = injectSheet<AddItemSheetData, InventoryView>();
  protected readonly data = this.handle.data;
  private readonly client = inject(InventoryClient);
  private readonly spellsClient = inject(SpellsClient);
  private readonly key = new ActionKey();

  protected readonly catalog = signal<readonly CatalogItem[] | null>(null);
  protected readonly loadFailed = signal(false);
  protected readonly query = signal('');
  protected readonly free = signal(false);
  protected readonly freeName = signal('');
  protected readonly picked = signal<CatalogItem | null>(null);
  protected readonly variant = signal<CatalogItem | null>(null);
  protected readonly base = signal('');
  protected readonly option = signal('');
  protected readonly spell = signal('');
  protected readonly spells = signal<readonly { key: string; name: string }[]>([]);
  protected readonly quantity = signal(1);
  protected readonly unidentified = signal(false);
  protected readonly look = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal('');

  /** What the grant is about: the family's chosen version, or the item itself. */
  protected readonly target = computed(() => {
    const p = this.picked();
    return p?.isFamily ? this.variant() : p;
  });

  protected readonly shown = computed(() => {
    const all = this.catalog() ?? [];
    const q = plain(this.query().trim());
    const allowed = all.filter((c) => this.data.isMaster || c.kind === ItemKind.EQUIPMENT);
    const found = q === '' ? allowed : allowed.filter((c) => plain(c.namePt).includes(q) || plain(c.name).includes(q));
    return found.slice(0, SHOWN);
  });

  protected readonly canCount = computed(() => {
    if (this.free()) {
      return true;
    }
    const t = this.target();
    return t !== null && t.kind !== ItemKind.MAGIC ? true : (t?.stackable ?? false);
  });

  protected readonly ready = computed(() => this.why() === '');

  protected readonly why = computed(() => {
    if (this.free()) {
      return this.freeName().trim() === '' ? 'Escreva o nome do item.' : '';
    }
    const p = this.picked();
    if (!p) {
      return 'Escolha um item da lista.';
    }
    const t = this.target();
    if (!t) {
      return 'Escolha uma versão.';
    }
    if (t.baseChoices.length > 0 && this.base() === '') {
      return 'Escolha o item de base.';
    }
    if (t.optionDamageTypes.length > 0 && this.option() === '') {
      return 'Escolha o tipo de dano.';
    }
    if (t.scrollLevel >= 0 && this.spell() === '') {
      return 'Escolha a magia do pergaminho.';
    }
    if (this.unidentified() && this.look().trim() === '') {
      return 'Escreva o aspecto: o que o jogador vê.';
    }
    return '';
  });

  protected readonly label = computed(() => {
    const name = this.free() ? this.freeName().trim() : (this.target()?.namePt ?? this.picked()?.namePt ?? '');
    const verb = this.data.isMaster ? 'Dar' : 'Adicionar';
    return name ? `${verb} ${name}` : verb;
  });

  constructor() {
    void this.load();
  }

  protected async load(): Promise<void> {
    this.loadFailed.set(false);
    try {
      this.catalog.set(await this.client.catalog(this.data.campaignId));
    } catch {
      this.loadFailed.set(true);
    }
  }

  protected sub(c: CatalogItem): string {
    return [c.category, c.rarity ? rarityLabel(c.rarity) : ''].filter(Boolean).join(' · ');
  }

  protected variantsOf(p: CatalogItem): readonly CatalogItem[] {
    const all = this.catalog() ?? [];
    return p.variants.map((k) => all.find((c) => c.key === k)).filter((c): c is CatalogItem => !!c);
  }

  protected baseNames(t: CatalogItem): readonly { key: string; name: string }[] {
    const all = this.catalog() ?? [];
    return t.baseChoices.map((k) => ({ key: k, name: all.find((c) => c.key === k)?.namePt ?? k }));
  }

  protected damageName(key: string): string {
    return key.replace('damage-type:', '');
  }

  protected pick(c: CatalogItem): void {
    this.picked.set(c);
    this.variant.set(null);
    this.base.set('');
    this.option.set('');
    this.spell.set('');
    this.quantity.set(c.packQuantity > 0 ? 1 : 1);
    if (c.scrollLevel >= 0) {
      void this.loadSpells(c.scrollLevel);
    }
  }

  private async loadSpells(level: number): Promise<void> {
    try {
      const res = await this.spellsClient.list({ campaignId: this.data.campaignId, levels: [level], pageSize: 400 });
      this.spells.set(res.spells.map((s) => ({ key: s.key, name: s.namePt })));
    } catch {
      this.spells.set([]);
    }
  }

  protected toggleFree(): void {
    this.free.update((f) => !f);
    this.error.set('');
  }

  protected async add(): Promise<void> {
    if (!this.ready() || this.busy()) {
      return;
    }
    const grant = this.free()
      ? create(ItemGrantSchema, { name: this.freeName().trim(), quantity: this.quantity() })
      : create(ItemGrantSchema, {
          catalogKey: this.target()!.key,
          baseKey: this.base(),
          optionKey: this.option(),
          scrollSpellKey: this.spell(),
          quantity: this.canCount() ? this.quantity() : 1,
          unidentified: this.data.isMaster && this.unidentified(),
          look: this.data.isMaster && this.unidentified() ? this.look().trim() : '',
        });
    this.busy.set(true);
    this.handle.lock(true);
    this.error.set('');
    try {
      const view = await this.client.give(
        this.data.campaignId,
        this.data.characterId,
        [grant],
        this.key.keyFor({ ...grant }),
      );
      this.key.renew();
      this.handle.close(view);
    } catch (err) {
      this.error.set(itemErrorMessage(err, 'give'));
    } finally {
      this.busy.set(false);
      this.handle.lock(false);
    }
  }
}
