/**
 * The address of the TV's tab: `/campaigns/<id>/table-screen` on this site. It carries no token and no secret: the tab
 * signs in as the master, and the server decides on every call.
 */
export function tableScreenLink(campaignId: string, origin: string = location.origin): string {
  return `${origin}/campaigns/${campaignId}/table-screen`;
}
