import { Component, computed, effect, ElementRef, inject, signal, viewChild } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type Encounter,
  EncounterMode,
  EncounterStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CreatureSummary } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient, type MonsterHp } from '../../../core/combat/combat-client';
import { sessionClosed } from '../../../core/combat/combat-errors';
import {
  ADD_MAX,
  AddKeys,
  MONSTER_NAME_MAX,
  addMonstersErrorMessage,
  monsterSentence,
  roomFor,
} from '../../../core/combat/monsters';
import { capitalized, listWithE } from '../../../core/creatures/bestiary-format';
import { nameCounter } from '../../../core/creatures/creature-format';
import { SheetFrame } from '../../sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../sheet/sheet-host';
import { type Segment, Segmented } from '../../segmented/segmented';
import { tieNumbers } from '../../../core/format/text';
import { OpenSessionLookup } from '../../../core/play/open-session';
import { MapsClient } from '../../../core/maps/maps-client';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { THEATRE_WHY, THEATRE_WHY_DIALOG } from '../../../core/combat/theatre';
import { DiceChoice } from '../../dice-choice/dice-choice';
import { COMBAT_MODE_OPTIONS } from '../../start-combat/start-combat-dialog';
import { CountStepper } from '../../count-stepper/count-stepper';
import { CreatureArt } from '../../creatures/creature-art';
import { HiddenSwitch } from '../../hidden-switch/hidden-switch';

/** What the bestiary hands the sheet. */
export interface PutMonstersData {
  readonly campaignId: string;
  readonly creature: CreatureSummary;
}

/** What closes the sheet: what was put in, for the page's confirmation. */
export interface PutMonstersResult {
  readonly count: number;
  /** "Bandido 1, Bandido 2 e Bandido 3". */
  readonly names: string;
  readonly combatName: string;
  /** The sheet had to make the combat first ("Criar o combate e pôr"). */
  readonly started: boolean;
  readonly hidden: boolean;
  /** The server found the add already done (a retry after a lost answer): it did it once. */
  readonly encounterId: string;
}

/** The open combat the monsters go into: one in preparation or under way. */
interface Target {
  readonly id: string;
  readonly name: string;
  readonly running: boolean;
  /** How many combatants it has: the combat takes 40 in all. */
  readonly existing: number;
}

const HP_SEGMENTS = (average: number): readonly Segment<MonsterHp>[] => [
  { value: 'average', label: `Média (${average})` },
  { value: 'rolled', label: 'Rolar' },
];

/**
 * "Pôr no combate" (MR-042, RN-29, E10-08 states 4 and 7): puts 1 to 10 monsters of one SRD creature in the session's
 * combat. The master picks how many (the preview says the names they take), the base name (the creature's Portuguese
 * one to start), whether the hit points are the average or rolled, and whether they start hidden (the default: the
 * players know nothing of them until he reveals them, RN-20). With no combat open, the button says "Criar o combate e
 * pôr" and starts one with the monsters (`StartEncounter`).
 *
 * The idempotency key is made once when the sheet opens and repeats on a retry with the same parameters (`AddKeys`):
 * a second tap or a retry after a lost answer adds once; another choice since the last try is a new add and a new key.
 * The 40-combatant cap has no detail of its own on the server, so the count stops at what fits and the sheet says why.
 *
 * It is a bottom sheet on a phone and a dialog from a tablet up (`openSheet`), with the footer fixed.
 */
@Component({
  selector: 'app-put-monsters-sheet',
  imports: [
    CountStepper,
    CreatureArt,
    DiceChoice,
    HiddenSwitch,
    MatButtonModule,
    MatIconModule,
    Segmented,
    SheetFrame,
  ],
  templateUrl: './put-sheet.html',
  styleUrl: './put-sheet.scss',
})
export class PutMonstersSheet {
  private readonly combat = inject(CombatClient);
  private readonly sheet = injectSheet<PutMonstersData, PutMonstersResult>();
  private readonly frame = viewChild.required(SheetFrame);
  private readonly lookup = inject(OpenSessionLookup);
  private readonly maps = inject(MapsClient);
  private readonly tableRules = inject(TableRulesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly max = MONSTER_NAME_MAX;
  protected readonly nameCounter = nameCounter;
  protected readonly hpSegments = HP_SEGMENTS(this.data.creature.hitPoints);

  /** How a combat made here is played (RN-25): the same choice and defaults as "Iniciar combate". */
  protected readonly playMode = signal<EncounterMode>(EncounterMode.THEATRE);
  protected readonly modeOptions = COMBAT_MODE_OPTIONS;
  protected readonly why = THEATRE_WHY;
  protected readonly whyMore = THEATRE_WHY_DIALOG;
  /** The session's current map has a grid: only then "Com mapa" is possible. */
  protected readonly mapReady = signal(false);
  private modeTouched = false;
  protected readonly theatre = computed(() => this.playMode() === EncounterMode.THEATRE);
  protected readonly modeInert = computed<readonly EncounterMode[]>(() =>
    this.mapReady() ? [] : [EncounterMode.GRID],
  );
  protected readonly modeNotes = computed<Partial<Record<number, string>>>(() =>
    this.mapReady()
      ? {}
      : { [EncounterMode.GRID]: 'O mapa atual não tem grade, ou a sessão não tem um mapa atual.' },
  );
  protected readonly loading = signal(true);
  protected readonly target = signal<Target | null>(null);
  /** Why there is nothing to add to or start: no open session, or a read that failed. */
  protected readonly noSession = signal(false);
  protected readonly count = signal(1);
  protected readonly name = signal(this.data.creature.namePt);
  protected readonly hp = signal<MonsterHp>('average');
  protected readonly hidden = signal(true);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly nameError = signal('');

  private readonly keys = new AddKeys();

  protected readonly c = this.data.creature;
  protected readonly typeLabel = capitalized(this.data.creature.typePt);
  protected readonly meta = computed(() =>
    tieNumbers(
      `ND ${this.c.challengeRating} · CA ${this.c.armorClass} · PV ${this.c.hitPoints}${this.hp() === 'average' ? ' (média)' : ''}`,
    ),
  );
  /** How many more the combat takes (the cap is 40 in all); a combat still to make takes one add's 10. */
  protected readonly room = computed(() =>
    this.target() ? roomFor(this.target()!.existing) : ADD_MAX,
  );
  protected readonly full = computed(() => this.target() !== null && this.room() === 0);
  protected readonly sentence = computed(() =>
    monsterSentence(this.name() || this.c.namePt, this.count()),
  );
  protected readonly starting = computed(
    () => !this.loading() && this.target() === null && !this.noSession(),
  );
  protected readonly goLabel = computed(() => {
    if (this.busy()) {
      return this.starting() ? 'Criando...' : 'Pondo...';
    }
    if (this.starting()) {
      return 'Criar o combate e pôr';
    }
    return this.count() > 1 ? `Pôr ${this.count()} no combate` : 'Pôr no combate';
  });
  /** Why the main button waits, or `''`. */
  protected readonly blocked = computed(() => {
    if (this.loading()) {
      return 'Lendo o combate da sessão.';
    }
    if (this.noSession()) {
      return 'Abra a sessão primeiro.';
    }
    if (this.full()) {
      return 'O combate já tem 40 combatentes.';
    }
    return '';
  });

  constructor() {
    void this.load();
  }

  private async load(): Promise<void> {
    try {
      const encounter = await this.combat.get(this.data.campaignId);
      this.target.set(encounter ? targetOf(encounter) : null);
      if (!this.target()) {
        await this.readMode();
      }
    } catch (err) {
      if (sessionClosed(err)) {
        this.noSession.set(true);
      } else {
        this.error.set(
          'Não deu para ler o combate da sessão: o servidor não respondeu. Feche e abra a folha de novo.',
        );
        this.noSession.set(true);
      }
    } finally {
      this.loading.set(false);
      this.clampCount();
    }
  }

  /** The defaults of "Iniciar combate": "Com mapa" when the current map has a grid and the table's rule says so, else the theatre. */
  private async readMode(): Promise<void> {
    try {
      const open = await this.lookup.currentMap(this.data.campaignId);
      if (!open || open.mapId === '') {
        return;
      }
      const map = (await this.maps.get(this.data.campaignId, open.mapId)).map;
      if (!map || map.gridColumns <= 0) {
        return;
      }
      this.mapReady.set(true);
      let rule = true;
      try {
        rule = (await this.tableRules.get(this.data.campaignId)).saved.combatStartsWithMap;
      } catch {
        // Without the rule the combat starts the way it always did: with a map.
      }
      if (!this.modeTouched) {
        this.playMode.set(rule ? EncounterMode.GRID : EncounterMode.THEATRE);
      }
    } catch {
      // Unreadable: the theatre of the mind stays, and the master can still start it.
    }
  }

  protected chooseMode(mode: EncounterMode): void {
    this.modeTouched = true;
    this.playMode.set(mode);
  }

  private clampCount(): void {
    if (this.count() > Math.max(1, this.room())) {
      this.count.set(Math.max(1, this.room()));
    }
  }

  protected setName(value: string): void {
    this.name.set(value);
    this.nameError.set('');
  }

  protected async put(): Promise<void> {
    if (this.busy() || this.blocked() !== '') {
      return;
    }
    const base = this.name().trim() || this.c.namePt;
    if (base.length > MONSTER_NAME_MAX || base.includes('\n')) {
      this.nameError.set(`O nome pede de 1 a ${MONSTER_NAME_MAX} letras, numa linha só.`);
      this.host.nativeElement.querySelector<HTMLInputElement>('input[name=name]')?.focus();
      return;
    }
    const target = this.target();
    const add = {
      creatureKey: this.c.key,
      count: this.count(),
      name: base,
      hp: this.hp(),
      hidden: this.hidden(),
    };
    const key = this.keys.keyFor({
      ...add,
      target: target?.id ?? '',
      mode: target ? undefined : this.playMode(),
    });
    this.busy.set(true);
    this.error.set('');
    try {
      let encounter: Encounter;
      let ids: readonly string[] = [];
      if (target) {
        const made = await this.combat.addMonsters(this.data.campaignId, target.id, add, key);
        encounter = made.encounter;
        ids = made.combatantIds;
      } else {
        encounter = await this.combat.start(
          this.data.campaignId,
          `Combate: ${this.c.namePt}`,
          [],
          key,
          {
            mode: this.playMode(),
            monsters: [{ creatureKey: add.creatureKey, count: add.count, name: add.name }],
            monsterHp: add.hp,
            monstersHidden: add.hidden,
          },
        );
      }
      this.sheet.close({
        count: add.count,
        // What the server made: with Bandidos already in, it numbers on ("Bandido 4, Bandido 5 e Bandido 6"); the client's preview is the fallback.
        names: this.namesMade(encounter, ids) || monsterSentence(base, add.count),
        combatName: encounter.name,
        started: target === null,
        hidden: add.hidden,
        encounterId: encounter.id,
      });
    } catch (err) {
      this.error.set(addMonstersErrorMessage(err));
      this.frame().scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }

  /** The labels of the monsters this add made, as the combat names them, in their numbers' order ("Bandido 1, Bandido 2 e
   * Bandido 3"): the combat lists its combatants by initiative, never the order to say them in. */
  private namesMade(encounter: Encounter, ids: readonly string[]): string {
    const labels =
      ids.length > 0
        ? ids
            .map((id) => encounter.combatants.find((c) => c.id === id)?.label ?? '')
            .filter((l) => l !== '')
        : encounter.combatants
            .filter((c) => c.bestiaryCreatureKey === this.c.key && !c.defeated)
            .map((c) => c.label);
    return listWithE([...labels].sort((a, b) => a.localeCompare(b, 'pt-BR', { numeric: true })));
  }

  protected close(): void {
    this.sheet.close();
  }
}

function targetOf(e: Encounter): Target | null {
  if (e.status !== EncounterStatus.SETUP && e.status !== EncounterStatus.ACTIVE) {
    return null;
  }
  return {
    id: e.id,
    name: e.name,
    running: e.status === EncounterStatus.ACTIVE,
    existing: e.combatants.length,
  };
}
