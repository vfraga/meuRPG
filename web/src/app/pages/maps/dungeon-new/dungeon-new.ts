import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { type OptionField, createFailure, previewFailure } from '../../../core/maps/dungeon-errors';
import { stairCountText } from '../../../core/maps/dungeon-layout';
import {
  CORRIDORS,
  DEFAULT_FORM,
  DOOR_MIXES,
  type DungeonForm,
  MASKS,
  SIZE_PRESETS,
  formProblems,
  optionsOf,
  parseSeed,
  shortSideOf,
  sideOf,
} from '../../../core/maps/dungeon-options';
import { DungeonsClient, type DungeonOptionsInit } from '../../../core/maps/dungeons-client';
import { ActionKey } from '../../../core/connect/idempotency';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import {
  DungeonPreview,
  type DungeonLayout,
} from '../../../shared/dungeon-preview/dungeon-preview';
import { mapNameError } from '../map-new/map-new';
import { DungeonOptions } from './dungeon-options';

type Phase = 'loading' | 'ready' | 'gone' | 'error';

/** The preview's state: nothing asked yet, asked and waiting, drawn, refused (a field or the whole), or failed. */
type PreviewState = 'loading' | 'ready' | 'failed';

/** How long the page waits after the last change of an option before it asks for a preview. */
export const PREVIEW_DELAY_MS = 350;

const SEED_PROBLEM = 'A semente é um número inteiro, só com dígitos.';

/**
 * "Gerar masmorra" (MR-010, RN-26, RN-10; E10-05 1 to 4), the master's page, reached from the campaign's maps: the options on the left, the
 * server's preview on the right and "Criar o mapa".
 *
 * - **The preview** is the server's (`PreviewDungeon`), asked a moment after the last change of an option and one at a time (a change
 *   during a request waits for it, then asks again with the latest options); the browser does no generation, it draws the layout it gets.
 *   "Outra semente" is a call without a seed, and the seed the server drew fills the field. The page's own checks (21 to 121 squares, the
 *   rooms' sides) refuse before asking; a refusal of the server lands on the field it names.
 * - **"Criar o mapa"** (`CreateDungeonMap`) sends the name, the options and the previewed seed, shows the steps while the server draws and
 *   stores the image, and opens the new map in the editor. The server finishes the map even when the master stops waiting ("Cancelar").
 * - The map is born hidden from the players with the fog on and the base light "Clara": the line under the preview says so.
 *
 * On a phone the page only says that generating a dungeon is done on a computer (E10-05 1).
 */
@Component({
  selector: 'app-dungeon-new',
  imports: [
    DungeonOptions,
    DungeonPreview,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressBarModule,
    MatProgressSpinnerModule,
    ReactiveFormsModule,
    RouterLink,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dungeon-new.html',
  styleUrl: './dungeon-new.scss',
})
export class DungeonNew {
  private readonly api = inject(DungeonsClient);
  private readonly createKey = new ActionKey();
  private readonly campaigns = inject(CampaignsService);
  private readonly router = inject(Router);

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly campaignId = signal('');
  protected readonly campaignName = signal('');
  protected readonly phase = signal<Phase>('loading');

  protected readonly form = signal<DungeonForm>(DEFAULT_FORM);
  protected readonly seedText = signal('');
  /** The server's refusals by field, kept until the option changes. */
  private readonly refused = signal<Partial<Record<OptionField, string>>>({});
  /** A refusal that names no field (the shape and the size leave no room). */
  protected readonly refusedAll = signal<string | null>(null);
  protected readonly problems = computed(() => ({
    ...this.refused(),
    ...formProblems(this.form()),
  }));
  protected readonly seedProblem = computed(() =>
    this.seedText().trim() === '' || parseSeed(this.seedText()) !== null ? null : SEED_PROBLEM,
  );
  /** Whether a problem the page can already see keeps it from asking. */
  protected readonly blocked = computed(
    () => Object.keys(formProblems(this.form())).length > 0 || this.seedProblem() !== null,
  );

  protected readonly preview = signal<PreviewState>('loading');
  protected readonly layout = signal<DungeonLayout | null>(null);
  protected readonly failure = signal('');
  /** The layout on screen: the seed and the options it was made with. "Criar o mapa" sends these, never what the form says now. */
  private readonly previewed = signal<{
    readonly seed: bigint;
    readonly options: DungeonOptionsInit;
  } | null>(null);

  protected readonly nameControl = new FormControl('', { nonNullable: true });
  protected readonly nameError = signal<string | null>(null);

  protected readonly creating = signal(false);
  protected readonly createError = signal<string | null>(null);
  protected readonly stopped = signal(false);
  /** The status line of a map being made shows after a moment, so a quick creation does not flash it. */
  protected readonly slow = signal(false);
  private slowTimer: ReturnType<typeof setTimeout> | undefined;
  /** What the creating screen says about the options (frozen while the map is made). */
  protected readonly chips = computed(() => this.optionChips());

  /** The reason "Criar o mapa" is off, or `null` when it can go. */
  protected readonly whyOff = computed<string | null>(() => {
    if (Object.keys(this.problems()).length > 0 || this.refusedAll() !== null) {
      return Object.keys(formProblems(this.form())).includes('size') || this.problems().size
        ? 'Corrija o tamanho para criar o mapa.'
        : 'Corrija as opções marcadas para criar o mapa.';
    }
    if (this.seedProblem() !== null) {
      return 'Corrija a semente para criar o mapa.';
    }
    if (this.preview() === 'failed') {
      return 'Sem a prévia não dá para criar o mapa.';
    }
    if (this.preview() === 'loading' || this.layout() === null) {
      return 'Espere a prévia para criar o mapa.';
    }
    return null;
  });
  /** What the empty preview box says while the options are refused. */
  protected readonly refusedText = computed(() => {
    const all = this.refusedAll();
    if (all !== null) {
      return all;
    }
    return this.problems().size
      ? 'Corrija o tamanho para ver a masmorra.'
      : 'Corrija as opções marcadas para ver a masmorra.';
  });
  /** The preview's frame has the proportion of the dungeon the form asks for (or of the one drawn), in every state, so the page does not jump. */
  protected readonly frameRatio = computed(() => {
    const side = sideOf(this.form());
    if (side !== null && side >= 21 && side <= 121) {
      return `${side} / ${shortSideOf(side)}`;
    }
    const l = this.layout();
    return l ? `${l.width} / ${l.height}` : '31 / 21';
  });
  protected readonly anyProblem = computed(
    () =>
      Object.keys(this.problems()).length > 0 ||
      this.refusedAll() !== null ||
      this.seedProblem() !== null,
  );

  private timer: ReturnType<typeof setTimeout> | undefined;
  private inFlight = false;
  /** A run was asked for while one was on its way: it goes once more when that one is back. */
  private again = false;
  private wantNewSeed = false;
  /** Goes up on every change of an option or of the seed: an answer made for an older number is never drawn. */
  private ticket = 0;
  private abort: AbortController | undefined;
  private started = false;

  constructor() {
    const destroyed = inject(DestroyRef);
    inject(ActivatedRoute)
      .paramMap.pipe(takeUntilDestroyed(destroyed))
      .subscribe((params) => void this.load(params.get('id') ?? ''));
    // The window widened from a phone's width: the page that only said "no notebook" now has its options, and its first preview.
    effect(() => {
      if (!this.phone() && this.phase() === 'ready' && !this.started) {
        this.started = true;
        this.schedule(0);
      }
    });
    destroyed.onDestroy(() => {
      clearTimeout(this.timer);
      clearTimeout(this.slowTimer);
      // Closing the page does not stop the server (the map it was creating is in the list when the master comes back), but it stops the page
      // from taking the master to that map when the answer arrives.
      this.abort?.abort();
    });
  }

  /** Goes up on every load: an answer made for an earlier route is dropped. */
  private loadGeneration = 0;

  private async load(campaignId: string): Promise<void> {
    const generation = ++this.loadGeneration;
    this.campaignId.set(campaignId);
    this.phase.set('loading');
    try {
      const res = await this.campaigns.getCampaign(campaignId);
      if (generation !== this.loadGeneration) {
        return;
      }
      if (res.campaign?.myRole !== Role.MASTER || res.campaign.awaitingApproval) {
        this.phase.set('gone');
        return;
      }
      this.campaignName.set(res.campaign.name);
      this.nameControl.setValue(`Masmorra de ${res.campaign.name}`.slice(0, 80));
      this.phase.set('ready');
    } catch (err) {
      if (generation === this.loadGeneration) {
        this.phase.set(ConnectError.from(err).code === Code.NotFound ? 'gone' : 'error');
      }
    }
  }

  protected retryLoad(): void {
    void this.load(this.campaignId());
  }

  // ---- the options ----

  protected onForm(form: DungeonForm): void {
    this.form.set(form);
    this.refused.set({});
    this.refusedAll.set(null);
    this.schedule();
  }

  protected onSeed(text: string): void {
    this.seedText.set(text);
    this.schedule();
  }

  /** "Outra semente": a call without a seed, at once; the server's seed fills the field. */
  protected anotherSeed(): void {
    this.seedText.set('');
    this.schedule(0, true);
  }

  protected retryPreview(): void {
    this.schedule(0);
  }

  /** Asks for a preview after `delay` ms of quiet; nothing is asked while a problem the page can see is on a field. The preview stays "loading"
   * from the change until its own answer, and an answer made for an older change is dropped. */
  private schedule(delay = PREVIEW_DELAY_MS, newSeed = false): void {
    clearTimeout(this.timer);
    this.ticket++;
    this.wantNewSeed ||= newSeed;
    if (this.blocked() && !newSeed) {
      return;
    }
    this.preview.set('loading');
    this.failure.set('');
    this.timer = setTimeout(() => void this.run(), delay);
  }

  private async run(): Promise<void> {
    if (this.inFlight) {
      this.again = true;
      return;
    }
    this.inFlight = true;
    try {
      do {
        this.again = false;
        const newSeed = this.wantNewSeed;
        this.wantNewSeed = false;
        if (this.blocked() && !newSeed) {
          break;
        }
        const ticket = this.ticket;
        const seedText = newSeed ? '' : this.seedText();
        const typed = newSeed ? undefined : parseSeed(seedText);
        const options = optionsOf(this.form());
        try {
          const res = await this.api.preview(this.campaignId(), options, typed ?? undefined);
          if (ticket !== this.ticket) {
            // Something changed while it was out: this answer is for options that are gone. A newer run is waiting on its timer, or goes next.
            continue;
          }
          this.layout.set({
            width: res.width,
            height: res.height,
            open: res.open,
            doors: res.doors,
            stairs: res.stairs,
            roomCount: res.rooms.length,
          });
          this.previewed.set({ seed: res.seed, options });
          // The seed the server drew fills the field, unless the master has typed in it meanwhile.
          if (this.seedText() === seedText) {
            this.seedText.set(String(res.seed));
          }
          this.refused.set({});
          this.refusedAll.set(null);
          this.preview.set('ready');
        } catch (err) {
          if (ticket !== this.ticket) {
            continue;
          }
          const f = previewFailure(err);
          if (f.kind === 'refused') {
            if (f.field) {
              this.refused.set({ [f.field]: f.text });
            } else {
              this.refusedAll.set(f.text);
            }
            this.layout.set(null);
            this.previewed.set(null);
            this.preview.set('ready');
          } else if (f.kind === 'busy') {
            // A preview of this campaign was already running (another tab): ask again in a moment.
            await new Promise((r) => setTimeout(r, 400));
            this.again = true;
          } else {
            this.failure.set(f.text);
            this.preview.set('failed');
          }
        }
      } while (this.again);
    } finally {
      this.inFlight = false;
    }
  }

  // ---- "Criar o mapa" ----

  protected async create(): Promise<void> {
    const made = this.previewed();
    const problem = mapNameError(this.nameControl.value);
    this.nameError.set(problem);
    if (problem !== null) {
      this.nameControl.setErrors({ name: true });
      this.nameControl.markAsTouched();
      return;
    }
    if (this.whyOff() !== null || made === null || this.creating()) {
      return;
    }
    this.creating.set(true);
    this.slow.set(false);
    this.slowTimer = setTimeout(() => this.slow.set(true), 400);
    this.stopped.set(false);
    this.createError.set(null);
    this.abort = new AbortController();
    try {
      // The options and the seed of the preview on screen: the map is the one the master was shown.
      // A retry of the same preview and name sends the same key: the server answers with the map the first call made.
      const name = this.nameControl.value.trim();
      const answer = await this.api.create(
        this.campaignId(),
        name,
        made.options,
        made.seed,
        this.createKey.keyFor([name, made.options, made.seed]),
        this.abort.signal,
      );
      this.createKey.renew();
      if (this.abort.signal.aborted) {
        return;
      }
      await this.router.navigate(['/campaigns', this.campaignId(), 'maps', answer.map.id]);
    } catch (err) {
      clearTimeout(this.slowTimer);
      if (this.abort?.signal.aborted) {
        return;
      }
      this.creating.set(false);
      const f = createFailure(err);
      if (f.field) {
        this.refused.set({ [f.field]: f.text });
      } else {
        this.createError.set(f.text);
      }
    }
  }

  /** "Cancelar": the master stops waiting. The server finishes the map anyway (it is created once the options are valid), so the page says
   * where to find it instead of promising it will not exist. */
  protected cancelCreate(): void {
    clearTimeout(this.slowTimer);
    this.abort?.abort();
    this.creating.set(false);
    this.stopped.set(true);
  }

  protected retryCreate(): void {
    void this.create();
  }

  /** The options as small chips, for the creating screen. */
  private optionChips(): readonly string[] {
    const f = this.form();
    const side = sideOf(f);
    const preset = SIZE_PRESETS.find((p) => p.side === side)?.label ?? `${side ?? '?'} quadrados`;
    const shape = MASKS.find((m) => m.value === f.mask)?.label ?? '';
    const corridors = CORRIDORS.find((c) => c.value === f.corridor)?.label ?? '';
    const doors = DOOR_MIXES.find((d) => d.value === f.doors)?.label ?? '';
    return [
      preset,
      shape,
      `Salas de ${f.roomMinText} a ${f.roomMaxText} quadrados de lado`,
      `Corredores ${corridors.toLowerCase()}`,
      `Portas ${doors.toLowerCase()}`,
      `Becos ${f.deadends} %`,
      stairCountText(f.stairs).replace(/^./, (c) => c.toUpperCase()),
      `Semente ${this.seedText()}`,
    ];
  }
}
