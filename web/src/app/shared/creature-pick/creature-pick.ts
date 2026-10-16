import { Component, DestroyRef, computed, inject, input, output, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { acAndHp, typeAndSize } from '../../core/creatures/bestiary-format';
import { bestiaryErrorMessage } from '../../core/creatures/bestiary-errors';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { joinDots } from '../../core/format/text';
import { CreatureArt } from '../creatures/creature-art';

let nextId = 0;

/**
 * "Adicionar criatura" (E10-09 state 1): a search field in the bestiary's words. Typing asks the server (the Portuguese or the
 * SRD's English name, after a short pause; an answer that is no longer for the text on screen is dropped) and the best
 * matches show under the field, each with its type, size, ND, and CA and PV. A combobox: the arrow keys move through the
 * matches, Enter adds the one in focus, Escape closes. Picking adds one of the creature and the field is clear again for the
 * next, with the focus kept in it.
 */
@Component({
  selector: 'app-creature-pick',
  imports: [CreatureArt, MatIconModule],
  template: `
    <label class="pick__field" [for]="id + '-in'">
      <span class="pick__label mr-label">Adicionar criatura</span>
      <span class="pick__box">
        <mat-icon aria-hidden="true">search</mat-icon>
        <input
          [id]="id + '-in'"
          type="text"
          role="combobox"
          autocomplete="off"
          aria-autocomplete="list"
          placeholder="Procure no Bestiário pelo nome"
          [attr.aria-expanded]="open()"
          [attr.aria-controls]="id + '-list'"
          [attr.aria-activedescendant]="active() >= 0 ? id + '-o' + active() : null"
          [disabled]="disabled()"
          [value]="text()"
          (input)="type($any($event.target).value)"
          (keydown)="key($event)"
          (blur)="close()"
        />
      </span>
    </label>
    <ul class="pick__list" role="listbox" [id]="id + '-list'" aria-label="Criaturas encontradas" [hidden]="!open()">
      @for (c of found(); track c.key; let i = $index) {
        <li
          role="option"
          class="pick__opt"
          [id]="id + '-o' + i"
          [class.pick__opt--on]="i === active()"
          [attr.aria-selected]="i === active()"
          (mousedown)="$event.preventDefault()"
          (click)="choose(c)"
        >
          <app-creature-art class="pick__art" [monsterKey]="c.key" [type]="c.type" />
          <span class="pick__n">
            <span class="pick__pt">{{ c.namePt }}</span>
            <span class="pick__s"><span lang="en">{{ c.name }}</span> · {{ meta(c) }}</span>
          </span>
        </li>
      }
    </ul>
    @if (open() && found().length === 0 && !busy()) {
      <p class="pick__none" role="status">{{ error() || 'Nenhuma criatura com “' + text().trim() + '”.' }}</p>
    }
    <p class="mr-visually-hidden" role="status" aria-live="polite">{{ announce() }}</p>
  `,
  styleUrl: './creature-pick.scss',
})
export class CreaturePick {
  private readonly client = inject(CreaturesClient);
  protected readonly id = `cp-${nextId++}`;

  readonly campaignId = input.required<string>();
  readonly disabled = input(false);
  readonly picked = output<CreatureSummary>();

  protected readonly text = signal('');
  protected readonly found = signal<readonly CreatureSummary[]>([]);
  protected readonly active = signal(-1);
  protected readonly open = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly announce = computed(() =>
    this.open() && this.text().trim() !== '' && !this.busy()
      ? `${this.found().length} criaturas encontradas`
      : '',
  );

  private timer: ReturnType<typeof setTimeout> | null = null;
  private seq = 0;

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      if (this.timer) {
        clearTimeout(this.timer);
      }
    });
  }

  protected meta(c: CreatureSummary): string {
    return joinDots([typeAndSize(c), `ND ${c.challengeRating}`, acAndHp(c)]);
  }

  protected type(value: string): void {
    this.text.set(value);
    this.error.set('');
    if (this.timer) {
      clearTimeout(this.timer);
    }
    if (value.trim() === '') {
      this.seq++;
      this.found.set([]);
      this.open.set(false);
      return;
    }
    this.open.set(true);
    this.busy.set(true);
    this.timer = setTimeout(() => void this.search(), 200);
  }

  private async search(): Promise<void> {
    const mine = ++this.seq;
    const query = this.text().trim();
    try {
      const res = await this.client.search(this.campaignId(), { query, pageSize: 8 });
      if (mine === this.seq) {
        this.found.set(res.creatures);
        this.active.set(res.creatures.length > 0 ? 0 : -1);
      }
    } catch (err) {
      if (mine === this.seq) {
        this.found.set([]);
        this.error.set(bestiaryErrorMessage(err, 'list'));
      }
    } finally {
      if (mine === this.seq) {
        this.busy.set(false);
      }
    }
  }

  protected key(event: KeyboardEvent): void {
    const n = this.found().length;
    switch (event.key) {
      case 'ArrowDown':
        if (this.open() && n > 0) {
          event.preventDefault();
          this.active.set((this.active() + 1) % n);
        }
        break;
      case 'ArrowUp':
        if (this.open() && n > 0) {
          event.preventDefault();
          this.active.set((this.active() - 1 + n) % n);
        }
        break;
      case 'Enter':
        if (this.open() && this.active() >= 0) {
          event.preventDefault();
          this.choose(this.found()[this.active()]);
        }
        break;
      case 'Escape':
        if (this.open()) {
          event.preventDefault();
          event.stopPropagation();
          this.close();
        }
        break;
    }
  }

  protected choose(creature: CreatureSummary): void {
    this.picked.emit(creature);
    this.text.set('');
    this.found.set([]);
    this.open.set(false);
    this.active.set(-1);
  }

  protected close(): void {
    this.open.set(false);
  }
}
