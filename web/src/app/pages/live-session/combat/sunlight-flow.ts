import type { Encounter, SunlightScope } from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatCalls } from '../../../core/combat/combat-calls';
import type { CombatClient } from '../../../core/combat/combat-client';
import { PaintQueue, type SunChange } from '../../../core/combat/sunlight-paint';

/** How long a stroke waits for a call in the air (20 x 100 ms), before it is tried anyway. */
const WAITS = 20;
const WAIT_MS = 100;

/** What `SunlightFlow` reads from the screen it serves. */
export interface SunlightFlowContext {
  readonly api: { readonly combatants: Pick<CombatClient['combatants'], 'setSunlight'> };
  readonly calls: Pick<CombatCalls, 'busy' | 'run'>;
  readonly campaignId: () => string;
}

/** What the map card asks: another scope, or squares to add and remove. */
export interface SunlightAsk extends Partial<SunChange> {
  readonly scope?: SunlightScope;
}

/**
 * The master's sun on the map (#704): the scope the map card
 * asks for and the strokes, sent one call at a time and merged (`PaintQueue`). A call waits for the one in the air, so a stroke is
 * not lost to a busy screen; the screen shows the server's squares when each answer comes.
 */
export class SunlightFlow {
  private readonly queue = new PaintQueue((c) => this.send(c));

  constructor(private readonly ctx: SunlightFlowContext) {}

  ask(change: SunlightAsk): void {
    if (change.scope !== undefined) {
      void this.send({ scope: change.scope });
      return;
    }
    void this.queue.push({
      addSquares: change.addSquares ?? [],
      removeSquares: change.removeSquares ?? [],
    });
  }

  private async send(change: Partial<SunChange> & { scope?: SunlightScope }): Promise<boolean> {
    for (let n = 0; this.ctx.calls.busy() && n < WAITS; n++) {
      await new Promise((resolve) => setTimeout(resolve, WAIT_MS));
    }
    return this.ctx.calls.run((e: Encounter) =>
      this.ctx.api.combatants.setSunlight(this.ctx.campaignId(), e.id, {
        on: e.sunlightOn,
        ...change,
      }),
    );
  }
}
