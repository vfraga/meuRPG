/** How near an edge the strip counts as at the edge, in pixels. */
const STRIP_SLACK = 1;

/** Which edges of a scrolling strip have more beyond them (the fade is drawn there). */
export function stripEdges(el: HTMLElement): { start: boolean; end: boolean } {
  return {
    start: el.scrollLeft > STRIP_SLACK,
    end: el.scrollLeft + el.clientWidth < el.scrollWidth - STRIP_SLACK,
  };
}

/**
 * Brings the open kind into the middle of the strip when it scrolls (at 768 px "Armas" is out of sight to the right). A strip that
 * fits, or a column, has nothing to scroll: the page stays where it is.
 */
export function showActiveKind(strip: HTMLElement): void {
  if (strip.scrollWidth <= strip.clientWidth) {
    return;
  }
  strip
    .querySelector('[aria-current="page"]')
    ?.scrollIntoView({ block: 'nearest', inline: 'center' });
}
