import { DOCUMENT } from '@angular/common';
import {
  ApplicationRef,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { MapsClient } from '../../../core/maps/maps-client';
import { PrintPreview } from './print-preview/print-preview';
import { PrintSetup } from './print-setup/print-setup';
import { PrintSheets } from './print-sheets/print-sheets';
import { PrintTable } from './print-table/print-table';
import {
  DEFAULT_SQUARE_CM,
  PAPERS,
  type PaperId,
  leanestPaper,
  mapSizeCm,
  parseSquareCm,
  plansForPaper,
} from './print-math';

type Phase = 'loading' | 'ready' | 'gone' | 'forbidden' | 'error';

/**
 * `/campaigns/:id/maps/:mapId/print` (MR-033, E8-12, Q65): the master
 * prints the map with its grid to scale. The page holds what the master
 * chose (the square's size and the paper) and works out the rest: how the map
 * is cut into sheets, which orientation spends fewer, the page preview and
 * the arithmetic. What comes out of the printer is only print CSS: the sheets
 * (`PrintSheets`) are hidden on screen and are all that shows in
 * `@media print`; the `@page` rule for the chosen paper is written here. No
 * server PDF: the browser's own "Imprimir" does the job.
 */
@Component({
  selector: 'app-map-print',
  imports: [
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    PrintPreview,
    PrintSetup,
    PrintSheets,
    PrintTable,
    RouterLink,
  ],
  templateUrl: './map-print.html',
  styleUrl: './map-print.scss',
})
export class MapPrint {
  private readonly api = inject(MapsClient);
  private readonly appRef = inject(ApplicationRef);
  private readonly campaigns = inject(CampaignsService);
  private readonly document = inject(DOCUMENT);
  private readonly route = inject(ActivatedRoute);

  protected readonly campaignId = signal('');
  protected readonly mapId = signal('');
  protected readonly phase = signal<Phase>('loading');
  protected readonly map = signal<MapMessage | null>(null);
  protected readonly campaignName = signal('');

  /** The field as typed ("2,54", "5"): shown back exactly so. */
  protected readonly typed = signal(String(DEFAULT_SQUARE_CM).replace('.', ','));
  protected readonly paperId = signal<PaperId>('a4');
  /** True only while the browser prints: the sheets are not in the page
   * before that (a big map is thousands of them). */
  protected readonly printing = signal(false);

  protected readonly squareCm = computed(() => parseSquareCm(this.typed()));
  protected readonly hasGrid = computed(() => (this.map()?.gridColumns ?? 0) > 0);
  protected readonly image = computed(() => this.map()?.image ?? null);
  protected readonly mapSize = computed(() => {
    const m = this.map();
    const image = m?.image;
    const square = this.squareCm();
    return m && image && square !== null && m.gridColumns > 0
      ? mapSizeCm(m.gridColumns, image.width, image.height, square)
      : null;
  });
  /** Every paper's arithmetic, for "As contas" and for the notice. */
  protected readonly allPlans = computed(() => {
    const size = this.mapSize();
    return size ? PAPERS.map((p) => plansForPaper(p, size)) : null;
  });
  protected readonly chosen = computed(
    () => this.allPlans()?.find((p) => p.paper.id === this.paperId()) ?? null,
  );
  protected readonly leanest = computed(() => {
    const all = this.allPlans();
    return all ? leanestPaper(all) : null;
  });
  protected readonly backLink = computed(() => [
    '/campaigns',
    this.campaignId(),
    'maps',
    this.mapId(),
  ]);

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const id = params.get('id');
      const mapId = params.get('mapId');
      if (id && mapId) {
        void this.load(id, mapId);
      }
    });

    // The sheets are drawn when the browser is about to print, whoever asked
    // (the button, Ctrl+P, the menu): `beforeprint` runs before it takes the
    // page, so the render is flushed right there; `afterprint` takes them out.
    const beforePrint = (): void => {
      this.printing.set(true);
      this.appRef.tick();
    };
    const afterPrint = (): void => this.printing.set(false);
    window.addEventListener('beforeprint', beforePrint);
    window.addEventListener('afterprint', afterPrint);
    inject(DestroyRef).onDestroy(() => {
      window.removeEventListener('beforeprint', beforePrint);
      window.removeEventListener('afterprint', afterPrint);
    });

    // The browser takes the paper size and the margins from `@page`, which a
    // component's styles cannot vary: one <style> in the head, for as long
    // as this page is open, says it for the chosen paper.
    const style = this.document.createElement('style');
    this.document.head.appendChild(style);
    inject(DestroyRef).onDestroy(() => style.remove());
    effect(() => {
      const plan = this.chosen()?.best;
      style.textContent = plan
        ? `@page { size: ${plan.paperW}cm ${plan.paperH}cm; margin: 1cm; }`
        : '';
    });
  }

  /** Goes up on every load: an answer made for an earlier route is dropped. */
  private generation = 0;

  private async load(campaignId: string, mapId: string): Promise<void> {
    const generation = ++this.generation;
    this.campaignId.set(campaignId);
    this.mapId.set(mapId);
    this.phase.set('loading');
    try {
      const [campaign, map] = await Promise.all([
        this.campaigns.getCampaign(campaignId),
        this.api.get(campaignId, mapId),
      ]);
      if (generation !== this.generation) {
        return;
      }
      if (campaign.campaign?.awaitingApproval || !map.map) {
        this.phase.set('gone');
        return;
      }
      this.campaignName.set(campaign.campaign?.name ?? '');
      if (campaign.campaign?.myRole !== Role.MASTER) {
        this.phase.set('forbidden');
        return;
      }
      this.map.set(map.map);
      this.phase.set('ready');
    } catch (err) {
      if (generation === this.generation) {
        this.phase.set(ConnectError.from(err).code === Code.NotFound ? 'gone' : 'error');
      }
    }
  }

  protected print(): void {
    if (this.chosen()) {
      window.print();
    }
  }
}
