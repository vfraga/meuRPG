import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  model,
  type OnInit,
  signal,
} from '@angular/core';

import type {
  Encounter,
  GetTurnOptionsResponse,
  ReadiedAction,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { type AimNeed, type ReadiedAim, readiedAim } from '../../../../core/combat/readied-aim';
import { ResourcePick } from '../../../../shared/resource-pick/resource-pick';

/**
 * Who a readied attack or spell is aimed at (#582): the list of creatures the readier's own turn options give, with the one named
 * at Ready chosen when it can be aimed at now. The same on the readier's prompt and in the master's queue. `need` tells the parent
 * what the list asks (`required` for an attack, `optional` for a spell, `failed` when it could not be read); `target` is the choice.
 */
@Component({
  selector: 'app-readied-aim',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ResourcePick],
  styleUrls: ['../resource-pick/resource-sheet.scss'],
  template: `
    @if (need() === 'failed') {
      <p class="small" role="status">Não deu para ler quem está ao alcance. {{ failedText() }}</p>
    } @else if (need() === 'loading') {
      <p class="small" role="status">Lendo quem está ao alcance…</p>
    } @else if (aim().rows.length > 0 || need() === 'required') {
      <app-resource-pick
        [label]="aim().legend"
        [rows]="aim().rows"
        [chosen]="target()"
        empty="Não há ninguém para escolher."
        (pick)="target.set($event)"
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

  private readonly api = inject(CombatClient);
  private readonly result = signal<GetTurnOptionsResponse | null>(null);
  protected readonly aim = computed<ReadiedAim>(() => {
    const res = this.result();
    return res
      ? readiedAim(res, this.readied(), this.encounter(), this.reactorId())
      : { need: 'loading', rows: [], legend: '', chosen: null };
  });

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
      const aim = this.aim();
      this.target.set(aim.chosen);
      this.need.set(aim.need);
    } catch {
      this.need.set('failed');
    }
  }
}
