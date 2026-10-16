import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { GalleryImage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import {
  ImageAspectRatio,
  type ImageGenerationStatus,
  ImageStyle,
} from '../../../gen/meurpg/maps/v1/imagegen_pb';
import {
  DEFAULT_MAX_CHARACTERS,
  DEFAULT_MAX_OBJECTS,
  DEFAULT_MAX_PROMPT,
  KIND_ABOUT,
  MAX_IMAGE_NAME,
  type GenerateOrigin,
  type KindChoice,
  type KindKey,
  NO_PLAYERS_VIEW,
  PROMPT_PRIVACY,
  RATIOS,
  RATIO_PRIVACY,
  STYLES,
  STYLE_PRIVACY,
  remainingText,
  whatGoesAlong,
} from '../../core/images/imagegen-copy';
import type { GenerateIssue } from '../../core/images/imagegen-errors';
import { type ImageForm, type PickableNpc, npcPortraits } from '../../core/images/imagegen-form';
import { ChoiceRow } from '../choice-row/choice-row';
import { KindStrip } from './kind-strip';
import { NpcPicks } from './npc-picks';
import { ReferencePicks } from './reference-picks';

/** What the server drew for the way chosen (`GetMapImageReference`), as the form shows it. */
export interface ReferenceView {
  /** A `data:` URL of the small PNG that goes to Google (the app's CSP allows images from its own origin and `data:`). */
  readonly previewUrl: string;
  readonly seenSquares: number;
  readonly totalSquares: number;
  /** False when no character of a player is on the map: the drawing is all black and the request would be refused. */
  readonly playersSee: boolean;
  /** The rooms of a generated dungeon that go in the text of a textured map. */
  readonly rooms: number;
  readonly gridColumns: number;
  readonly gridRows: number;
}

/**
 * The body of "Gerar imagem" (E10-07 1 to 3, 9 and 10): the way (scene art, isometric view or textured map), what goes along (the drawing
 * the server makes of the players' view, with a sentence on what it leaves out), who appears, the gallery references, the text with what goes
 * to Google under it, the style, the ratio (none for the textured map) and the month's count. It only draws: the dialog owns the values and
 * sends the request, and the browser never builds a picture.
 *
 * The textured map takes no creature: "Quem aparece na imagem" is not drawn for it and no portrait goes (RN-28). A way that cannot be made
 * from where the dialog opened is dashed with its reason under the strip.
 */
@Component({
  selector: 'app-generate-form',
  imports: [
    ChoiceRow,
    KindStrip,
    NgTemplateOutlet,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    NpcPicks,
    ReferencePicks,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './generate-form.html',
  styleUrl: './generate-form.scss',
})
export class GenerateForm {
  readonly origin = input.required<GenerateOrigin>();
  readonly choices = input.required<readonly KindChoice[]>();
  readonly form = input.required<ImageForm>();
  readonly reference = input<ReferenceView | null>(null);
  /** `loading` while the drawing is read, `error` when it could not be. */
  readonly referenceState = input<'none' | 'loading' | 'ready' | 'error'>('none');
  readonly npcs = input<readonly PickableNpc[]>([]);
  readonly references = input<readonly GalleryImage[]>([]);
  readonly status = input<ImageGenerationStatus | null>(null);
  readonly issue = input<GenerateIssue | null>(null);
  /** The phone's sheet: the text and its privacy line come right after the way (artboards 9 and 10), not after the references. */
  readonly phone = input(false);
  readonly formChange = output<Partial<ImageForm>>();
  readonly pickReference = output<void>();
  readonly removeReference = output<string>();
  readonly retryReference = output<void>();

  protected readonly noPlayers = NO_PLAYERS_VIEW;
  protected readonly promptPrivacy = PROMPT_PRIVACY;
  protected readonly stylePrivacy = STYLE_PRIVACY;
  protected readonly ratioPrivacy = RATIO_PRIVACY;
  protected readonly styles = STYLES.map((s) => ({ value: s.value, label: s.label }));
  protected readonly ratios = RATIOS;

  protected readonly isMap = computed(
    () => this.origin().kind === 'map' && this.origin().hasGrid !== false,
  );
  protected readonly fromMapDrawing = computed(
    () => this.isMap() && this.referenceState() !== 'none',
  );
  protected readonly texture = computed(() => this.form().kind === 'texture');
  protected readonly about = computed(() => {
    const kind = this.form().kind;
    const reason = this.choices().find((c) => !c.available && c.reason !== '')?.reason ?? '';
    const ref = this.reference();
    if (kind === 'texture' && ref && ref.gridColumns > 0) {
      return `O próprio mapa visto de cima, ajustado à grade de ${ref.gridColumns} × ${ref.gridRows} quadrados.`;
    }
    return [KIND_ABOUT[kind], reason].filter((t) => t !== '').join(' ');
  });
  protected readonly along = computed(() =>
    whatGoesAlong(this.form().kind, this.reference()?.rooms ?? 0),
  );
  protected readonly showNpcs = computed(() => this.isMap() && !this.texture());
  protected readonly characterNames = computed(() =>
    npcPortraits(this.form(), this.npcs()).length === 0
      ? []
      : this.npcs()
          .filter((n) => this.form().npcIds.includes(n.characterId) && n.portraitImageId !== '')
          .map((n) => n.name),
  );
  protected readonly promptLabel = computed(() =>
    this.isMap() ? 'Descreva o lugar' : 'Descreva a cena',
  );
  protected readonly maxPrompt = computed(
    () => this.status()?.maxPromptCharacters || DEFAULT_MAX_PROMPT,
  );
  protected readonly length = computed(() => [...this.form().prompt].length);
  protected readonly maxObjects = computed(
    () => this.status()?.maxObjectReferences || DEFAULT_MAX_OBJECTS,
  );
  protected readonly maxCharacters = computed(
    () => this.status()?.maxCharacterReferences || DEFAULT_MAX_CHARACTERS,
  );
  protected readonly remaining = computed(() => {
    const s = this.status();
    return s ? remainingText(s) : '';
  });
  /** What is left of the month, as the bar fills (full at 20 of 20, empty at 0). */
  protected readonly leftPercent = computed(() => {
    const s = this.status();
    return s && s.monthlyLimit > 0
      ? Math.min(100, Math.max(0, Math.round((s.remaining / s.monthlyLimit) * 100)))
      : 0;
  });
  protected readonly maxName = MAX_IMAGE_NAME;

  protected setKind(kind: KindKey): void {
    this.formChange.emit({ kind });
  }

  protected setName(event: Event): void {
    this.formChange.emit({ name: (event.target as HTMLInputElement).value });
  }

  protected setPrompt(event: Event): void {
    this.formChange.emit({ prompt: (event.target as HTMLTextAreaElement).value });
  }

  protected setStyle(style: ImageStyle): void {
    this.formChange.emit({ style });
  }

  protected setRatio(event: Event): void {
    this.formChange.emit({
      ratio: Number((event.target as HTMLSelectElement).value) as ImageAspectRatio,
    });
  }

  protected toggleNpc(id: string): void {
    const ids = this.form().npcIds;
    this.formChange.emit({ npcIds: ids.includes(id) ? ids.filter((i) => i !== id) : [...ids, id] });
  }
}
