import { type Injector, afterNextRender, signal } from '@angular/core';

import type { CenterOption } from './center-options';
import { listMove } from './area-moves';

/** What the "Centrar em…" list needs from the map. */
export interface CenterListHost {
  readOnly(): boolean;
  options(): readonly CenterOption[];
  /** A pick: places like a tap on that square; "Um quadrado do mapa" (no square) only hands the arrows the map. */
  choose(option: CenterOption): void;
  /** The element the focus goes to. */
  focusOf(where: 'list' | 'button' | 'map'): HTMLElement | undefined;
  readonly injector: Injector;
}

/** The listbox of "Centrar em…" ("Apontar para…"): open or not, the active option, and its keys, as the map's listbox. */
export class CenterList {
  readonly open = signal(false);
  readonly active = signal(0);

  constructor(private readonly host: CenterListHost) {}

  show(): void {
    if (this.host.readOnly()) {
      return;
    }
    this.open.set(true);
    this.active.set(0);
    this.focus('list');
  }

  toggle(): void {
    if (this.open()) {
      this.close(true);
    } else {
      this.show();
    }
  }

  close(toButton: boolean): void {
    this.open.set(false);
    this.focus(toButton ? 'button' : 'map');
  }

  keydown(event: KeyboardEvent): void {
    const options = this.host.options();
    const move = listMove(event.key, this.active(), options.length);
    if (move === null) {
      return;
    }
    event.preventDefault();
    if (move === 'choose') {
      this.pick(options[this.active()]);
    } else if (move === 'close') {
      event.stopPropagation();
      this.close(true);
    } else {
      this.active.set(move);
    }
  }

  /** A click on an option (the listbox handles the pointer for its options, which take no focus of their own). */
  click(event: MouseEvent): void {
    const i = Number((event.target as HTMLElement).closest<HTMLElement>('[data-i]')?.dataset['i']);
    const option = Number.isInteger(i) ? this.host.options()[i] : undefined;
    if (option) {
      this.pick(option);
    }
  }

  private pick(option: CenterOption): void {
    this.close(false);
    this.host.choose(option);
  }

  private focus(where: 'list' | 'button' | 'map'): void {
    afterNextRender(() => this.host.focusOf(where)?.focus(), { injector: this.host.injector });
  }
}
