import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import {
  type CharacterCreature,
  CreatureSource,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { creatureErrorMessage } from '../../core/creatures/creature-errors';
import { CREATURE_NAME_MAX, nameCounter } from '../../core/creatures/creature-format';
import { focusWithRing } from '../../core/creatures/focus-ring';
import { tight } from '../../core/format/text';

/** What the card is asking: a new name, a dismissal, or (the master) the hit points. */
export type EditMode = 'rename' | 'dismiss' | 'hp';

let nextId = 0;

/**
 * The questions a creature's card asks in place (E9-10, quadro 3), never in a dialog:
 * - `rename`: the name, up to 40 characters, the counter at the right; one line only. The field
 *   takes the focus.
 * - `dismiss`: "Dispensar Nanquim?" with what it costs in words, "Voltar" and "Dispensar Nanquim"
 *   stacked and 48 px high; the focus starts on "Voltar" and shows its ring (an `alertdialog`, so a
 *   stray Enter never dismisses).
 * - `hp` (the master's correction outside a combat, RN-02): the hit points themselves, from 0 to the
 *   maximum; 0 sends the creature away, and the question says so.
 * The server decides everything; an error stays here, in words, and the question stays open. A button
 * that cannot act yet is the dashed one (`mr-button--off`) with the reason in a line under it.
 */
@Component({
  selector: 'app-creature-edit',
  imports: [FormsModule, MatButtonModule, MatFormFieldModule, MatIconModule, MatInputModule],
  template: `
    @let c = creature();
    @switch (mode()) {
      @case ('dismiss') {
        <div class="ask" role="alertdialog" [attr.aria-labelledby]="id + '-t'" [attr.aria-describedby]="id + '-d'">
          <p class="ask__t" [id]="id + '-t'"><mat-icon aria-hidden="true">warning</mat-icon><span>Dispensar {{ c.name }}?</span></p>
          <p class="ask__d" [id]="id + '-d'">{{ dismissText() }}</p>
          @if (error()) {
            <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
          }
          <button #first type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="closed.emit(false)">Voltar</button>
          <button type="button" matButton="outlined" class="big big--danger" [disabled]="busy()" (click)="dismiss()"><span class="lbl">Dispensar&nbsp;<span class="nm">{{ c.name }}</span></span></button>
        </div>
      }
      @case ('rename') {
        <form class="form" (ngSubmit)="rename()">
          <mat-form-field appearance="outline" subscriptSizing="dynamic">
            <mat-label>Nome da criatura</mat-label>
            <input #first matInput name="name" autocomplete="off" [maxlength]="max" [ngModel]="name()" (ngModelChange)="name.set($event)" />
            <mat-hint align="end">{{ counter() }}</mat-hint>
          </mat-form-field>
          @if (error()) {
            <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
          }
          <button type="submit" matButton="outlined" class="big" [class.mr-button--off]="!canSave()" [disabled]="busy() || !canSave()" disabledInteractive [attr.aria-describedby]="canSave() ? null : id + '-why'">Salvar o nome</button>
          @if (!canSave()) {
            <p class="why" [id]="id + '-why'">{{ renameWhy() }}</p>
          }
          <button type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="closed.emit(false)">Cancelar</button>
        </form>
      }
      @case ('hp') {
        <form class="form" (ngSubmit)="setHitPoints()">
          <mat-form-field appearance="outline" subscriptSizing="dynamic">
            <mat-label>PV de {{ c.name }}</mat-label>
            <input #first matInput name="hp" type="number" inputmode="numeric" min="0" [max]="c.hitPointsMax" autocomplete="off" [ngModel]="hp()" (ngModelChange)="hp.set($event)" />
            <mat-hint>De 0 a {{ c.hitPointsMax }}. Com 0, a criatura vai embora.</mat-hint>
          </mat-form-field>
          @if (error()) {
            <p class="err" role="alert"><mat-icon aria-hidden="true">error</mat-icon>{{ error() }}</p>
          }
          <button type="submit" matButton="outlined" class="big" [class.mr-button--off]="!hpValid()" [disabled]="busy() || !hpValid()" disabledInteractive [attr.aria-describedby]="hpValid() ? null : id + '-why'">Corrigir os PV</button>
          @if (!hpValid()) {
            <p class="why" [id]="id + '-why'">Escreva um número inteiro de 0 a {{ c.hitPointsMax }}.</p>
          }
          <button type="button" matButton="outlined" class="big" [disabled]="busy()" (click)="closed.emit(false)">Cancelar</button>
        </form>
      }
    }
  `,
  styleUrl: './creature-edit.scss',
})
export class CreatureEdit {
  private readonly client = inject(CreaturesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly creature = input.required<CharacterCreature>();
  readonly mode = input.required<EditMode>();
  /** Whose creature it is, for the sentence ("da sua ficha" or "da ficha de Pensantus"). */
  readonly ownerView = input(true);
  readonly ownerName = input('');
  /** Done: true when something changed (the list is read again), false when the person backed out. */
  readonly closed = output<boolean>();

  protected readonly id = `creature-edit-${nextId++}`;
  protected readonly max = CREATURE_NAME_MAX;
  protected readonly name = signal('');
  protected readonly hp = signal<number | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly first = viewChild('first', { read: ElementRef<HTMLElement> });

  protected readonly counter = computed(() => nameCounter(this.name().length, this.max));
  protected readonly canSave = computed(
    () => this.name().trim().length > 0 && this.name().trim() !== this.creature().name,
  );
  protected readonly renameWhy = computed(() =>
    this.name().trim().length === 0 ? 'Escreva um nome.' : 'Escreva um nome diferente do atual.',
  );
  protected readonly hpValid = computed(() => {
    const v = this.hp();
    return v !== null && Number.isInteger(v) && v >= 0 && v <= this.creature().hitPointsMax;
  });

  constructor() {
    afterNextRender(
      () => {
        this.name.set(this.creature().name);
        this.hp.set(this.creature().hitPointsCurrent);
        // The whole question comes into view under the sticky bar, then the focus goes in without scrolling again.
        this.host.nativeElement.scrollIntoView({ block: 'nearest' });
        const el = this.first()?.nativeElement;
        if (el instanceof HTMLInputElement) {
          el.focus({ preventScroll: true });
          // The field's value is written a tick later (ngModel): select it then, so a new name replaces the old at once.
          setTimeout(() => el.select());
        } else {
          focusWithRing(el);
        }
      },
      { injector: this.injector },
    );
  }

  /** What dismissing costs, by where the creature came from: a familiar is conjured again, a gift is given again. */
  protected dismissText(): string {
    const c = this.creature();
    const from = this.ownerView() ? 'da sua ficha' : `da ficha de ${this.ownerName()}`;
    const gone = `${c.name} some ${from} e do mapa.`;
    switch (c.source) {
      case CreatureSource.FAMILIAR:
        return tight(
          `${gone} Para ter um familiar de novo, é preciso conjurar Convocar Familiar outra vez (ritual de 1 hora).`,
        );
      case CreatureSource.MASTER:
        return `${gone} Para ter essa criatura de novo, ${this.ownerView() ? 'é preciso pedir ao mestre' : 'é preciso dar a criatura outra vez'}.`;
      default:
        return `${gone} Para ter essa criatura de novo, é preciso conjurar a magia outra vez.`;
    }
  }

  protected async rename(): Promise<void> {
    if (this.busy() || !this.canSave()) {
      return;
    }
    await this.run('rename', () =>
      this.client.rename(this.campaignId(), this.creature().id, this.name().trim()),
    );
  }

  protected async dismiss(): Promise<void> {
    await this.run('dismiss', () => this.client.dismiss(this.campaignId(), this.creature().id));
  }

  protected async setHitPoints(): Promise<void> {
    if (this.busy() || !this.hpValid()) {
      return;
    }
    await this.run('adjust', () =>
      this.client.setHitPoints(this.campaignId(), this.creature().id, this.hp() ?? 0),
    );
  }

  private async run(
    action: 'rename' | 'dismiss' | 'adjust',
    call: () => Promise<unknown>,
  ): Promise<void> {
    this.busy.set(true);
    this.error.set('');
    try {
      await call();
      this.closed.emit(true);
    } catch (err) {
      this.error.set(creatureErrorMessage(err, action));
    } finally {
      this.busy.set(false);
    }
  }
}
