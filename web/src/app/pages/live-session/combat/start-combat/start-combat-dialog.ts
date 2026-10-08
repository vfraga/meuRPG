import { Component, computed, effect, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogRef, MAT_DIALOG_DATA } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { RouterLink } from '@angular/router';

import {
  DiceMode,
  DicePreference,
  Role,
} from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../../../gen/meurpg/characters/v1/characters_pb';
import {
  type Encounter,
  EncounterBlockedReason,
  EncounterMode,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { DiceOption } from '../../../../core/campaigns/dice-labels';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { THEATRE_WHY, THEATRE_WHY_DIALOG } from '../../../../core/combat/theatre';
import { DiceChoice } from '../../../../shared/dice-choice/dice-choice';
import { CampaignsService } from '../../../../core/campaigns/campaigns.service';
import { effectivePreference, preferenceLabel } from '../../../../core/campaigns/dice-labels';
import {
  CombatClient,
  type JoinSpec,
  type MonsterHp,
  newKey,
} from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import { becomesText } from '../../../../core/combat/monsters';
import { HiddenSwitch } from '../../../../shared/hidden-switch/hidden-switch';
import { type Segment, Segmented } from '../move-page/segmented';
import { combatErrorMessage, encounterBlocked } from '../../../../core/combat/combat-errors';
import { formatMeters, squaresToMeters } from '../../../../core/units';
import { combatantInitial, npcKindLabel } from '../../../../core/combat/combat-view';
import { type RosterEntry, RosterClient } from '../../../../core/maps/roster-client';
import { CombatMap } from '../../../../shared/combat-map/combat-map';
import { NpcRow } from './npc-row';
import { SavedMonstersList } from './saved-monsters';
import { PlayerRow } from './player-row';

/** The map the fight is on (the session's current map). */
export interface CombatMapInfo {
  readonly id: string;
  readonly name: string;
  readonly image: { readonly url: string; readonly width: number; readonly height: number };
  /** 0 when the map has no grid. */
  readonly columns: number;
  readonly rows: number;
}

/** One kind of creature of a saved encounter, as the dialog lists it (read only: the encounter is the page's). */
export interface SavedMonsters {
  readonly key: string;
  readonly namePt: string;
  readonly count: number;
  /** The base name the point kept for the group (empty: the creature's Portuguese name). */
  readonly name?: string;
}

/** "Começar este combate" (MR-043): what the dialog starts with, from the encounter a battle point keeps. */
export interface SavedStart {
  /** The battle point it was kept on: the combat starts from it. */
  readonly pointId: string;
  readonly pointName: string;
  readonly groups: readonly SavedMonsters[];
  readonly hp: MonsterHp;
  readonly hidden: boolean;
}

export interface StartCombatData {
  readonly campaignId: string;
  /** `start`: "Iniciar combate"; `add`: reinforcements for a running combat. */
  readonly mode: 'start' | 'add';
  readonly map: CombatMapInfo | null;
  /** The running combat, for `add`. */
  readonly encounterId?: string;
  /** How many combatants it already has (the limit is 40 in all). */
  readonly existing?: number;
  /** "Começar este combate": the monsters of a saved encounter come in with the party. */
  readonly saved?: SavedStart;
}

/** The server's limit (combat.proto): 40 combatants in a combat. */
const MAX_COMBATANTS = 40;

/**
 * "Iniciar combate" (E6-01) and "Adicionar combatente" (the same dialog,
 * with the NPCs only): the master names the combat, sees the map it is on
 * (a map without a grid cannot start one: "Definir a grade"), checks who of
 * the party joins and how each rolls, and says how many copies of each NPC
 * fight. An NPC starts hidden unless the master says otherwise (question 31).
 * A phone gets it full screen, with the footer pinned so "Iniciar combate"
 * is always in reach. It calls `CombatService` itself and closes with the
 * combat, so a refusal shows here with its reason.
 */
@Component({
  selector: 'app-start-combat-dialog',
  imports: [
    CombatMap,
    DiceChoice,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    HiddenSwitch,
    NpcRow,
    PlayerRow,
    RouterLink,
    SavedMonstersList,
    Segmented,
  ],
  templateUrl: './start-combat-dialog.html',
  styleUrl: './start-combat-dialog.scss',
})
export class StartCombatDialog {
  protected readonly data = inject<StartCombatData>(MAT_DIALOG_DATA);
  private readonly ref = inject<MatDialogRef<StartCombatDialog, Encounter>>(MatDialogRef);
  private readonly combat = inject(CombatClient);
  private readonly roster = inject(RosterClient);
  private readonly campaigns = inject(CampaignsService);
  private readonly tableRules = inject(TableRulesClient);
  /** One key for this dialog: a second tap on the button can't start two. */
  private readonly key = newKey();
  /** The key of "Adicionar": a tap after a lost answer is the same addition, other NPCs another. */
  private readonly addKey = new ActionKey();

  protected readonly adding = this.data.mode === 'add';
  /** How the combat is played (RN-25), fixed once it starts: with the table's "combate com mapa" rule as the default, and "Sem mapa" when the session has no map. */
  protected readonly playMode = signal<EncounterMode>(
    this.data.map && this.data.map.columns > 0 ? EncounterMode.GRID : EncounterMode.THEATRE,
  );
  /** The person chose: the table's rule, read a moment later, no longer moves the choice. */
  private modeTouched = false;
  protected readonly theatre = computed(() => this.playMode() === EncounterMode.THEATRE);
  protected readonly modeOptions: readonly DiceOption<EncounterMode>[] = [
    {
      value: EncounterMode.GRID,
      title: 'Com mapa',
      description: 'Posições, alcance, névoa, armadilhas e portas valem. Pede um mapa com grade.',
    },
    {
      value: EncounterMode.THEATRE,
      title: 'Sem mapa (teatro da mente)',
      description:
        'Só a ordem, o movimento por número e a sua palavra: você decide quem está ao alcance. Não precisa de mapa.',
    },
  ];
  /** "Com mapa" waits when the session has no map: dashed, with the reason where its line is. */
  protected readonly modeInert = computed<readonly EncounterMode[]>(() =>
    this.mapReady() ? [] : [EncounterMode.GRID],
  );
  protected readonly modeNotes = computed<Partial<Record<number, string>>>(() =>
    this.mapReady()
      ? {}
      : {
          [EncounterMode.GRID]: this.data.map
            ? 'O mapa atual não tem grade.'
            : 'A sessão não tem um mapa atual.',
        },
  );
  /** A map with a grid: a map without one counts as no map for "Com mapa". */
  private readonly mapReady = computed(
    () => !!this.data.map && this.data.map.columns > 0 && !this.noGridMap(),
  );
  protected readonly why = THEATRE_WHY;
  protected readonly whyMore = THEATRE_WHY_DIALOG;
  protected readonly state = signal<'loading' | 'ready' | 'error'>('loading');
  protected readonly saved = this.data.saved ?? null;
  // Without a map to name it after, the combat starts as "Combate": the master renames it.
  protected readonly name = signal(this.saved?.pointName ?? this.data.map?.name ?? 'Combate');
  /** The monsters' hit points and whether they start hidden: the encounter's choice, the master's to change here. */
  protected readonly monsterHp = signal<MonsterHp>(this.saved?.hp ?? 'average');
  protected readonly monstersHidden = signal(this.saved?.hidden ?? true);
  protected readonly hpSegments: readonly Segment<MonsterHp>[] = [
    { value: 'average', label: 'Média' },
    { value: 'rolled', label: 'Rolar' },
  ];
  protected readonly monsterRows = (this.saved?.groups ?? []).map((g) => ({
    ...g,
    becomes: becomesText(g.name || g.namePt, g.count),
  }));
  protected readonly monsterTotal = (this.saved?.groups ?? []).reduce((sum, g) => sum + g.count, 0);
  protected readonly busy = signal(false);
  /** The start is in the air: Esc and the backdrop do not close the dialog under it. */
  protected readonly lockWhileBusy = effect(() => (this.ref.disableClose = this.busy()));
  protected readonly error = signal('');
  /** The server said the map has no grid (it may have been cleared meanwhile). */
  protected readonly noGridMap = signal<string | null>(null);

  protected readonly entries = signal<readonly RosterEntry[]>([]);
  private readonly rolls = signal<ReadonlyMap<string, string>>(new Map());
  protected readonly included = signal<ReadonlySet<string>>(new Set());
  protected readonly counts = signal<ReadonlyMap<string, number>>(new Map());
  protected readonly hiddenAt = signal<ReadonlyMap<string, boolean>>(new Map());

  protected readonly players = computed(() =>
    this.entries().filter((e) => e.kind === CharacterKind.PLAYER),
  );
  protected readonly npcs = computed(() =>
    this.entries().filter((e) => e.kind !== CharacterKind.PLAYER),
  );
  protected readonly npcTotal = computed(() =>
    this.npcs().reduce((sum, n) => sum + (this.counts().get(n.id) ?? 0), 0),
  );
  protected readonly npcKinds = computed(
    () => this.npcs().filter((n) => (this.counts().get(n.id) ?? 0) > 0).length,
  );
  protected readonly total = computed(
    () =>
      (this.adding ? 0 : this.included().size) +
      this.npcTotal() +
      this.monsterTotal +
      (this.data.existing ?? 0),
  );
  protected readonly hasGrid = computed(() => this.adding || (this.data.map?.columns ?? 0) > 0);
  protected readonly gridMapId = computed(() => this.noGridMap() ?? this.data.map?.id ?? null);
  protected readonly size = computed(() => {
    const map = this.data.map;
    return map && map.columns > 0
      ? {
          squares: `${map.columns} × ${map.rows} quadrados de 1,5 m`,
          meters: `${formatMeters(squaresToMeters(map.columns))} × ${formatMeters(squaresToMeters(map.rows))}`,
        }
      : null;
  });
  protected readonly nameOk = computed(
    () => this.name().trim().length >= 1 && this.name().trim().length <= 80,
  );
  /** Why the main button waits, or `null` when it can go. */
  protected readonly blocked = computed(() => {
    if (this.state() !== 'ready') {
      return 'Carregando quem pode lutar.';
    }
    if (!this.adding && !this.theatre() && !this.data.map) {
      return 'Escolha o mapa atual da sessão primeiro.';
    }
    if (!this.theatre() && (!this.hasGrid() || this.noGridMap())) {
      return 'O mapa precisa de uma grade.';
    }
    if (!this.adding && !this.nameOk()) {
      return 'Dê um nome ao combate.';
    }
    if (!this.adding && this.included().size === 0 && this.monsterTotal === 0) {
      return 'Escolha quem do grupo entra.';
    }
    if (this.adding && this.npcTotal() === 0) {
      return 'Escolha quantos NPCs entram.';
    }
    if (this.total() > MAX_COMBATANTS) {
      return `Um combate tem no máximo ${MAX_COMBATANTS} combatentes.`;
    }
    return null;
  });

  constructor() {
    void this.load();
  }

  /** The table's rule is only the default the dialog offers (RN-24): read apart, so a failure leaves the dialog as it is. The dialog
   * waits for it before it is ready, so the radio never flips under the master's hand. */
  private async readRule(): Promise<void> {
    // From a battle point on a map with a grid the combat is a map combat: the table's rule does not flip it (the master can still
    // choose "Sem mapa").
    if (this.adding || !this.mapReady() || this.saved) {
      return;
    }
    try {
      const rules = await this.tableRules.get(this.data.campaignId);
      if (!this.modeTouched && !rules.saved.combatStartsWithMap) {
        this.playMode.set(EncounterMode.THEATRE);
      }
    } catch {
      // Without the rule the combat starts the way it always did: with a map.
    }
  }

  protected chooseMode(mode: EncounterMode): void {
    this.modeTouched = true;
    this.playMode.set(mode);
    this.error.set('');
  }

  private async load(): Promise<void> {
    try {
      const [entries, members, campaign] = await Promise.all([
        this.roster.list(this.data.campaignId),
        this.campaigns.listMembers(this.data.campaignId),
        this.campaigns.getCampaign(this.data.campaignId),
        this.readRule(),
      ]);
      const mode = campaign.campaign?.diceMode ?? DiceMode.PLAYERS_CHOOSE;
      const byUser = new Map(
        members.members
          .filter((m) => m.role === Role.PLAYER)
          .map((m) => [m.userId, m.dicePreference] as const),
      );
      this.rolls.set(
        new Map(
          entries.map((e) => [
            e.id,
            preferenceLabel(
              effectivePreference(mode, byUser.get(e.playerUserId) ?? DicePreference.APP),
            ),
          ]),
        ),
      );
      this.entries.set(entries);
      this.included.set(
        new Set(entries.filter((e) => e.kind === CharacterKind.PLAYER).map((e) => e.id)),
      );
      this.state.set('ready');
    } catch {
      this.state.set('error');
    }
  }

  protected roll(entry: RosterEntry): string {
    return this.rolls().get(entry.id) ?? '';
  }

  protected rollsInApp(entry: RosterEntry): boolean {
    return this.roll(entry) === 'No app';
  }

  protected playerSub(e: RosterEntry): string {
    return [e.raceName, e.classSummary, e.playerName ? `de ${e.playerName}` : null]
      .filter(Boolean)
      .join(' · ');
  }

  protected npcSub(n: RosterEntry): string {
    const count = this.counts().get(n.id) ?? 0;
    const kind = npcKindLabel(n.kind);
    return count > 1 ? `${kind} · vira ${this.copies(n.name, count)}` : kind;
  }

  /** "Goblin 1, 2 e 3". */
  private copies(name: string, count: number): string {
    const numbers = Array.from({ length: count }, (_, i) => i + 1);
    return `${name} ${numbers.length > 1 ? numbers.slice(0, -1).join(', ') + ' e ' + numbers[numbers.length - 1] : numbers[0]}`;
  }

  protected initial(label: string): string {
    return combatantInitial(label);
  }

  protected toggle(id: string): void {
    this.included.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected setCount(id: string, count: number): void {
    this.counts.update((m) => new Map(m).set(id, count));
  }

  protected setHidden(id: string, hidden: boolean): void {
    this.hiddenAt.update((m) => new Map(m).set(id, hidden));
  }

  protected close(): void {
    this.ref.close();
  }

  protected async submit(): Promise<void> {
    if (this.blocked() || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    const specs: JoinSpec[] = [
      ...(this.adding
        ? []
        : this.players()
            .filter((p) => this.included().has(p.id))
            .map((p) => ({ characterId: p.id }))),
      ...this.npcs()
        .filter((n) => (this.counts().get(n.id) ?? 0) > 0)
        .map((n) => ({
          characterId: n.id,
          count: this.counts().get(n.id),
          hidden: this.hiddenAt().get(n.id) ?? true,
        })),
    ];
    try {
      const encounter = this.adding
        ? await this.combat.add(
            this.data.campaignId,
            this.data.encounterId ?? '',
            specs,
            this.addKey.keyFor([this.data.encounterId, specs]),
          )
        : await this.combat.start(
            this.data.campaignId,
            this.name().trim(),
            specs,
            this.key,
            this.saved
              ? {
                  mode: this.playMode(),
                  monsters: this.saved.groups.map((g) => ({
                    creatureKey: g.key,
                    count: g.count,
                    name: g.name,
                  })),
                  monsterHp: this.monsterHp(),
                  monstersHidden: this.monstersHidden(),
                  mapPointId: this.saved.pointId,
                }
              : { mode: this.playMode() },
          );
      this.ref.close(encounter);
    } catch (err) {
      this.busy.set(false);
      const blocked = encounterBlocked(err);
      if (blocked?.reason === EncounterBlockedReason.MAP_HAS_NO_GRID) {
        this.noGridMap.set(blocked.mapId || this.data.map?.id || null);
      }
      this.error.set(
        combatErrorMessage(err, this.adding ? 'adicionar os combatentes' : 'iniciar o combate'),
      );
    }
  }
}
