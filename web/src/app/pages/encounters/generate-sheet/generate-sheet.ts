import { NgTemplateOutlet } from '@angular/common';
import {
  afterNextRender,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import {
  EncounterBand,
  type EncounterEvaluation,
  type EncounterLine,
} from '../../../../gen/meurpg/play/v1/encounters_pb';
import type { CreatureSummary } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { MonsterGroupSpec } from '../../../core/combat/combat-client';
import { CREATURE_TYPES } from '../../../core/creatures/creature-types';
import { type DraftEntry } from '../../../core/encounters/encounter-draft';
import {
  EncountersClient,
  type GenerateBand,
  type PartyNpcSpec,
} from '../../../core/encounters/encounters-client';
import {
  GUIDE_CAVEAT,
  GUIDE_LABEL,
  bandWord,
  capLine,
  encounterErrorMessage,
  headline,
  partyByLevel,
  warningLines,
} from '../../../core/encounters/encounter-text';
import { formatInt, tight } from '../../../core/format/text';
import { Segmented, type Segment } from '../../live-session/combat/move-page/segmented';
import { SheetFrame } from '../../live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../live-session/combat/sheet-host';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { SwapPanel } from '../swap-panel/swap-panel';

/** What the builder hands the sheet. */
export interface GenerateData {
  readonly campaignId: string;
  readonly campaignName: string;
  readonly party: readonly PartyNpcSpec[];
  /** The last measure of the builder: the party and the budgets, for the lead and the hint. */
  readonly evaluation: EncounterEvaluation | null;
}

/** What closes the sheet with "Usar este encontro": the lines and the seed they came from (`null` once one was swapped by hand). */
export interface GenerateResult {
  readonly entries: readonly DraftEntry[];
  readonly seed: number | null;
  readonly creatureType: string;
}

const BANDS: readonly Segment<GenerateBand>[] = [
  { value: 'low', label: 'Baixa' },
  { value: 'moderate', label: 'Moderada' },
  { value: 'high', label: 'Alta' },
];

/**
 * "Gerar encontro" (MR-043, E10-09 states 4 and 5 and the phone's): a difficulty (Baixa, Moderada, Alta) and, if the master
 * wants, a creature type; the result shows at once, with its seed in small type. "Gerar outro" asks the server for another (seed 0:
 * it draws one and says which); the same difficulty, type and seed always give the same encounter. "Trocar criatura" lists the
 * creatures with the same XP (the encounter's own are left out), the count stays, and the server measures the result again.
 * "Usar este encontro" puts the result in the builder, where it can still be changed.
 *
 * The server decides everything: the band's budget, the cap on a creature's rating (lowest level plus 3), what fits. The
 * refusals (`NO_PARTY`, `NOTHING_FITS`) are typed and said in words under the options.
 */
@Component({
  selector: 'app-generate-sheet',
  imports: [
    CreatureArt,
    MatButtonModule,
    NgTemplateOutlet,
    MatIconModule,
    RouterLink,
    Segmented,
    SheetFrame,
    SwapPanel,
  ],
  templateUrl: './generate-sheet.html',
  styleUrl: './generate-sheet.scss',
})
export class GenerateSheet {
  private readonly api = inject(EncountersClient);
  private readonly sheet = injectSheet<GenerateData, GenerateResult>();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly bands = BANDS;
  protected readonly types = CREATURE_TYPES;
  protected readonly guide = GUIDE_LABEL;
  protected readonly caveat = GUIDE_CAVEAT;
  protected readonly format = formatInt;

  protected readonly band = signal<GenerateBand>('moderate');
  protected readonly type = signal('');
  protected readonly result = signal<EncounterEvaluation | null>(null);
  /** The seed the server used; `null` after a swap by hand. */
  protected readonly seed = signal<number | null>(null);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  /** The line being swapped, the creatures it can become and the one picked. */
  protected readonly swapping = signal<EncounterLine | null>(null);
  protected readonly swaps = signal<readonly CreatureSummary[] | null>(null);
  protected readonly swapPick = signal('');
  protected readonly swapError = signal('');
  protected readonly swapBusy = signal(false);

  private version = 0;

  protected readonly lead = computed(() => {
    const party = this.data.evaluation?.party ?? this.result()?.party ?? [];
    return party.length > 0
      ? `Para o grupo de ${this.data.campaignName}: ${partyByLevel(party)}`
      : `Para o grupo de ${this.data.campaignName}`;
  });
  protected readonly bandHint = computed(() => {
    const b = this.data.evaluation?.budget;
    const [band, xp] = {
      low: [EncounterBand.LOW, b?.low ?? 0],
      moderate: [EncounterBand.MODERATE, b?.moderate ?? 0],
      high: [EncounterBand.HIGH, b?.high ?? 0],
    }[this.band()] as [EncounterBand, number];
    return xp > 0 ? tight(`${bandWord(band)}: até ${formatInt(xp)} XP para o seu grupo.`) : '';
  });
  protected readonly head = computed(() => (this.result() ? headline(this.result()!) : ''));
  protected readonly cap = computed(() => (this.result() ? capLine(this.result()!) : ''));
  protected readonly warnings = computed(() => (this.result() ? warningLines(this.result()!) : []));
  protected readonly swapPickName = computed(
    () => this.swapNames()?.find((c) => c.key === this.swapPick())?.namePt ?? '',
  );
  /** The creatures it can become: the server's list without the ones the encounter already has; `null` while it answers. */
  protected readonly swapNames = computed(() => {
    const list = this.swaps();
    const have = new Set((this.result()?.lines ?? []).map((l) => l.creature?.key));
    return list === null ? null : list.filter((c) => !have.has(c.key));
  });

  constructor() {
    void this.generate();
  }

  protected setBand(band: GenerateBand): void {
    this.band.set(band);
    void this.generate();
  }

  protected setType(type: string): void {
    this.type.set(type);
    void this.generate();
  }

  /** Draws one (seed 0: the server picks the seed and says which). */
  protected async generate(): Promise<void> {
    const mine = ++this.version;
    this.busy.set(true);
    this.error.set('');
    this.swapping.set(null);
    try {
      const res = await this.api.generate(
        this.data.campaignId,
        this.band(),
        this.type(),
        0,
        this.data.party,
      );
      if (mine === this.version) {
        this.result.set(res.evaluation);
        this.seed.set(res.seed);
      }
    } catch (err) {
      if (mine === this.version) {
        this.result.set(null);
        this.error.set(encounterErrorMessage(err, 'generate'));
      }
    } finally {
      if (mine === this.version) {
        this.busy.set(false);
      }
    }
  }

  protected async startSwap(line: EncounterLine): Promise<void> {
    this.swapping.set(line);
    this.swaps.set(null);
    this.swapPick.set('');
    this.swapError.set('');
    // The panel's title takes the focus: it is where the person looks next.
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>('#swap-h')?.focus(), {
      injector: this.injector,
    });
    try {
      const list = await this.api.swaps(
        this.data.campaignId,
        line.creature?.key ?? '',
        this.type(),
      );
      if (this.swapping() === line) {
        this.swaps.set(list);
        // The first creature is the one the focus lands on, as in a radio group that has no choice yet.
      }
    } catch (err) {
      if (this.swapping() === line) {
        this.swapError.set(encounterErrorMessage(err, 'swap'));
        this.swaps.set([]);
      }
    }
  }

  protected cancelSwap(): void {
    this.swapping.set(null);
  }

  /** "Trocar por Orc": the same count of the other creature; the server measures the encounter again. */
  protected async confirmSwap(): Promise<void> {
    const line = this.swapping();
    const next = this.swaps()?.find((c) => c.key === this.swapPick());
    const current = this.result();
    if (!line || !next || !current || this.swapBusy()) {
      return;
    }
    const mine = this.version;
    this.swapBusy.set(true);
    this.swapError.set('');
    try {
      const entries: MonsterGroupSpec[] = current.lines.map((l) => ({
        creatureKey: l === line ? next.key : (l.creature?.key ?? ''),
        count: l.count,
      }));
      const swapped = await this.api.evaluate(this.data.campaignId, entries, this.data.party);
      // A new draw (another band or type) while this answered is the encounter on screen: the swap was of the old one.
      if (mine !== this.version) {
        return;
      }
      this.result.set(swapped);
      this.seed.set(null);
      this.swapping.set(null);
    } catch (err) {
      if (mine === this.version) {
        this.swapError.set(encounterErrorMessage(err, 'swap'));
      }
    } finally {
      this.swapBusy.set(false);
    }
  }

  protected use(): void {
    const ev = this.result();
    if (!ev || this.busy()) {
      return;
    }
    this.sheet.close({
      entries: ev.lines
        .filter((l) => l.creature)
        .map((l) => ({ creature: l.creature!, count: l.count })),
      seed: this.seed(),
      creatureType: this.type(),
    });
  }

  protected close(): void {
    this.sheet.close();
  }
}
