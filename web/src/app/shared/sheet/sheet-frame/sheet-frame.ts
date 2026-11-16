import {
  Component,
  DestroyRef,
  ElementRef,
  afterNextRender,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * The frame of the combat's sheets and dialogs that scroll inside
 * themselves (the cast sheet, Escudo, "Condições…", Retomar o Fôlego):
 * the grab bar on a phone, the title and what it is about, the close
 * button, a body that scrolls and a footer with the action buttons that
 * never scrolls away, so a tall list on a 667px phone still leaves
 * "Conjurar" in reach. The title is the first focus (`tabindex="-1"`,
 * see `openSheet`) and the name of the dialog (`titleId`).
 */
@Component({
  selector: 'app-sheet-frame',
  imports: [MatIconModule],
  template: `
    <div class="frame" [class.frame--phone]="phone()" [style.height]="height() || null">
      @if (phone() && handle()) {
        <span class="frame__handle" aria-hidden="true"></span>
      }
      <div class="frame__head">
        @if (icon()) {
          <mat-icon class="frame__icon" aria-hidden="true">{{ icon() }}</mat-icon>
        }
        <!-- A glyph the icon font does not have (the mount's saddle), in the icon's place. -->
        <ng-content select="[head-icon]" />
        <div class="frame__titles">
          <h2 class="frame__title" [class.frame__title--small]="small()" [id]="titleId()" tabindex="-1">{{ title() }}</h2>
        </div>
        <!-- An extra icon button before the close one (the cast sheet's "?"). -->
        <ng-content select="[head-action]" />
        @if (closable()) {
          <button type="button" class="frame__close" aria-label="Fechar" [attr.title]="small() ? 'Fechar' : null" (click)="closed.emit()">
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        }
      </div>
      @if (subtitle() || subtitleLead()) {
        <p class="frame__sub"><ng-content select="[sub-mark]" />@if (subtitleLead()) {<b class="frame__lead">{{ subtitleLead() }}</b>}{{ subtitle() }}</p>
      }
      <div #body class="frame__body" [class.frame__body--scrolls]="scrolls()" [attr.tabindex]="focusableBody() && scrolls() ? 0 : null"><ng-content /></div>
      <div class="frame__foot"><ng-content select="[foot]" /></div>
    </div>
  `,
  styleUrl: './sheet-frame.scss',
  host: { style: 'display: block' },
})
export class SheetFrame {
  readonly title = input.required<string>();
  readonly subtitle = input('');
  /** The subtitle's first words in bold, before `subtitle` ("Reação" of "■ Reação · desmontar · Rodada 4"); a `[sub-mark]` is drawn before them. */
  readonly subtitleLead = input('');
  readonly titleId = input('sheet-t');
  /** A leading icon before the title (Escudo's shield). */
  readonly icon = input('');
  /** The phone's bottom sheet: it draws its grab bar. */
  readonly phone = input(false);
  /** `false`: the phone's sheet draws no grab bar (the chooser of the conversation, whose board has none). */
  readonly handle = input(true);
  /** The title at 21/26 bold (the board's chooser) instead of the sheets' 26/30 display size; the close button then also has a tooltip. */
  readonly small = input(false);
  /** An alert dialog that must be answered has no close button. */
  readonly closable = input(true);
  /** A fixed height (a CSS length) for a dialog whose list filters: it does not jump as the results change. */
  readonly height = input('');

  /** A body that scrolls and holds nothing to focus (a question whose answers are in the footer) is a tab stop itself, so the keyboard can scroll it. */
  readonly focusableBody = input(false);

  readonly closed = output<void>();

  private readonly body = viewChild.required<ElementRef<HTMLElement>>('body');
  /** The body has more than fits: only then the shadows at its edges are drawn (a body that fits shows no stray line). */
  protected readonly scrolls = signal(false);

  constructor() {
    const destroyRef = inject(DestroyRef);
    afterNextRender(() => {
      const el = this.body().nativeElement;
      const check = () => this.scrolls.set(el.scrollHeight > el.clientHeight + 1);
      check();
      if (typeof ResizeObserver === 'function') {
        const observer = new ResizeObserver(check);
        observer.observe(el);
        Array.from(el.children).forEach((child) => observer.observe(child));
        destroyRef.onDestroy(() => observer.disconnect());
        // Content that shows up later (an `@if` block) is observed too, and the check runs again.
        if (typeof MutationObserver === 'function') {
          const mutations = new MutationObserver((records) => {
            records.forEach((r) =>
              r.addedNodes.forEach((n) => n instanceof Element && observer.observe(n)),
            );
            check();
          });
          mutations.observe(el, { childList: true });
          destroyRef.onDestroy(() => mutations.disconnect());
        }
      }
    });
  }

  /** A question that opens under the picture is brought into view with its buttons. */
  scrollToEnd(): void {
    const el = this.body().nativeElement;
    el.scrollTop = el.scrollHeight;
  }

  /** An error opens at the top of the scrolling body, where it is seen. */
  scrollToTop(): void {
    this.body().nativeElement.scrollTop = 0;
  }
}
