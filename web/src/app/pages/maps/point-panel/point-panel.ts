import {
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
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { RouterLink } from '@angular/router';
import { Code } from '@connectrpc/connect';

import { MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type {
  MapPoint,
  SceneAction,
  SceneClue,
  SceneImage,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { ActionKey } from '../../../core/connect/idempotency';
import { MapsClient } from '../../../core/maps/maps-client';
import type { PointChanges } from '../../../core/maps/maps-client';
import type { CluePlayer } from '../../../core/maps/scene-clues';
import { GalleryPicker } from '../../../shared/gallery-picker/gallery-picker';
import { pointKindIcon, pointKindLabel } from '../../../shared/map-view/map-labels';
import { mapNameError } from '../map-new/map-new';
import { ClueList } from '../clue-list/clue-list';
import { HooksField } from '../hooks-field/hooks-field';
import { RevealSwitch } from '../../../shared/reveal-switch/reveal-switch';
import { SceneImages } from '../scene-images/scene-images';
import { SceneActions } from '../scene-actions/scene-actions';
import {
  POINT_DESCRIPTION_MAX,
  POINT_NAME_MAX,
  DraftErrors,
  PointDraft,
  changesOf,
  draftErrors,
  draftOf,
  isDirty,
} from './point-draft';

/** The select's "Criar mapa novo…" option (never a map id). */
const NEW_MAP = '__new__';
const MAP_NAME_MAX = 80;

const KINDS = [MapPointKind.BATTLE, MapPointKind.SUBMAP, MapPointKind.SCENE] as const;

/**
 * The editor's side panel for the selected point (E5-23): Tipo, Nome,
 * "Leva para" (Submapa only), "Descrição para os jogadores", the switch
 * "Revelado aos jogadores", the screen's one primary action "Salvar ponto",
 * and "Apagar ponto", which confirms in place ("Apagar Taverna do Javali?
 * Não dá para desfazer."). A saved SCENE point also has "Ações da cena"
 * (E7-01) and "Pistas" (E8-04), which save on their own; "Ganchos e
 * anotações" (the master's private text) waits for "Salvar ponto" like the
 * description. The rest saves together; positions don't belong here (the map
 * moves them). The page runs the calls.
 */
@Component({
  selector: 'app-point-panel',
  imports: [
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    ReactiveFormsModule,
    RouterLink,
    ClueList,
    GalleryPicker,
    HooksField,
    RevealSwitch,
    SceneActions,
    SceneImages,
  ],
  templateUrl: './point-panel.html',
  styleUrl: './point-panel.scss',
})
export class PointPanel {
  private readonly injector = inject(Injector);
  private readonly api = inject(MapsClient);

  readonly point = input.required<MapPoint>();
  /** The campaign, for the scene actions the panel saves on its own. */
  readonly campaignId = input('');
  /** The map the point is on, for "Montar o encontro" (MR-043). */
  readonly mapId = input('');
  /** The campaign's player characters, to say who has each clue. */
  readonly players = input<readonly CluePlayer[]>([]);
  /** The campaign's other maps, for "Leva para". */
  readonly maps = input<readonly { id: string; name: string }[]>([]);
  readonly saving = input(false);
  readonly error = input<string | null>(null);
  /** A point just placed: focus goes to "Nome". */
  readonly focusName = input(false);

  /** "Salvar ponto" (the page asks `changes()` and saves). */
  readonly saveRequested = output<void>();
  readonly removeConfirmed = output<void>();
  /** Whether there are unsaved changes (the page asks before leaving). */
  readonly dirtyChange = output<boolean>();
  /** A SCENE point's actions changed (each change is saved at once, apart
   * from "Salvar ponto"): the page puts the new list on the point. */
  readonly sceneActionsChange = output<readonly SceneAction[]>();
  /** A SCENE point's clues changed (saved at once too). */
  readonly cluesChange = output<readonly SceneClue[]>();
  /** A SCENE point's images changed (saved at once too). */
  readonly imagesChange = output<readonly SceneImage[]>();
  /** "Mostrar a CD aos jogadores" saved on its own (at once, like the actions): the point carries it. */
  readonly showDcSaved = output<boolean>();
  /** "Criar mapa novo…" made a map: the page reads the campaign's maps again. */
  readonly mapCreated = output<{ id: string; name: string }>();

  protected readonly kinds = KINDS;
  protected readonly kindLabel = pointKindLabel;
  protected readonly kindIcon = pointKindIcon;
  protected readonly Submap = MapPointKind.SUBMAP;
  protected readonly Battle = MapPointKind.BATTLE;
  protected readonly Scene = MapPointKind.SCENE;
  protected readonly nameMax = POINT_NAME_MAX;
  protected readonly descriptionMax = POINT_DESCRIPTION_MAX;

  protected readonly draft = signal<PointDraft>({
    kind: MapPointKind.SCENE,
    name: '',
    description: '',
    hooks: '',
    revealed: false,
    targetMapId: '',
  });
  protected readonly dirty = computed(() => isDirty(this.draft(), this.point()));
  protected readonly errors = signal<DraftErrors>({});
  protected readonly confirming = signal(false);
  // The two text fields are form controls: their messages are `mat-error`s,
  // which Material shows for a control with errors that was touched.
  protected readonly nameControl = new FormControl('', { nonNullable: true });
  protected readonly descriptionControl = new FormControl('', { nonNullable: true });
  protected readonly hooksControl = new FormControl('', { nonNullable: true });
  protected readonly hooksLength = computed(() => [...this.draft().hooks].length);
  /** Maps made here, before the page reads the list again. */
  private readonly madeHere = signal<readonly { id: string; name: string }[]>([]);
  protected readonly targets = computed(() => {
    const known = this.maps();
    const extra = this.madeHere().filter((m) => !known.some((k) => k.id === m.id));
    return [...known, ...extra].filter((m) => m.id !== this.point().mapId);
  });
  /** The name of the map the draft leads to, for "Abrir <mapa>". */
  protected readonly targetName = computed(() => {
    const id = this.draft().targetMapId;
    if (!id || !this.campaignId()) {
      return '';
    }
    return this.targets().find((m) => m.id === id)?.name ?? '';
  });

  // ---- "Criar mapa novo…" ----
  protected readonly newOption = NEW_MAP;
  protected readonly mapNameMax = MAP_NAME_MAX;
  protected readonly creating = signal(false);
  protected readonly newNameControl = new FormControl('', { nonNullable: true });
  protected readonly newImageId = signal<string | null>(null);
  protected readonly newNameError = signal<string | null>(null);
  protected readonly newImageError = signal<string | null>(null);
  protected readonly newFailure = signal<string | null>(null);
  protected readonly newBusy = signal(false);
  private readonly newKey = new ActionKey();
  private readonly newNameField = viewChild('newNameField', {
    read: ElementRef<HTMLInputElement>,
  });

  private readonly nameField = viewChild('nameField', { read: ElementRef<HTMLInputElement> });
  private readonly confirmButton = viewChild('confirmButton', {
    read: ElementRef<HTMLButtonElement>,
  });
  private currentId = '';

  constructor() {
    // Another point: a fresh draft. The same point after a save or a move
    // keeps what is being typed (the draft only differs by what the master
    // changed).
    effect(() => {
      const point = this.point();
      untracked(() => {
        if (point.id !== this.currentId) {
          this.currentId = point.id;
          this.reset(draftOf(point));
          this.confirming.set(false);
          if (this.focusName()) {
            afterNextRender(() => this.nameField()?.nativeElement.focus(), {
              injector: this.injector,
            });
          }
        }
      });
    });
    effect(() => this.dirtyChange.emit(this.dirty()));
    this.nameControl.valueChanges
      .pipe(takeUntilDestroyed())
      .subscribe((name) => this.patch({ name }));
    this.descriptionControl.valueChanges
      .pipe(takeUntilDestroyed())
      .subscribe((description) => this.patch({ description }));
    this.hooksControl.valueChanges
      .pipe(takeUntilDestroyed())
      .subscribe((hooks) => this.patch({ hooks }));
  }

  /** A fresh draft: the fields show it, with no message. */
  private reset(draft: PointDraft): void {
    this.draft.set(draft);
    this.nameControl.setValue(draft.name, { emitEvent: false });
    this.descriptionControl.setValue(draft.description, { emitEvent: false });
    this.hooksControl.setValue(draft.hooks, { emitEvent: false });
    this.nameControl.setErrors(null);
    this.descriptionControl.setErrors(null);
    this.hooksControl.setErrors(null);
    this.errors.set({});
  }

  /** The select changed: a map, none, or "Criar mapa novo…" (which opens the form and keeps the old choice). */
  protected onTarget(select: HTMLSelectElement): void {
    if (select.value !== NEW_MAP) {
      this.patch({ targetMapId: select.value });
      return;
    }
    select.value = this.draft().targetMapId;
    this.newNameControl.setValue(this.draft().name.trim());
    this.newNameError.set(null);
    this.newImageError.set(null);
    this.newFailure.set(null);
    this.creating.set(true);
    afterNextRender(() => this.newNameField()?.nativeElement.focus(), { injector: this.injector });
  }

  /** Creates the map (hidden), makes it this point's destination and asks the page to save the point. */
  protected async createTarget(): Promise<void> {
    const nameError = mapNameError(this.newNameControl.value);
    this.newNameError.set(nameError);
    const imageId = this.newImageId();
    this.newImageError.set(imageId === null ? 'Escolha uma imagem para o mapa.' : null);
    if (nameError !== null) {
      this.newNameControl.setErrors({ name: true });
      this.newNameControl.markAsTouched();
      this.newNameField()?.nativeElement.focus();
      return;
    }
    if (imageId === null || this.newBusy()) {
      return;
    }
    this.newBusy.set(true);
    this.newFailure.set(null);
    try {
      const name = this.newNameControl.value.trim();
      const map = await this.api.create(
        this.campaignId(),
        name,
        imageId,
        this.newKey.keyFor([name, imageId]),
      );
      this.newKey.renew();
      const made = { id: map.id, name: map.name };
      this.madeHere.update((list) => [...list, made]);
      this.mapCreated.emit(made);
      this.patch({ targetMapId: map.id });
      this.creating.set(false);
      this.newImageId.set(null);
      this.saveRequested.emit();
    } catch (err) {
      this.newFailure.set(
        describeConnectError(err, {
          [Code.InvalidArgument]:
            'Não deu para criar o mapa: o nome ou a imagem não valem mais. Confira os dois e tente de novo.',
          [Code.PermissionDenied]: 'Só o mestre da campanha cria mapas.',
          [Code.ResourceExhausted]: 'A campanha chegou ao limite de 200 mapas.',
        }),
      );
    } finally {
      this.newBusy.set(false);
    }
  }

  protected patch(change: Partial<PointDraft>): void {
    this.draft.update((d) => ({ ...d, ...change }));
  }

  /** What "Salvar ponto" sends; `null` when nothing changed or the draft
   * breaks a rule (the fields then say what). */
  changes(): PointChanges | null {
    const errors = draftErrors(this.draft());
    this.errors.set(errors);
    if (errors.name) {
      this.nameControl.setErrors({ name: true });
      this.nameControl.markAsTouched();
    }
    if (errors.description) {
      this.descriptionControl.setErrors({ description: true });
      this.descriptionControl.markAsTouched();
    }
    if (errors.hooks) {
      this.hooksControl.setErrors({ hooks: true });
      this.hooksControl.markAsTouched();
    }
    if (errors.name || errors.description || errors.hooks) {
      (errors.name ? this.nameField() : undefined)?.nativeElement.focus();
      return null;
    }
    return changesOf(this.draft(), this.point());
  }

  /** Drops the unsaved changes. */
  discard(): void {
    this.reset(draftOf(this.point()));
  }

  protected askRemove(): void {
    this.confirming.set(true);
    afterNextRender(() => this.confirmButton()?.nativeElement.focus(), { injector: this.injector });
  }

  /** Arrow keys inside the segmented control (the radio pattern). */
  protected onKindKey(event: KeyboardEvent): void {
    const keys = ['ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown'];
    if (!keys.includes(event.key)) {
      return;
    }
    event.preventDefault();
    const step = event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1;
    const index = KINDS.indexOf(this.draft().kind as (typeof KINDS)[number]);
    const next = KINDS[(index + step + KINDS.length) % KINDS.length];
    this.patch({ kind: next });
    const buttons = (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('button');
    buttons[KINDS.indexOf(next)]?.focus();
  }
}
