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
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { MapPoint } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { type LightOption, LightPresets } from '../../../core/maps/light-presets';
import type { PointChanges } from '../../../core/maps/maps-client';
import { tight } from '../../../core/format/text';
import type { LightReach } from '../../../shared/editor-overlay/editor-overlay';
import {
  CUSTOM_KEY,
  type LightDraft,
  hasLightErrors,
  isLightDirty,
  lightChangesOf,
  lightDraftOf,
  lightErrors,
  lightFt,
  lightSquares,
  withPreset,
} from './light-draft';
import { PointFoot } from './point-foot';

/**
 * The panel of a Luz point (E9-02 2, MR-036): its name, the SRD's light sources as 52 px radios (the key, the two
 * radii in metres) and "Personalizada" with the two radii typed in metres (whole squares of 1,5 m, up to 36 m),
 * and what the choice reaches in squares. The master's alone: a player never gets the point, only its light. The
 * reach is **the radii** drawn on the map (`reachChange`), never the lit squares: no read gives the master those.
 * Nothing is saved until "Salvar ponto". Same contract as the other point panels: `changes()`, `discard()`.
 */
@Component({
  selector: 'app-light-point-panel',
  imports: [MatFormFieldModule, MatIconModule, MatInputModule, PointFoot],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './light-point-panel.html',
  styleUrl: './point-panel-kinds.scss',
})
export class LightPointPanel {
  private readonly injector = inject(Injector);
  private readonly presetsApi = inject(LightPresets);

  readonly point = input.required<MapPoint>();
  readonly campaignId = input.required<string>();
  readonly saving = input(false);
  readonly error = input<string | null>(null);
  readonly focusName = input(false);

  readonly saveRequested = output<void>();
  readonly removeConfirmed = output<void>();
  readonly dirtyChange = output<boolean>();
  /** The radii of the light as the form has them, to draw on the map; `null` while they are not valid. */
  readonly reachChange = output<LightReach | null>();

  protected readonly presets = signal<readonly LightOption[]>([]);
  protected readonly presetsFailed = signal(false);
  protected readonly draft = signal<LightDraft>({
    name: '',
    description: '',
    presetKey: CUSTOM_KEY,
    brightM: '',
    dimM: '',
  });
  protected readonly show = signal(false);
  protected readonly custom = computed(() => this.draft().presetKey === CUSTOM_KEY);
  protected readonly errors = computed(() => lightErrors(this.draft()));
  protected readonly shown = computed(() => (this.show() ? this.errors() : {}));
  protected readonly dirty = computed(() =>
    isLightDirty(this.draft(), this.point(), this.presets()),
  );
  protected readonly squares = computed(() => lightSquares(this.draft()));
  protected readonly reachLine = computed(() => {
    const s = this.squares();
    return s
      ? tight(
          `Raios: ${s.bright} ${s.bright === 1 ? 'quadrado' : 'quadrados'} de luz clara e mais ${s.dim} de penumbra, até ${s.bright + s.dim} ${s.bright + s.dim === 1 ? 'quadrado' : 'quadrados'} no total.`,
        )
      : '';
  });
  protected readonly metresLine = computed(() =>
    this.reachLine()
      ? `Em metros, contados do centro do quadrado, em múltiplos de 1,5 m (um quadrado). ${this.reachLine()}`
      : '',
  );

  private readonly nameField = viewChild('nameField', { read: ElementRef<HTMLInputElement> });
  private currentId = '';
  /** The master has chosen a light source or typed a radius since the form was last filled from the point: the presets arriving must not wipe that. */
  private editedLight = false;

  constructor() {
    // The SRD's light sources, once per campaign; the saved light may be one of them, so the radios follow once they are known.
    effect(() => {
      const campaignId = this.campaignId();
      void untracked(() =>
        this.presetsApi.list(campaignId).then(
          (list) => {
            this.presets.set(list);
            // The saved light may be a preset: the radios follow, unless he has already chosen a source or typed a radius (that would wipe
            // it). A name or a description typed meanwhile stay.
            if (!this.editedLight) {
              const fromPoint = lightDraftOf(this.point(), list);
              this.draft.update((d) => ({
                ...d,
                presetKey: fromPoint.presetKey,
                brightM: fromPoint.brightM,
                dimM: fromPoint.dimM,
              }));
            }
          },
          () => this.presetsFailed.set(true),
        ),
      );
    });
    effect(() => {
      const point = this.point();
      untracked(() => {
        if (point.id !== this.currentId) {
          this.currentId = point.id;
          this.reset();
          if (this.focusName()) {
            afterNextRender(() => this.nameField()?.nativeElement.focus(), {
              injector: this.injector,
            });
          }
        }
      });
    });
    effect(() => this.dirtyChange.emit(this.dirty()));
    effect(() => {
      const ft = lightFt(this.draft());
      const p = this.point();
      this.reachChange.emit(
        ft ? { xBp: p.xBp, yBp: p.yBp, brightFt: ft.brightFt, dimFt: ft.dimFt } : null,
      );
    });
  }

  private reset(): void {
    this.editedLight = false;
    this.draft.set(lightDraftOf(this.point(), this.presets()));
    this.show.set(false);
  }

  protected choose(key: string): void {
    this.editedLight = true;
    this.draft.update((d) => withPreset(d, key, this.presets()));
  }

  protected patch(change: Partial<LightDraft>): void {
    if ('presetKey' in change || 'brightM' in change || 'dimM' in change) {
      this.editedLight = true;
    }
    this.draft.update((d) => ({ ...d, ...change }));
  }

  protected input(field: 'name' | 'brightM' | 'dimM', event: Event): void {
    this.patch({ [field]: (event.target as HTMLInputElement).value });
  }

  /** What "Salvar ponto" sends; `null` when nothing changed or the form breaks a rule (the fields say which). */
  changes(): PointChanges | null {
    this.show.set(true);
    if (hasLightErrors(this.errors())) {
      return null;
    }
    return lightChangesOf(this.draft(), this.point(), this.presets());
  }

  discard(): void {
    this.reset();
  }

  /** Arrow keys inside the radio list. */
  protected onKey(event: KeyboardEvent): void {
    const keys = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'];
    if (!keys.includes(event.key)) {
      return;
    }
    event.preventDefault();
    const step = event.key === 'ArrowUp' || event.key === 'ArrowLeft' ? -1 : 1;
    const buttons = Array.from(
      (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role="radio"]'),
    );
    const here = buttons.findIndex((b) => b === document.activeElement);
    const next = buttons[(Math.max(0, here) + step + buttons.length) % buttons.length];
    next?.focus();
    next?.click();
  }
}
