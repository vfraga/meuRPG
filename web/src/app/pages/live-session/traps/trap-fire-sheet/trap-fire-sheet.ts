import {
  Component,
  type Signal,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import type { MapPoint } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import type { TrapFiring } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { trapErrorMessage } from '../../../../core/traps/trap-errors';
import { firstLine } from '../../../../core/traps/trap-text';
import { PairFoot } from '../../../../shared/pair-foot/pair-foot';
import { type PickRow, PersonPick } from '../../../../shared/person-pick/person-pick';
import { SheetFrame } from '../../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../combat/sheet-host';

/** What the trap card hands "Disparar…". */
export interface TrapFireData {
  readonly campaignId: string;
  readonly mapId: string;
  readonly point: MapPoint;
  /** Who can be caught: outside a combat the characters with a token on the map (ID of the character), in a
   * combat the combatants (ID of the combatant). The server decides who stands in the area; this list only
   * lets the master pick someone else. A signal, `null` while it is still being read: the dialog may open before the
   * trap's "Quem notaria" answer arrives, and the list fills in when it does (a snapshot taken at the click stayed empty). */
  readonly targets: Signal<readonly PickRow[] | null>;
  /** The firing to add creatures to (a trap that fired already); empty for a new firing. */
  readonly extendFiringId: string;
  /** "Quem está no mapa" could not be read: the list says so instead of "Ninguém com token no mapa". */
  readonly targetsFailed?: Signal<boolean>;
}

/** "Disparar…": a dialog from a tablet up and a bottom sheet on a phone; it answers the firing, or `undefined`. */
export function openTrapFire(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: TrapFireData,
): Observable<TrapFiring | undefined> {
  return openSheet<TrapFireSheet, TrapFireData, TrapFiring>(dialog, bottomSheet, TrapFireSheet, {
    data,
    ariaLabel: data.extendFiringId ? 'Pegar mais gente na armadilha' : 'Disparar a armadilha',
    labelledBy: 'trap-fire-t',
    width: '520px',
    restoreFocus: false, // the opener takes it back with the focus ring
  });
}

/** The button's words: "Disparar para quem está na área" (nobody picked), "Disparar para Toren", "Disparar para 2 personagens". */
export function fireLabel(picked: readonly string[], extend: boolean): string {
  if (extend) {
    return picked.length === 0
      ? 'Incluir'
      : `Incluir ${picked.length === 1 ? picked[0] : `${picked.length} personagens`}`;
  }
  if (picked.length === 0) {
    return 'Disparar para quem está na área';
  }
  return picked.length === 1
    ? `Disparar para ${picked[0]}`
    : `Disparar para ${picked.length} personagens`;
}

/**
 * "Disparar o …" (E9-08 3, MR-035): the master fires a trap by hand and picks who was caught. **The server
 * knows who stands in the area** (it reads the tokens and the combat), and the app never works it out: with
 * nobody checked the trap fires for whoever is in its area, and the button says so; to catch someone else
 * the master checks them ("Disparar para Toren", "Disparar para 2 personagens"). The same sheet adds
 * creatures to a firing that already happened (`extend_firing_id`), where at least one must be checked.
 * The effect and the dice are the server's; the damage to a player's character waits for the master.
 */
@Component({
  selector: 'app-trap-fire-sheet',
  imports: [MatIconModule, PairFoot, PersonPick, SheetFrame],
  template: `
    <app-sheet-frame [title]="title" titleId="trap-fire-t" [subtitle]="subtitle" [phone]="inSheet" (closed)="cancel()">
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      <p class="lead">
        {{ extend ? 'Quem mais foi pego? O app rola o efeito e o dano para quem você marcar.' : 'O app rola o efeito e o dano. O dano de um personagem espera você aplicar.' }}
      </p>
      <app-person-pick headingId="trap-fire-who" [heading]="extend ? 'Quem mais foi pego' : 'Quem foi pego'" [rows]="rows()" [picked]="picked()" (pickedChange)="pick($event)" [empty]="emptyText()" />
      <p class="note" role="status" aria-live="polite">{{ note() }}</p>
      <app-pair-foot
        foot
        [confirmLabel]="label()"
        [ready]="ready()"
        [busy]="busy()"
        (cancel)="cancel()"
        (confirm)="confirm()"
      />
    </app-sheet-frame>
  `,
  styles: `
    .lead {
      margin: 0 0 var(--mr-space-3);
      font-size: 15px;
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .note {
      margin: var(--mr-space-3) 0 0;
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class TrapFireSheet {
  private readonly api = inject(TrapsClient);
  private readonly sheet = injectSheet<TrapFireData, TrapFiring>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  private readonly frame = viewChild(SheetFrame);
  private readonly key = new ActionKey();

  protected readonly extend = this.data.extendFiringId !== '';
  protected readonly title = this.extend
    ? `Pegar mais gente no\u00a0${this.data.point.name}`
    : `Disparar o\u00a0${this.data.point.name}`;
  protected readonly subtitle = firstLine(this.data.point.description);
  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  /** The names of ticked people who left the list since: an empty pick would fire for the whole area, so the master ticks again. */
  protected readonly gone = signal<readonly string[]>([]);
  private readonly seen = new Map<string, string>();
  protected readonly rows = computed(() => this.data.targets() ?? []);
  protected readonly emptyText = computed(() =>
    this.data.targetsFailed?.()
      ? 'Não deu para ler quem está no mapa. Feche e tente de novo.'
      : this.data.targets() === null
        ? 'Lendo quem está no mapa…'
        : 'Ninguém com token no mapa.',
  );
  private readonly chosen = computed(() => this.rows().filter((t) => this.picked().has(t.id)));
  protected readonly label = computed(() =>
    fireLabel(
      this.chosen().map((t) => t.name),
      this.extend,
    ),
  );
  /** A new firing needs nobody (the area); adding to one needs someone. Neither goes on while someone ticked has left the list. */
  protected readonly ready = computed(
    () => this.gone().length === 0 && (!this.extend || this.chosen().length > 0),
  );
  protected readonly note = computed(() => {
    if (this.gone().length > 0) {
      return `${this.gone().join(', ')} saiu da lista. Marque de novo quem foi pego.`;
    }
    if (this.extend) {
      return this.chosen().length === 0
        ? 'Ninguém marcado. Escolha quem entra no disparo que já aconteceu.'
        : '';
    }
    return this.chosen().length === 0
      ? 'Ninguém marcado. Dispara para quem estiver na área da armadilha; quem está lá, o servidor sabe.'
      : 'Só quem você marcou é pego, esteja na área ou não.';
  });
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  constructor() {
    // Whoever was ticked and is no longer listed (the token left, the combat ended) leaves the pick.
    effect(() => {
      const rows = this.rows();
      untracked(() => {
        rows.forEach((r) => this.seen.set(r.id, r.name));
        const listed = new Set(rows.map((r) => r.id));
        const left = [...this.picked()].filter((id) => !listed.has(id));
        if (left.length > 0) {
          this.picked.update((set) => new Set([...set].filter((id) => listed.has(id))));
          this.gone.update((names) => [
            ...names,
            ...left.map((id) => this.seen.get(id) ?? 'Alguém'),
          ]);
        }
      });
    });
  }

  protected pick(next: ReadonlySet<string>): void {
    this.picked.set(next);
    this.gone.set([]);
  }

  protected cancel(): void {
    this.sheet.close(undefined);
  }

  protected async confirm(): Promise<void> {
    if (!this.ready() || this.busy()) {
      return;
    }
    const ids = this.chosen().map((t) => t.id);
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.fire(
        this.data.campaignId,
        this.data.mapId,
        this.data.point.id,
        ids,
        this.key.keyFor({ ids, extend: this.data.extendFiringId }),
        this.data.extendFiringId,
      );
      this.sheet.close(res.firing);
    } catch (err) {
      this.error.set(trapErrorMessage(err, 'disparar a armadilha'));
      this.frame()?.scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }
}
