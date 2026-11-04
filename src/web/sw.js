const CACHE='pocket-own-web-016';
const PUBLIC=['/manifest.webmanifest','/icon-192.png','/icon-512.png','/apple-touch-icon.png','/icon.svg','/offline.html'];
self.addEventListener('install',event=>event.waitUntil(caches.open(CACHE).then(c=>c.addAll(PUBLIC)).then(()=>self.skipWaiting())));
self.addEventListener('activate',event=>event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(k=>k!==CACHE).map(k=>caches.delete(k)))).then(()=>self.clients.claim())));
self.addEventListener('fetch',event=>{
 const url=new URL(event.request.url);if(url.origin!==self.location.origin||event.request.method!=='GET')return;
 // Account data, HTML with session bootstrap, and downloads are never cached.
 if(event.request.mode==='navigate'){event.respondWith(fetch(event.request).catch(()=>caches.match('/offline.html')));return;}
 if(PUBLIC.includes(url.pathname)&&!url.search)event.respondWith(caches.match(event.request).then(hit=>hit||fetch(event.request)));
});
