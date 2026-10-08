import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { CreatureSummary } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { type BestiaryAccess, BestiaryAccessCheck } from '../../../core/creatures/bestiary-access';
import {
  EncounterDraft,
  ENTRY_MAX,
  KINDS_MAX,
  PARTY_NPC_MAX,
} from '../../../core/encounters/encounter-draft';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import {
  GUIDE_CAVEAT,
  GUIDE_LABEL,
  capLine,
  encounterErrorMessage,
  headline,
  warningLines,
} from '../../../core/encounters/encounter-text';
import { formatInt, tight } from '../../../core/format/text';
import { RosterClient } from '../../../core/maps/roster-client';
import { openSheet } from '../../live-session/combat/sheet-host';
import { BudgetBar } from '../budget-bar/budget-bar';
import { EncounterRows } from '../encounter-rows/encounter-rows';
import { PartyChips } from '../party-chips/party-chips';
import { CreaturePick } from '../creature-pick/creature-pick';
import {
  GenerateSheet,
  type GenerateData,
  type GenerateResult,
} from '../generate-sheet/generate-sheet';
import { PartyNpcSheet, type PartyNpcData } from '../party-npc-sheet/party-npc-sheet';
import { SaveSheet, type SaveData, type SaveResult } from '../save-sheet/save-sheet';
import type { DraftNpc } from '../../../core/encounters/encounter-draft';

type PageState = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready' };

/**
 * "/campaigns/:id/encounters" (MR-043, RN-29, E10-09): the master's encounter builder. The party (the living player
 * characters, plus NPCs he adds with a level), the bar of the difficulty, the creatures and their counts, "Gerar encontro"
 * and "Guardar no ponto de batalha". The browser does no maths: every change asks the server to measure the encounter
 * (`EncounterDraft`: debounced, one at a time, stale answers dropped) and the page draws the budgets, the band, the total
 * and the warnings it answers. Past the high budget the band is "Acima de alta": allowed, warned, never "mortal".
 *
 * Only the master gets the page: the builder and a saved encounter are his secret (RN-10), and the server answers a player
 * with `not_found`. `?map=&point=` (from a battle point of the map editor) opens "Guardar" on that point.
 */
@Component({
  selector: 'app-encounter-builder',
  imports: [
    BudgetBar,
    CreaturePick,
    EncounterRows,
    PartyChips,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
  ],
  templateUrl: './encounter-builder.html',
  styleUrl: './encounter-builder.scss',
})
export class EncounterBuilder {
  private readonly route = inject(ActivatedRoute);
  private readonly accessCheck = inject(BestiaryAccessCheck);
  private readonly roster = inject(RosterClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  protected readonly campaignId = this.route.snapshot.paramMap.get('id') ?? '';
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly encounters = inject(EncountersClient);
  protected readonly draft = new EncounterDraft(this.encounters, this.campaignId);
  /** The battle point whose saved encounter is on screen (`?point=`), and the question "Tirar o encontro?" in place. */
  protected readonly keptAt = signal<{ readonly mapId: string; readonly pointId: string } | null>(
    null,
  );
  protected readonly clearing = signal(false);
  protected readonly clearBusy = signal(false);
  /** Saved creatures the SRD no longer has: left out of the draft, and gone from the point once it is saved again. */
  protected readonly lost = signal(0);
  protected readonly access = signal<BestiaryAccess | { status: 'loading' }>({ status: 'loading' });
  protected readonly state = signal<PageState>({ status: 'loading' });
  /** What the last "Guardar" kept, for the line above the page. */
  protected readonly saved = signal<SaveResult | null>(null);
  protected readonly npcError = signal('');

  protected readonly guide = GUIDE_LABEL;
  protected readonly caveat = GUIDE_CAVEAT;
  protected readonly format = formatInt;
  protected readonly entryMax = ENTRY_MAX;

  protected readonly ev = this.draft.evaluation;
  protected readonly head = computed(() => (this.ev() ? headline(this.ev()!) : ''));
  protected readonly cap = computed(() => (this.ev() ? capLine(this.ev()!) : ''));
  protected readonly warnings = computed(() => (this.ev() ? warningLines(this.ev()!) : []));
  protected readonly total = computed(() => tight(`${formatInt(this.ev()?.totalXp ?? 0)} XP`));
  protected readonly creatures = computed(() => {
    const n = this.draft.creatureCount();
    return n === 1 ? '1 criatura' : `${n} criaturas`;
  });
  protected readonly canAddKind = computed(() => this.draft.entries().length < KINDS_MAX);
  protected readonly canAddNpc = computed(() => this.draft.npcs().length < PARTY_NPC_MAX);
  /** What each row shows: the draft's own count at once, the XP the server measured for it (dimmed while a newer measure is on its way). */
  protected readonly rows = computed(() => {
    const lines = new Map((this.ev()?.lines ?? []).map((l) => [l.creature?.key, l]));
    return this.draft.entries().map((e) => {
      const line = lines.get(e.creature.key);
      const fresh = line?.count === e.count;
      return {
        creature: e.creature,
        count: e.count,
        each: tight(`${formatInt(e.creature.xp)} XP cada`),
        subtotal: fresh ? tight(`${formatInt(line!.subtotalXp)} XP`) : '…',
        aboveCap: fresh && line!.aboveCap,
      };
    });
  });
  /** The chips of the party: the server's list (player characters first, then the NPCs in the order added). */
  protected readonly party = computed(() => {
    const members = this.ev()?.party ?? [];
    let npcIndex = -1;
    return members.map((m) => {
      if (m.npc) {
        npcIndex++;
      }
      return {
        name: m.name,
        level: m.level,
        npc: m.npc,
        index: m.npc ? npcIndex : -1,
        characterId: m.characterId,
      };
    });
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => this.draft.stop());
    void this.start();
    const query = this.route.snapshot.queryParamMap;
    this.fromPoint = { mapId: query.get('map') ?? '', pointId: query.get('point') ?? '' };
  }

  private readonly fromPoint: { mapId: string; pointId: string };

  protected async start(): Promise<void> {
    this.access.set({ status: 'loading' });
    const access = await this.accessCheck.check(this.campaignId);
    this.access.set(access);
    if (access.status === 'master') {
      this.state.set({ status: 'ready' });
      await this.loadPoint();
      this.draft.measureNow();
    }
  }

  /** A link from a battle point (`?map=&point=`) brings its saved encounter into the draft, so the master changes it, replaces it or takes it off. */
  private async loadPoint(): Promise<void> {
    const { mapId, pointId } = this.fromPoint;
    if (!pointId) {
      return;
    }
    try {
      const read = await this.encounters.get(this.campaignId, pointId);
      if (!read.encounter) {
        return;
      }
      this.keptAt.set({ mapId, pointId });
      this.lost.set(read.unknownKeys.length);
      const have = new Map(
        (read.evaluation?.lines ?? []).map((l) => [l.creature?.key, l.creature]),
      );
      const entries = read.encounter.monsters.flatMap((m) => {
        const creature = have.get(m.creatureKey);
        return creature ? [{ creature, count: m.count }] : [];
      });
      this.draft.replace(entries);
    } catch {
      // The point is not readable (gone, or not a battle point): the builder opens empty, as without the link.
    }
  }

  protected askClear(): void {
    this.clearing.set(true);
    // The question is drawn on the next render: "Voltar" takes the focus.
    afterNextRender(
      () => this.host.nativeElement.querySelector<HTMLElement>('[data-initial-focus]')?.focus(),
      { injector: this.injector },
    );
  }

  /** "Tirar o encontro" (`ClearBattleEncounter`): the point keeps nothing; the draft stays on screen. */
  protected async clearPoint(): Promise<void> {
    const at = this.keptAt();
    if (!at || this.clearBusy()) {
      return;
    }
    this.clearBusy.set(true);
    try {
      await this.encounters.clear(this.campaignId, at.pointId);
      this.keptAt.set(null);
      this.clearing.set(false);
      this.lost.set(0);
      this.saved.set(null);
      this.npcError.set('');
      this.cleared.set(true);
    } catch (err) {
      this.npcError.set(encounterErrorMessage(err, 'clear'));
    } finally {
      this.clearBusy.set(false);
    }
  }
  protected readonly cleared = signal(false);

  protected setCount(key: string, count: number): void {
    this.saved.set(null);
    this.draft.setCount(key, count);
  }

  protected add(creature: CreatureSummary): void {
    this.saved.set(null);
    this.draft.add(creature);
  }

  protected async openNpc(): Promise<void> {
    this.npcError.set('');
    let npcs;
    try {
      const have = new Set(this.draft.npcs().map((n) => n.characterId));
      npcs = (await this.roster.list(this.campaignId)).filter(
        (e) => e.kind !== CharacterKind.PLAYER && !have.has(e.id),
      );
    } catch {
      this.npcError.set(
        'Não deu para ler os NPCs da campanha: o servidor não respondeu. Tente de novo.',
      );
      return;
    }
    openSheet<PartyNpcSheet, PartyNpcData, DraftNpc>(this.dialog, this.bottomSheet, PartyNpcSheet, {
      data: {
        campaignId: this.campaignId,
        npcs,
        entries: this.draft.specs(),
        party: this.draft.party(),
        before: this.ev(),
      },
      ariaLabel: 'Pôr um NPC no grupo',
      labelledBy: 'pn-t',
      width: '620px',
      tall: true,
      focus: 'input[type=radio]:checked',
    }).subscribe((npc) => {
      if (npc) {
        this.saved.set(null);
        this.draft.addNpc(npc);
      }
    });
  }

  protected removeNpc(index: number): void {
    const chip = this.party().find((m) => m.npc && m.index === index);
    if (!chip) {
      return;
    }
    this.saved.set(null);
    this.draft.removeNpc(chip);
  }

  protected openGenerate(): void {
    openSheet<GenerateSheet, GenerateData, GenerateResult>(
      this.dialog,
      this.bottomSheet,
      GenerateSheet,
      {
        data: {
          campaignId: this.campaignId,
          campaignName:
            this.access().status === 'master'
              ? (this.access() as { campaignName: string }).campaignName
              : '',
          party: this.draft.party(),
          evaluation: this.ev(),
        },
        ariaLabel: 'Gerar encontro',
        labelledBy: 'gen-t',
        width: '760px',
        tall: true,
        focus: 'input[type=radio]:checked',
      },
    ).subscribe((result) => {
      if (result) {
        this.saved.set(null);
        this.draft.replace(result.entries, result.seed);
      }
    });
  }

  protected openSave(): void {
    if (this.draft.entries().length === 0) {
      // The button is only dimmed while the draft is empty (it keeps focus), so a click still comes.
      return;
    }
    const data: SaveData = {
      campaignId: this.campaignId,
      entries: this.draft.specs(),
      party: this.draft.party(),
      evaluation: this.ev(),
      mapId: this.fromPoint.mapId,
      pointId: this.fromPoint.pointId,
    };
    openSheet<SaveSheet, SaveData, SaveResult>(this.dialog, this.bottomSheet, SaveSheet, {
      data,
      ariaLabel: 'Guardar no ponto de batalha',
      labelledBy: 'save-t',
      width: '680px',
      tall: true,
      focus: 'select',
    }).subscribe((result) => {
      if (result) {
        this.saved.set(result);
        this.cleared.set(false);
        this.keptAt.set({ mapId: result.mapId, pointId: result.pointId });
        this.clearing.set(false);
      }
    });
  }

  /** The master's page is also where a lost read is retried. */
  protected retry(): void {
    this.draft.measureNow();
  }

  protected readonly savedLine = computed(() => {
    const s = this.saved();
    return s?.evaluation
      ? `${headline(s.evaluation)} · ${s.evaluation.creatureCount} criaturas`
      : '';
  });
}
