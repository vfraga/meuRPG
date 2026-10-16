import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { PuzzleKind } from '../../../../gen/meurpg/play/v1/puzzles_pb';
import { type Draft, clamped, resized } from '../../../core/puzzles/puzzle-draft';
import { moveWord } from '../../../core/puzzles/puzzle-format';
import { GLYPHS, pillarFaces } from '../../../core/puzzles/puzzle-symbols';
import { PillarsBoard } from '../../../shared/puzzle-boards/pillars-board';
import type { Turn } from '../../../shared/puzzle-boards/symbol-columns';
import { SymbolGlyph } from '../../../shared/puzzle-boards/symbol-glyph';
import { RevealSwitch } from '../../../shared/reveal-switch/reveal-switch';
import { Stepper } from '../fields/stepper';
import type { StartPreview } from './start-preview';

/**
 * The form of the turning symbols (E10-06 state 2): how many pillars (3 to 6) and symbols (3 to 6, the first of the table's
 * six, listed with their names), whether a pillar turns its neighbours too, the mural the players must copy (the master
 * turns it with the arrows) and the start the server drew from it. The start is a preview (no pillar is a control): "Gerar
 * outro começo" asks for another, and the fewest turns come from the server. If the links make the mural impossible to
 * reach, the server says so and the line under the preview carries it.
 */
@Component({
  selector: 'app-pillars-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, PillarsBoard, RevealSwitch, Stepper, SymbolGlyph],
  template: `
    <app-stepper label="Pilares" noun="pilar" [value]="draft().pillars" [min]="3" [max]="6" (valueChange)="setPillars($event)" />
    <app-stepper label="Símbolos" noun="símbolo" [value]="draft().symbols" [min]="3" [max]="6" (valueChange)="setSymbols($event)" />
    <ul class="glyphs" aria-label="Os seis símbolos da mesa">
      @for (g of glyphs; track g.key) {
        <li class="glyph"><app-symbol-glyph [face]="g.key" />{{ g.namePt }}</li>
      }
    </ul>
    <p class="help">Os {{ draft().symbols }} primeiros estão em uso. Cada giro leva ao próximo, na ordem acima.</p>

    <app-reveal-switch
      [label]="'Girar junto com os vizinhos · ' + (draft().linked ? 'Ligado' : 'Desligado')"
      hint="Girar um pilar gira também o da esquerda e o da direita."
      [checked]="draft().linked"
      (checkedChange)="patch.emit({ linked: $event })"
    />

    <section class="part" aria-labelledby="mural-title">
      <h3 class="part__title" id="mural-title">O mural (o alvo)</h3>
      <app-pillars-board mode="edit" label="O mural" [pillars]="draft().mural" [faces]="faces()" (turn)="turnMural($event)" />
    </section>

    <section class="part" aria-labelledby="pstart-title">
      <h3 class="part__title" id="pstart-title">O começo</h3>
      @switch (preview().status) {
        @case ('ready') {
          @if (startPillars(); as pillars) {
            <app-pillars-board mode="view" label="O começo dos pilares" [pillars]="pillars" [faces]="faces()" />
          }
          @if (preview().solvable) {
            <p class="line" role="status">Dá para resolver em <strong>{{ preview().moves }} {{ word() }}</strong>, no mínimo.</p>
          } @else {
            <p class="line line--bad" role="alert">Com essas ligações, os pilares não chegam ao mural. Mude o mural ou as ligações.</p>
          }
        }
        @case ('error') {
          <p class="line line--bad" role="alert">{{ preview().message }}</p>
        }
        @default {
          <p class="line" role="status">Sorteando um começo...</p>
        }
      }
      <button matButton="outlined" type="button" class="again" [disabled]="preview().status === 'loading'" disabledInteractive [class.mr-button--off]="preview().status === 'loading'" (click)="preview().status !== 'loading' && again.emit()">
        <mat-icon aria-hidden="true">shuffle</mat-icon>Gerar outro começo
      </button>
    </section>
  `,
  styleUrl: './pillars-form.scss',
})
export class PillarsForm {
  protected readonly glyphs = GLYPHS;
  readonly draft = input.required<Draft>();
  readonly preview = input.required<StartPreview>();
  readonly patch = output<Partial<Draft>>();
  readonly again = output<void>();

  protected readonly faces = computed(() => pillarFaces(this.draft().symbols));
  protected readonly startPillars = computed(() => {
    const kind = this.preview().start?.kind;
    return kind?.case === 'pillars' ? kind.value.pillars : null;
  });
  protected readonly word = computed(() => moveWord(PuzzleKind.PILLARS, this.preview().moves));

  protected setPillars(pillars: number): void {
    this.patch.emit({ pillars, mural: resized(this.draft().mural, pillars) });
  }

  protected setSymbols(symbols: number): void {
    this.patch.emit({ symbols, mural: clamped(this.draft().mural, symbols) });
  }

  protected turnMural(turn: Turn): void {
    const n = this.draft().symbols;
    this.patch.emit({
      mural: this.draft().mural.map((v, i) =>
        i === turn.index ? (((v + turn.delta) % n) + n) % n : v,
      ),
    });
  }
}
