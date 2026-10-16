import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { MapPoint } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import type { PointChanges } from '../../../core/maps/maps-client';
import { MapsClient } from '../../../core/maps/maps-client';
import { trapMapErrorMessage } from '../../../core/traps/trap-errors';
import { foundLine, summaryLine } from '../../../core/traps/treasure-text';
import { MapAsk } from '../../../shared/map-ask/map-ask';
import { PairFoot } from '../../../shared/pair-foot/pair-foot';
import { type PickRow, PersonPick } from '../../../shared/person-pick/person-pick';
import {
  type TreasureDraft,
  hasTreasureErrors,
  isTreasureDirty,
  treasureChangesOf,
  treasureDraftOf,
  treasureErrors,
} from './treasure-draft';
import { PointFoot } from './point-foot';

/**
 * The panel of a Tesouro point (E9-02 5, MR-041): its name, "Descrição para os jogadores" (what is inside: they read it
 * once it is found) and "Valor em ouro" (whole PO), then "Encontrado":
 * - **not found:** "Marcar como encontrado" opens in place the same pick as the session's card (who found it, nobody
 *   checked, at least one), and says under the button that a treasure marked outside a session counts in no summary;
 * - **found:** by whom and when, and "Desmarcar", which asks in place (the focus on "Voltar");
 * - **converted into XP:** the lock and the reason; the description and the value are locked, "Desmarcar" and "Apagar
 *   ponto" are the dashed buttons that cannot act (the server refuses both).
 * The fields wait for "Salvar ponto"; marking and unmarking are saved at once, as the session's card does.
 */
@Component({
  selector: 'app-treasure-point-panel',
  imports: [
    MapAsk,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    PairFoot,
    PersonPick,
    PointFoot,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './treasure-point-panel.html',
  styleUrl: './point-panel-kinds.scss',
})
export class TreasurePointPanel {
  private readonly api = inject(MapsClient);
  private readonly injector = inject(Injector);

  readonly point = input.required<MapPoint>();
  readonly campaignId = input.required<string>();
  /** The living player characters, for "Quem encontrou". */
  readonly people = input<readonly PickRow[]>([]);
  /** The open session's number, when there is one (marking then counts in its summary). */
  readonly sessionNumber = input<number | null>(null);
  readonly saving = input(false);
  readonly error = input<string | null>(null);
  readonly focusName = input(false);

  readonly saveRequested = output<void>();
  readonly removeConfirmed = output<void>();
  readonly dirtyChange = output<boolean>();
  /** The point after marking or unmarking (saved at once): the page puts it on the map. */
  readonly pointChange = output<MapPoint>();

  protected readonly draft = signal<TreasureDraft>({ name: '', description: '', valuePo: '0' });
  protected readonly show = signal(false);
  protected readonly mode = signal<'view' | 'marking' | 'unmarking'>('view');
  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  protected readonly busy = signal(false);
  protected readonly callError = signal('');

  protected readonly found = computed(() => this.point().treasureFoundAt !== undefined);
  protected readonly converted = computed(() => this.point().treasureConverted);
  protected readonly line = computed(() => foundLine(this.point()));
  protected readonly errors = computed(() => treasureErrors(this.draft()));
  protected readonly shown = computed(() => (this.show() ? this.errors() : {}));
  protected readonly dirty = computed(() => isTreasureDirty(this.draft(), this.point()));
  protected readonly names = computed(() =>
    this.people()
      .filter((p) => this.picked().has(p.id))
      .map((p) => p.name),
  );
  protected readonly summary = computed(() =>
    summaryLine(this.names(), this.point().treasureValuePo),
  );
  protected readonly state = computed(() =>
    this.converted()
      ? 'convertido'
      : this.found()
        ? 'visível para todos'
        : this.point().revealed
          ? 'visível para todos'
          : 'escondido',
  );
  protected readonly blocked = computed(() =>
    this.converted()
      ? 'Convertido em XP: este tesouro não pode ser apagado.'
      : this.found()
        ? 'Este tesouro foi encontrado. Desmarque antes de apagar.'
        : '',
  );
  protected readonly sessionLine = computed(() => {
    const n = this.sessionNumber();
    return n === null
      ? 'Marcado fora de uma sessão, o tesouro não entra em resumo nenhum.'
      : `Marcado durante a Sessão ${n}, o tesouro entra no resumo dela.`;
  });

  private readonly nameField = viewChild('nameField', { read: ElementRef<HTMLInputElement> });
  private readonly opener = viewChild('opener', { read: ElementRef<HTMLButtonElement> });
  private readonly undo = viewChild('undo', { read: ElementRef<HTMLButtonElement> });
  private readonly form = viewChild<ElementRef<HTMLElement>>('form');
  private currentId = '';

  constructor() {
    effect(() => {
      const point = this.point();
      untracked(() => {
        if (point.id !== this.currentId) {
          this.currentId = point.id;
          this.reset();
          this.mode.set('view');
          this.callError.set('');
          if (this.focusName()) {
            afterNextRender(() => this.nameField()?.nativeElement.focus(), {
              injector: this.injector,
            });
          }
        }
      });
    });
    // The answer came back (found, or unmarked): the section is a plain section again.
    effect(() => {
      this.found();
      untracked(() => this.mode.set('view'));
    });
    effect(() => this.dirtyChange.emit(this.dirty()));
  }

  private reset(): void {
    this.draft.set(treasureDraftOf(this.point()));
    this.show.set(false);
  }

  protected input(field: keyof TreasureDraft, event: Event): void {
    this.draft.update((d) => ({ ...d, [field]: (event.target as HTMLInputElement).value }));
  }

  changes(): PointChanges | null {
    this.show.set(true);
    if (hasTreasureErrors(this.errors())) {
      return null;
    }
    return treasureChangesOf(this.draft(), this.point());
  }

  discard(): void {
    this.reset();
  }

  protected startMarking(): void {
    this.picked.set(new Set());
    this.callError.set('');
    this.mode.set('marking');
    afterNextRender(
      () => {
        const form = this.form()?.nativeElement;
        form?.scrollIntoView?.({ block: 'nearest' });
        focusWithRing(form?.querySelector<HTMLInputElement>('input[type=checkbox]:not(:disabled)'));
      },
      { injector: this.injector },
    );
  }

  protected cancelMarking(): void {
    this.mode.set('view');
    afterNextRender(() => focusWithRing(this.opener()?.nativeElement), { injector: this.injector });
  }

  protected startUnmark(): void {
    this.callError.set('');
    this.mode.set('unmarking');
  }

  protected cancelUnmark(): void {
    this.mode.set('view');
    afterNextRender(() => focusWithRing(this.undo()?.nativeElement), { injector: this.injector });
  }

  protected async confirmMarking(): Promise<void> {
    if (this.picked().size === 0 || this.busy()) {
      return;
    }
    const ids = this.people()
      .filter((p) => this.picked().has(p.id))
      .map((p) => p.id);
    await this.run('marcar o tesouro', () =>
      this.api.markTreasureFound(this.campaignId(), this.point().mapId, this.point().id, ids),
    );
  }

  protected async confirmUnmark(): Promise<void> {
    await this.run('desmarcar o tesouro', () =>
      this.api.unmarkTreasureFound(this.campaignId(), this.point().mapId, this.point().id),
    );
  }

  private async run(what: string, call: () => Promise<MapPoint>): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.callError.set('');
    try {
      this.pointChange.emit(await call());
    } catch (err) {
      this.callError.set(trapMapErrorMessage(err, what));
    } finally {
      this.busy.set(false);
    }
  }
}
