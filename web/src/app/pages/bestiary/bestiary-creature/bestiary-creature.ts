import {
  Component,
  ElementRef,
  afterNextRender,
  inject,
  signal,
  viewChild,
  Injector,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Creature } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { type BestiaryAccess, BestiaryAccessCheck } from '../../../core/creatures/bestiary-access';
import { bestiaryErrorMessage } from '../../../core/creatures/bestiary-errors';
import {
  alignmentPt,
  capitalized,
  creatureKeyOf,
  listWithE,
} from '../../../core/creatures/bestiary-format';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { StatBlock } from '../../../shared/creatures/stat-block';
import { openSheet } from '../../../shared/sheet/sheet-host';
import {
  CreateNpcSheet,
  type CreateNpcData,
  type CreateNpcResult,
} from '../create-npc-sheet/create-npc-sheet';
import {
  PutMonstersSheet,
  type PutMonstersData,
  type PutMonstersResult,
} from '../../../shared/monsters/put-sheet/put-sheet';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; creature: Creature };

/**
 * "/campaigns/:id/bestiary/:slug" (MR-042, E10-08, state 3): one creature's stat block, for the
 * master. The numbers and the Portuguese labels come from `GetCreature`; the text of the traits and
 * actions is the SRD's English (`StatBlock`, marked `lang="en"`), and "SRD" is credited under the
 * panel. "Criar NPC" opens the dialog; the NPC it makes is announced here, with a way to its sheet.
 * "Pôr no combate" (MR-042, 10.17b) opens the sheet that puts the creature in the session's combat; the
 * monsters that went in are announced here, with the way to the session.
 */
@Component({
  selector: 'app-bestiary-creature',
  imports: [
    CreatureArt,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
    StatBlock,
  ],
  templateUrl: './bestiary-creature.html',
  styleUrl: './bestiary-creature.scss',
})
export class BestiaryCreature {
  private readonly route = inject(ActivatedRoute);
  private readonly client = inject(CreaturesClient);
  private readonly accessCheck = inject(BestiaryAccessCheck);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);

  protected readonly campaignId = this.route.snapshot.paramMap.get('id') ?? '';
  private readonly key = creatureKeyOf(this.route.snapshot.paramMap.get('slug') ?? '');
  protected readonly access = signal<BestiaryAccess | { status: 'loading' }>({ status: 'loading' });
  protected readonly state = signal<PageState>({ status: 'loading' });
  /** The NPC just made, for the confirmation. */
  protected readonly made = signal<CreateNpcResult | null>(null);

  /** The monsters just put in the combat, for the confirmation. */
  protected readonly put = signal<PutMonstersResult | null>(null);

  private readonly madeBox = viewChild<ElementRef<HTMLElement>>('madeBox');
  private readonly createButton = viewChild('createButton', {
    read: ElementRef<HTMLButtonElement>,
  });
  private readonly putButton = viewChild('putButton', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    void this.start();
  }

  protected async start(): Promise<void> {
    this.access.set({ status: 'loading' });
    const access = await this.accessCheck.check(this.campaignId);
    this.access.set(access);
    if (access.status === 'master') {
      await this.load();
    }
  }

  protected async load(): Promise<void> {
    this.state.set({ status: 'loading' });
    try {
      this.state.set({
        status: 'ready',
        creature: await this.client.statBlock(this.campaignId, this.key),
      });
    } catch (err) {
      this.state.set(
        ConnectError.from(err, Code.Unavailable).code === Code.NotFound
          ? { status: 'not-found' }
          : { status: 'error', message: bestiaryErrorMessage(err, 'read') },
      );
    }
  }

  protected readonly attackList = listWithE;

  protected typeLine(c: Creature): {
    readonly en: string;
    readonly rest: string;
    readonly alignment: { text: string; english: boolean };
  } {
    const s = c.summary;
    return {
      en: s?.name ?? '',
      rest: [s?.sizePt, capitalized(s?.typePt ?? '')].filter((p) => p).join(' · '),
      alignment: alignmentPt(c.alignment),
    };
  }

  protected openPut(creature: Creature): void {
    if (!creature.summary) {
      return;
    }
    openSheet<PutMonstersSheet, PutMonstersData, PutMonstersResult>(
      this.dialog,
      this.bottomSheet,
      PutMonstersSheet,
      {
        data: { campaignId: this.campaignId, creature: creature.summary },
        ariaLabel: 'Pôr no combate',
        labelledBy: 'put-t',
        width: '600px',
        tall: true,
        restoreFocus: false,
      },
    ).subscribe((result) => {
      if (result) {
        this.made.set(null);
        this.put.set(result);
        afterNextRender(
          () => {
            const box = this.madeBox()?.nativeElement;
            box?.scrollIntoView?.({ block: 'nearest' });
            focusWithRing(box);
          },
          { injector: this.injector },
        );
      } else {
        focusWithRing(this.putButton()?.nativeElement);
      }
    });
  }

  protected openCreate(creature: Creature): void {
    openSheet<CreateNpcSheet, CreateNpcData, CreateNpcResult>(
      this.dialog,
      this.bottomSheet,
      CreateNpcSheet,
      {
        data: { campaignId: this.campaignId, creature },
        ariaLabel: 'Criar NPC',
        labelledBy: 'npc-t',
        width: '540px',
        tall: true,
        focus: 'input[name=name]',
        restoreFocus: false,
      },
    ).subscribe((result) => {
      if (result) {
        this.put.set(null);
        this.made.set(result);
        // The confirmation is where the person looks next: it comes into view and takes the focus.
        afterNextRender(
          () => {
            const box = this.madeBox()?.nativeElement;
            box?.scrollIntoView?.({ block: 'nearest' });
            focusWithRing(box);
          },
          { injector: this.injector },
        );
      } else {
        focusWithRing(this.createButton()?.nativeElement);
      }
    });
  }
}
