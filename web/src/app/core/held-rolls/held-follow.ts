import type { HeldRollOffer } from '../../../gen/meurpg/play/v1/combat_pb';
import type { ListHeldRollsResponse } from '../../../gen/meurpg/play/v1/resources_pb';
import type { HeldRollController } from './held-roll-controller';
import { wasAnsweredHere } from './held-roll-controller';
import { HeldVanishWatch } from './held-vanish';

const keyOf = (offer: HeldRollOffer | null | undefined) =>
  offer ? `${offer.holdId}:${offer.question.case}` : '';

/**
 * Keeps the question a sheet asks true to the server: the sheet asked it itself (`held.ask`) when its roll was held, and
 * when the master answers it, the question goes and `vanish.notice` takes its place, the way the attack sheet does. The
 * place feeds `see` with the question as it reads it now (the roller's combatant in the combat, or the list of held rolls
 * out of one); the player's own answer never leaves the note. Plain class a sheet builds.
 */
export class HeldFollow {
  readonly vanish = new HeldVanishWatch();
  private reads = 0;

  constructor(private readonly held: Pick<HeldRollController, 'offer' | 'ask'>) {}

  /** `open`: the question of the hold the sheet shows, as the place reads it now (`null`: none is open). */
  see(open: HeldRollOffer | null | undefined): void {
    const shown = this.held.offer();
    if (!shown) {
      return;
    }
    // The player's own answer never leaves the note.
    if (wasAnsweredHere(shown)) {
      this.vanish.dismiss();
    }
    // Another hold on the roller is not this question: the one the sheet shows is gone.
    const mine = open?.holdId === shown.holdId ? open : null;
    this.vanish.observe(shown);
    this.vanish.observe(mine);
    // Only a question that changed goes on screen: the same one read again must not be asked again (the sheet reads
    // when its question changes, so that would read without end), and a question that comes next is not the note's.
    if (this.vanish.notice() && keyOf(mine) !== keyOf(shown)) {
      this.held.ask(mine);
      if (mine) {
        this.vanish.dismiss();
      }
    }
  }

  /** Out of a combat: reads the held rolls and feeds `see` with the one of the hold the sheet shows. A read that is
   * overtaken by a newer one, or by a question that changed meanwhile, is ignored. */
  async seeList(list: () => Promise<ListHeldRollsResponse>): Promise<void> {
    const shown = this.held.offer();
    if (!shown) {
      return;
    }
    const mine = ++this.reads;
    try {
      const { offers } = await list();
      if (mine === this.reads && this.held.offer() === shown) {
        this.see(offers.find((o) => o.holdId === shown.holdId));
      }
    } catch {
      // A hint that did not come back: the sheet keeps its question and reads again on the next one.
    }
  }
}
