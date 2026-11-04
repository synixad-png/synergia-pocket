/* Synergia Pocket Web Push. No caching of school content. */
'use strict';
self.addEventListener('push', event => {
  // Apple requires a visible notification for every delivered push.
  let data = {};
  try { data = event.data ? event.data.json() : {}; } catch (_) {}
  const kind = data.kind === 'message' ? 'message' : 'test';
  const title = 'Synergia Pocket';
  const options = {
    body: kind === 'message' ? String(data.body || 'Nowa wiadomość szkolna.').slice(0,120)
                             : 'Powiadomienia na tym urządzeniu działają.',
    icon: '/static/icon-192.png',
    badge: '/static/icon-192.png',
    tag: String(data.tag || 'synergia-test').slice(0,90),
    data: {
      kind,
      profile: kind === 'message' ? String(data.profile || '').slice(0,80) : '',
      message: kind === 'message' ? String(data.message || '').slice(0,160) : ''
    }
  };
  event.waitUntil(self.registration.showNotification(title, options));
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  const d = event.notification.data || {};
  const hash = d.kind === 'message' && d.profile && d.message
    ? '#push-profile=' + encodeURIComponent(d.profile) + '&push-message=' + encodeURIComponent(d.message)
    : '';
  const url = self.location.origin + '/' + hash;
  event.waitUntil((async () => {
    const windows = await clients.matchAll({type: 'window', includeUncontrolled: true});
    for (const client of windows) {
      if ('navigate' in client) {
        try {
          const updated = await client.navigate(url);
          await (updated || client).focus();
          return;
        } catch (_) { /* open new window below */ }
      }
    }
    await clients.openWindow(url);
  })());
});
