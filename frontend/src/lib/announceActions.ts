/**
 * Announcement actions shared by the Announcements view and the command
 * palette. Lives outside stores.ts so both entry points call the same code.
 */

import { api, errMsg } from './api';
import { announcements, toast } from './stores';

/** Mark every announcement read, backend first, then the local list. */
export async function markAllAnnouncementsRead(): Promise<void> {
  try {
    await api.markAllAnnouncementsRead();
    // `unreadAnnouncements` is derived from this list, so it falls to 0 here.
    announcements.update((list) => list.map((a) => (a.Read ? a : { ...a, Read: true })));
  } catch (err) {
    toast(`Could not mark all as read: ${errMsg(err)}`, 'error');
  }
}
