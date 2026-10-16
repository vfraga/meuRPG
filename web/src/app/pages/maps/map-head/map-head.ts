import {
  Component,
  ElementRef,
  afterNextRender,
  computed,
  inject,
  Injector,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { RouterLink } from '@angular/router';

import { type GenerateOutcome } from '../../../shared/image-generate/image-generate-dialog';
import { GenerateImageButton } from '../../../shared/image-generate/generate-image-button';
import type { GenerateOrigin } from '../../../core/images/imagegen-copy';
import { MapAsk } from '../../../shared/map-ask/map-ask';

import { MapBlockedReason, type Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { DELETE_BLOCKED_TEXT } from '../../../core/maps/map-errors';
import { deleteMapConsequences } from './map-head-copy';

type Mode = 'view' | 'rename' | 'delete' | 'image';

/**
 * The master's header of a map (E5-23, E5-24, E6-27): "← Voltar para a
 * campanha", the name with "Renomear", and the map's state in words:
 * "Revelado aos jogadores" (eye) with the text button "Esconder", or
 * "Escondido dos jogadores" (eye-off) with "Revelar aos jogadores". On a
 * computer, also "Imagem: <nome>" with "Trocar imagem". The battle grid
 * (RN-21) has its own page: "Definir a grade" or "Mudar a grade" opens it.
 * "Apagar mapa" closes the line.
 *
 * "Apagar mapa" stays clickable when the server would refuse it, grey, with
 * the reason under it (`aria-describedby`); the question still shows the
 * server's refusal if the master insists.
 *
 * "Renomear" turns the title into the "Nome do mapa" field, with "Salvar
 * nome" and "Cancelar" under it. "Apagar mapa" asks in place of the state
 * line, saying what goes with the map, with the focus on "Cancelar". The
 * page runs the calls (`saveName`, `deleteMap`); this keeps the form, the
 * confirmation and the focus.
 */
@Component({
  selector: 'app-map-head',
  imports: [
    GenerateImageButton,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MapAsk,
    ReactiveFormsModule,
    RouterLink,
  ],
  templateUrl: './map-head.html',
  styleUrl: './map-head.scss',
})
export class MapHead {
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly map = input.required<MapMessage>();
  readonly showImage = input(true);
  readonly busy = input(false);
  /** What goes with the map, for the confirmation. */
  readonly pointCount = input(0);
  readonly tokenCount = input(0);
  /** The open session's number when this is its current map, else null. */
  readonly currentSession = input<number | null>(null);
  /** Saves the new name; rejects with the message to show. */
  readonly saveName = input.required<(name: string) => Promise<void>>();
  /** Deletes the map (the page then leaves); rejects with the message. */
  readonly deleteMap = input.required<() => Promise<void>>();
  /** A new image (like a new grid) erases what is painted and what the players saw: "Trocar imagem" asks first (E9-01 4). */
  readonly imageErases = input(false);
  /** A combat that has not ended runs on the map: the image cannot change (the editor's banner says why). */
  readonly combatRunning = input(false);
  /** A point of the map is a treasure marked found: the server refuses to delete the map (`TREASURE_FOUND`). */
  readonly treasureFound = input(false);
  /** A treasure of the map was turned into XP (`TREASURE_CONVERTED`). */
  readonly treasureConverted = input(false);
  readonly toggleReveal = output<void>();
  readonly changeImage = output<void>();
  /** The generate dialog made pictures, or made one the map's image: the page reads the map again. */
  readonly imageGenerated = output<GenerateOutcome>();

  protected readonly generateOrigin = computed<GenerateOrigin>(() => ({
    kind: 'map',
    mapId: this.map().id,
    name: this.map().name,
    hasGrid: this.map().gridColumns > 0,
    revealed: this.map().revealed,
  }));
  /** Why "Apagar mapa" would be refused, in the server's order (combat, treasure turned into XP, treasure found), or null. */
  protected readonly deleteReason = computed<string | null>(() => {
    if (this.combatRunning()) {
      return DELETE_BLOCKED_TEXT[MapBlockedReason.COMBAT_RUNNING];
    }
    if (this.treasureConverted()) {
      return DELETE_BLOCKED_TEXT[MapBlockedReason.TREASURE_CONVERTED];
    }
    if (this.treasureFound()) {
      return DELETE_BLOCKED_TEXT[MapBlockedReason.TREASURE_FOUND];
    }
    return null;
  });
  protected readonly mode = signal<Mode>('view');
  protected readonly working = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly form = new FormGroup({
    name: new FormControl('', {
      nonNullable: true,
      validators: [Validators.required, Validators.maxLength(80), Validators.pattern(/\S/)],
    }),
  });
  private readonly name = this.form.controls.name;
  protected readonly consequences = computed(() =>
    deleteMapConsequences(this.pointCount(), this.tokenCount(), this.currentSession()),
  );

  // `read: ElementRef`: on a Material button the reference is the button
  // component, and focus needs the element.
  private readonly nameInput = viewChild('nameInput', { read: ElementRef<HTMLInputElement> });
  private readonly renameButton = viewChild('renameButton', { read: ElementRef<HTMLElement> });
  private readonly deleteButton = viewChild('deleteButton', { read: ElementRef<HTMLElement> });
  private readonly cancelDelete = viewChild('cancelDelete', { read: ElementRef<HTMLElement> });
  private readonly imageButton = viewChild('imageButton', { read: ElementRef<HTMLElement> });

  protected startRename(): void {
    this.name.reset(this.map().name);
    this.error.set(null);
    this.mode.set('rename');
    this.focusAfterRender(() => {
      const el = this.nameInput()?.nativeElement;
      el?.focus();
      el?.select();
    });
  }

  protected cancelRename(): void {
    this.mode.set('view');
    this.error.set(null);
    this.focusAfterRender(() => this.renameButton()?.nativeElement.focus());
  }

  protected async submitName(): Promise<void> {
    const name = this.name.value.trim();
    if (this.name.invalid || name === '') {
      this.name.markAsTouched();
      return;
    }
    if (name === this.map().name) {
      this.cancelRename();
      return;
    }
    await this.run(async () => {
      await this.saveName()(name);
      this.cancelRename();
    });
  }

  protected askDelete(): void {
    this.error.set(null);
    this.mode.set('delete');
    // The safe button gets the focus (timeline.md, decision 10).
    this.focusAfterRender(() => this.cancelDelete()?.nativeElement.focus());
  }

  protected keepMap(): void {
    this.mode.set('view');
    this.error.set(null);
    this.focusAfterRender(() => this.deleteButton()?.nativeElement.focus());
  }

  /** "Trocar imagem": with something painted or seen it asks in place; with nothing to lose it goes straight to the picker. */
  protected startImage(): void {
    if (this.combatRunning()) {
      return;
    }
    if (!this.imageErases()) {
      this.changeImage.emit();
      return;
    }
    this.mode.set('image');
  }

  protected keepImage(): void {
    this.mode.set('view');
    this.focusAfterRender(() => this.imageButton()?.nativeElement.focus());
  }

  /** The question's button is gone once the header is back to its state: the picker opens from "Trocar imagem", where the focus returns after it. */
  protected confirmImage(): void {
    this.mode.set('view');
    this.focusAfterRender(() => {
      this.imageButton()?.nativeElement.focus();
      this.changeImage.emit();
    });
  }

  protected async confirmDelete(): Promise<void> {
    await this.run(() => this.deleteMap()());
  }

  private async run(action: () => Promise<void>): Promise<void> {
    if (this.working()) {
      return;
    }
    this.working.set(true);
    this.error.set(null);
    try {
      await action();
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : String(err));
    } finally {
      this.working.set(false);
    }
  }

  private focusAfterRender(focus: () => void): void {
    afterNextRender(focus, { injector: this.injector });
  }
}
