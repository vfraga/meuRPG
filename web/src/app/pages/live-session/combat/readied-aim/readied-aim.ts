import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  model,
  type OnInit,
  output,
  signal,
} from '@angular/core';

import type {
  Encounter,
  GetTurnOptionsResponse,
  ReadiedAction,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { dealOne } from '../../../../core/combat/cast-flow';
import { CombatClient } from '../../../../core/combat/combat-client';
import { isPlayer } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { type AimNeed, type ReadiedAim, readiedAim } from '../../../../core/combat/readied-aim';
import {
  NO_PICK,
  type ReleasePick,
  releaseStart,
  releaseStatus,
  releaseToggled,
} from '../../../../core/combat/readied-release';
import { CastTargets } from '../cast-sheet/cast-targets';
import { ResourcePick } from '../resource-pick/resource-pick';

/**
 * Who a readied attack or spell is aimed at (#582, #857): the list of creatures the readier's own turn options give, with the one named
 * at Ready chosen when it can be aimed at now. A held spell the server lists targets for is released as the cast screens cast it: the
 * darts split with steppers, the creatures ticked. The same on the readier's prompt and in the master's queue. `need` tells the parent
 * what the list asks (`required` for an attack, `optional` for a spell, `failed` when it could not be read); `target` is the choice of an
 * attack and `release` the pick of a spell, whose `aim` the parent reads to build the answer.
 */
@Component({
  selector: 'app-readied-aim',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ResourcePick, CastTargets],
  styleUrls: ['../resource-pick/resource-sheet.scss'],
  template: `
    @if (need() === 'failed') {
      <p class="small" role="status">Não deu para ler quem está ao alcance. {{ failedText() }}</p>
    } @else if (need() === 'loading') {
      <p class="small" role="status">Lendo quem está ao alcance…</p>
    } @else if (aim().plan; as plan) {
      @if (plan.rule.kind !== 'none' && !plan.onMap) {
        <app-cast-targets
          [rows]="plan.rows"
          [rule]="plan.rule"
          [chosen]="release().chosen"
          [dealt]="release().dealt"
          [total]="plan.total"
          [npcs]="npcs()"
          [creatures]="creatures()"
          (toggle)="touch(); release.set(toggled(release(), $event))"
          (deal)="touch(); release.set(dealt(release(), $event.id, $event.delta))"
        />
        @if (status()) {
          <p class="small" role="status">{{ status() }}</p>
        }
      }
    } @else if (aim().rows.length > 0 || need() === 'required') {
      <app-resource-pick
        [label]="aim().legend"
        [rows]="aim().rows"
        [chosen]="target()"
        empty="Não há ninguém para escolher."
        (pick)="touch(); target.set($event)"
      />
    }
  `,
})
export class ReadiedAimList implements OnInit {
  readonly campaignId = input.required<string>();
  readonly encounterId = input.required<string>();
  readonly readied = input.required<ReadiedAction>();
  readonly encounter = input.required<Encounter | null>();
  readonly reactorId = input.required<string>();
  /** What the sentence after "Não deu para ler quem está ao alcance." says the one answering can still do. */
  readonly failedText = input('Dá para deixar passar.');
  readonly need = model<AimNeed>('loading');
  readonly target = model<string | null>(null);
  /** What the list of a held spell holds: the creatures ticked and the darts dealt. */
  readonly release = model<ReleasePick>(NO_PICK);
  /** What the options say of the held spell, once read: the parent builds the answer with it. */
  readonly plan = model<ReadiedAim['plan']>(null);
  /** The readier's own turn options as they were read: the roll step of a released attack or spell reads its bonus, its dice and the mode the server suggests from them (#938). */
  readonly options = model<GetTurnOptionsResponse | null>(null);
  /** The first time the one answering touches the list (a creature, a dart): the parent tells the table they are choosing. Once. */
  readonly touched = output<void>();
  private told = false;

  private readonly api = inject(CombatClient);
  private readonly result = signal<GetTurnOptionsResponse | null>(null);
  protected readonly aim = computed<ReadiedAim>(() => {
    const res = this.result();
    return res
      ? readiedAim(res, this.readied(), this.encounter(), this.reactorId())
      : { need: 'loading', rows: [], legend: '', chosen: null, plan: null };
  });
  protected readonly status = computed(() => {
    const plan = this.aim().plan;
    return plan ? releaseStatus(plan, this.release()) : '';
  });
  protected readonly npcs = computed(
    () =>
      new Set((this.encounter()?.combatants ?? []).filter((c) => !isPlayer(c)).map((c) => c.id)),
  );
  protected readonly creatures = computed(
    () => new Set((this.encounter()?.combatants ?? []).filter(isCreature).map((c) => c.id)),
  );
  protected touch(): void {
    if (!this.told) {
      this.told = true;
      this.touched.emit();
    }
  }
  protected toggled(pick: ReleasePick, id: string): ReleasePick {
    const plan = this.aim().plan;
    return plan ? releaseToggled(plan, pick, id) : pick;
  }
  protected readonly dealt = (pick: ReleasePick, id: string, delta: 1 | -1): ReleasePick => {
    const total = this.aim().plan?.total ?? 0;
    return { ...pick, dealt: dealOne(pick.dealt, id, delta, total) };
  };

  ngOnInit(): void {
    void this.read();
  }

  private async read(): Promise<void> {
    try {
      const res = await this.api.moves.turnOptions(
        this.campaignId(),
        this.encounterId(),
        this.reactorId(),
      );
      this.result.set(res);
      this.options.set(res);
      const aim = this.aim();
      this.target.set(aim.chosen);
      if (aim.plan) {
        this.release.set(releaseStart(aim.plan, this.readied().targetCombatantId));
      }
      this.plan.set(aim.plan);
      this.need.set(aim.need);
    } catch {
      this.need.set('failed');
    }
  }
}
