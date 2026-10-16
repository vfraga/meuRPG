import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  ViewContainerRef,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  viewChild,
  viewChildren,
} from '@angular/core';
import { DOCUMENT } from '@angular/common';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatIconModule } from '@angular/material/icon';
import {
  NavigationCancel,
  NavigationEnd,
  NavigationError,
  NavigationStart,
  Router,
  RouterLink,
  RouterLinkActive,
  RouterOutlet,
} from '@angular/router';
import { filter } from 'rxjs';

import { AppFooter } from './shell/app-footer/app-footer';
import { LivePill } from './shared/live-pill/live-pill';
import { RollOverlay } from './shared/roll-overlay/roll-overlay';
import { LiveNotice } from './shell/live-notice/live-notice';
import { SessionNotes } from './shell/session-notes/session-notes';
import { OpenSessions, sessionForLiveLink } from './core/play/open-sessions';
import type { OpenSessionVm } from './core/play/open-sessions.types';
import { UserMenu } from './shell/user-menu/user-menu';

/**
 * The app shell: the top bar, the page and the footer.
 *
 * On a phone the bar shows only the name and a menu button; the button
 * opens the same links and the account controls in a panel under the bar
 * (docs/design.md#espaço-forma-e-layout). The panel is plain markup, not a
 * MatMenu: the CDK overlay would land in the initial bundle, which this
 * component always sits in. From 768px up, the links are simply inline and
 * the button is hidden.
 *
 * While a session of one of the person's campaigns is open, the bar shows
 * the "Ao vivo" link to it (next to the menu button on a phone, before the
 * account on wider screens), and the session notice sits above the page
 * (RN-06; `shell/live-notice`). Both read `OpenSessions`, whose poll loads
 * the generated PlayService client lazily, so neither adds it to the
 * initial bundle.
 *
 * On a player's session page the bar also shows "Anotações" (E8-06): the
 * page turns it on through `SessionNotes`, a small service with no generated
 * client, so the shell stays light.
 */
@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    MatIconModule,
    AppFooter,
    LivePill,
    LiveNotice,
    RollOverlay,
    UserMenu,
  ],
  templateUrl: './app.html',
  styleUrl: './app.scss',
  host: { '(document:keydown.escape)': 'menuOpen.set(false)' },
})
export class App {
  private readonly router = inject(Router);
  private readonly injector = inject(Injector);
  private readonly document = inject(DOCUMENT);
  /** The main region: the skip link's target, and the fallback for the focus after a navigation. */
  private readonly main = viewChild.required<ElementRef<HTMLElement>>('main');
  /** The path (no query, no fragment) of the page shown, to tell a new page from a changed filter. */
  private path = pathOf(this.router.url);
  /** The first navigation is the page load: the browser starts at the top, and focus stays where it is. */
  private firstNavigation = true;
  private readonly openSessions = inject(OpenSessions);
  protected readonly sessionNotes = inject(SessionNotes);
  /** Where the player's "Anotações" button goes (the session page hands over its template). */
  private readonly notesSlots = viewChildren('notesSlot', { read: ViewContainerRef });

  protected readonly menuOpen = signal(false);
  /** The current URL: the notice and the "Ao vivo" link hide on some pages. */
  protected readonly url = signal(this.router.url);
  /** A navigation is on its way (the first load included): the guard asks who is signed in and the page's code is fetched, with
   * nothing to draw meanwhile, so the shell says "Abrindo a página..." (it only shows after a short wait, in CSS). */
  protected readonly navigating = signal(true);
  protected readonly liveSession = computed(() =>
    sessionForLiveLink(this.openSessions.sessions(), this.url()),
  );

  constructor() {
    // The phone's slot is the first (beside the menu button), the wide one the second
    // (before the account); the template says which it is drawn in.
    effect(() => {
      const template = this.sessionNotes.bar();
      this.notesSlots().forEach((slot, i) => {
        slot.clear();
        if (template) {
          slot.createEmbeddedView(template, { $implicit: i === 0 ? 'phone' : 'wide' });
        }
      });
    });
    this.router.events.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((e) => {
      if (e instanceof NavigationStart) {
        this.navigating.set(true);
      } else if (
        e instanceof NavigationEnd ||
        e instanceof NavigationCancel ||
        e instanceof NavigationError
      ) {
        this.navigating.set(false);
      }
    });
    // Following a link from the open panel closes it.
    this.router.events
      .pipe(
        filter((e) => e instanceof NavigationEnd),
        takeUntilDestroyed(inject(DestroyRef)),
      )
      .subscribe((e) => {
        this.menuOpen.set(false);
        this.url.set(e.urlAfterRedirects);
        this.moveFocusToThePage(pathOf(e.urlAfterRedirects));
      });
  }

  /** "Pular para o conteúdo": a plain `#main` link would go through the router (the `<base href>` is `/`). */
  protected skipToContent(event: Event): void {
    event.preventDefault();
    this.main().nativeElement.focus();
  }

  /**
   * After a navigation to another page, focus goes to the page's heading, so a
   * screen reader reads where it landed and the next Tab starts inside the page
   * (WCAG 2.4.3). It does not move on the first load, on a change that only
   * touches the query or the fragment, when the page already put the focus
   * somewhere inside itself, or while a dialog or sheet holds the focus.
   */
  private moveFocusToThePage(path: string): void {
    const samePage = path === this.path;
    this.path = path;
    if (this.firstNavigation) {
      this.firstNavigation = false;
      return;
    }
    if (samePage) {
      return;
    }
    // After the page's first render, so the heading exists and the page had its turn to place the focus.
    afterNextRender(
      () => {
        const main = this.main().nativeElement;
        const active = this.document.activeElement;
        if (
          active instanceof HTMLElement &&
          (main.contains(active) || active.closest('.cdk-overlay-container'))
        ) {
          return;
        }
        const heading = main.querySelector<HTMLElement>('h1');
        if (heading && !heading.hasAttribute('tabindex')) {
          // Focusable by code only: it is not a stop for Tab (styles.scss draws no ring on it).
          heading.setAttribute('tabindex', '-1');
        }
        (heading ?? main).focus();
      },
      { injector: this.injector },
    );
  }

  /** The visible word comes first, then which session it opens. */
  protected liveLabel(session: OpenSessionVm): string {
    return `Ao vivo: sessão ${session.sessionNumber} de ${session.campaignName}`;
  }

  protected toggleMenu(): void {
    this.menuOpen.update((open) => !open);
  }
}

/** The URL without its query and fragment. */
function pathOf(url: string): string {
  return url.split(/[?#]/)[0];
}
