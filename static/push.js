/* Synergia Pocket: opt-in iPhone Home Screen Web Push (no fake notifications). */
(() => {
  'use strict';
  const ROOT = '/';
  let config = null;
  let pending = null;
  let busy = false;
  let refreshVersion = 0;
  const supported = () => 'serviceWorker' in navigator && 'PushManager' in window &&
      'Notification' in window && window.isSecureContext;
  const endpoint = subscription => ({endpoint:subscription.endpoint,keys:subscription.toJSON().keys});
  const req = async (path, body) => {
    const response = await fetch(path, {method:'POST',credentials:'same-origin',cache:'no-store',
      headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    const payload = await response.json().catch(()=>({}));
    if(!response.ok) throw new Error(typeof payload.detail==='string'?payload.detail:'Nie udało się wykonać żądania.');
    return payload;
  };
  const label = s => {
    const el = document.querySelector('#push-status');
    if(el) el.textContent=s;
  };
  async function ensureConfig() {
    if(!config) {
      const response=await fetch('/api/push/config',{credentials:'same-origin',cache:'no-store'});
      if(!response.ok)throw new Error('Serwer powiadomień jest niedostępny.');
      config=await response.json();
    }
    if(!config.available || !config.public_key)throw new Error('Powiadomienia nie są jeszcze skonfigurowane na serwerze.');
    return config;
  }
  function keyBytes(base64url) {
    const base64=base64url.replace(/-/g,'+').replace(/_/g,'/');
    const raw=atob(base64.padEnd(Math.ceil(base64.length/4)*4,'='));
    return Uint8Array.from(raw, c=>c.charCodeAt(0));
  }
  async function registration() {
    await navigator.serviceWorker.register('/sw.js',{scope:ROOT});
    return navigator.serviceWorker.ready;
  }
  async function currentSubscription() {
    const reg=await registration();
    return reg.pushManager.getSubscription();
  }
  async function refreshUi() {
    const version=++refreshVersion;
    const on=document.getElementById('push-enable');
    const off=document.getElementById('push-disable');
    const test=document.getElementById('push-test');
    if(!on)return;
    if(!supported()) {
      on.disabled=true; if(off)off.hidden=true;if(test)test.hidden=true;
      label('Na iPhonie wymagany iOS 16.4+ i aplikacja dodana do ekranu głównego.');
      return;
    }
    if(Notification.permission==='denied') {
      on.disabled=true;on.hidden=false;if(off)off.hidden=true;if(test)test.hidden=true;
      label('Powiadomienia zablokowane w ustawieniach urządzenia.');return;
    }
    try {
      await ensureConfig();
      const existing=Notification.permission==='granted'?await currentSubscription():null;
      if(version!==refreshVersion)return;
      on.disabled=false;on.hidden=false;if(off)off.hidden=!existing;if(test)test.hidden=true;
      if(!existing){label('Powiadomienia wyłączone.');return;}
      const status=await req('/api/push/status',{endpoint:existing.endpoint});
      if(version!==refreshVersion)return;
      on.hidden=Boolean(status.active);if(test)test.hidden=!status.active;
      if(!status.active){label('Brak aktywnego powiązania z serwerem. Włącz powiadomienia ponownie.');return;}
      let text='Serwer potwierdza powiadomienia o nowych nieprzeczytanych wiadomościach: '+(status.profiles||[]).join(', ')+'.';
      text+=status.last_checked?' Ostatnie sprawdzenie: '+new Date(status.last_checked*1000).toLocaleString('pl-PL')+'.':' Oczekiwanie na pierwsze sprawdzenie.';
      label(text+' Darmowy serwer może zasypiać i opóźniać powiadomienia.');
    } catch(error) {
      if(version!==refreshVersion)return;
      on.hidden=false;on.disabled=false;if(test)test.hidden=true;
      label(error.message||'Nie można potwierdzić powiadomień na serwerze.');
    }
  }
  async function enable(permissionResult) {
    if(busy)return;
    busy=true;
    try {
      if(permissionResult!=='granted')throw new Error('Brak zgody na powiadomienia. Włącz je w ustawieniach urządzenia.');
      const c=await ensureConfig();
      const reg=await registration();
      let subscription=await reg.pushManager.getSubscription();
      if(!subscription)subscription=await reg.pushManager.subscribe({
        userVisibleOnly:true,applicationServerKey:keyBytes(c.public_key)
      });
      const result=await req('/api/push/subscribe',endpoint(subscription));
      label('Włączono powiadomienia: '+(result.profiles||[]).join(', '));
      await refreshUi();
    }catch(error){++refreshVersion;label(error.message||'Nie udało się włączyć powiadomień.');}
    finally {busy=false;}
  }
  async function disable() {
    if(busy)return;
    busy=true;
    try {
      const subscription=await currentSubscription();
      if(subscription) {
        await req('/api/push/unsubscribe',{endpoint:subscription.endpoint});
        await subscription.unsubscribe();
      }
      label('Powiadomienia wyłączone.');
      await refreshUi();
    }catch(e){++refreshVersion;label(e.message||'Nie udało się wyłączyć powiadomień.');}
    finally{busy=false;}
  }
  async function test() {
    if(busy)return;
    busy=true;
    try {
      const subscription=await currentSubscription();
      if(!subscription)throw new Error('Najpierw włącz powiadomienia.');
      await req('/api/push/subscribe',endpoint(subscription));
      await req('/api/push/test',{endpoint:subscription.endpoint});
      ++refreshVersion;
      label('Usługa Push przyjęła test. Sprawdź centrum powiadomień telefonu; przyjęcie nie potwierdza doręczenia.');
    }catch(e){label(e.message||'Błąd testu.');}
    finally{busy=false;}
  }
  async function renewExisting() {
    if(!supported()||Notification.permission!=='granted')return;
    const subscription=await currentSubscription();
    if(subscription && window.SynergiaPushApp?.getUser()?.mode==='live') {
      await req('/api/push/subscribe',endpoint(subscription));
    }
  }
  function readLink() {
    const hash=location.hash.slice(1);
    if(!hash.startsWith('push-profile='))return;
    const args=new URLSearchParams(hash);
    const profile=args.get('push-profile'),message=args.get('push-message');
    if(profile&&message&&profile.length<=80&&message.length<=160)pending={profile,message};
    history.replaceState(null,'',location.pathname+location.search);
  }
  async function openLink() {
    const app=window.SynergiaPushApp;
    if(!pending||!app?.getUser())return;
    if(app.getUser().mode==='demo')return;
    const link=pending;pending=null;
    try {
      await app.openMessageInProfile(link.profile,link.message);
    }catch(e){
      label('Nie udało się otworzyć powiadomienia. Otwórz Wiadomości ręcznie.');
    }
  }
  function afterLogin() {
    renewExisting().then(refreshUi).catch(e=>label(e.message||'Nie udało się odnowić powiadomień.'));
    openLink().catch(()=>{});
  }
  document.addEventListener('click',event=>{
    const enableButton=event.target.closest('#push-enable');
    if(enableButton) {
      if(!supported()){label('Powiadomienia nie są obsługiwane na tym urządzeniu.');return;}
      // Request permission synchronously from the user's tap — required by iOS.
      const permission=Notification.requestPermission();
      Promise.resolve(permission).then(enable).catch(e=>label(e.message||'Brak zgody.'));
    }
    if(event.target.closest('#push-disable'))disable();
    if(event.target.closest('#push-test'))test();
  });
  document.addEventListener('DOMContentLoaded',()=>{
    readLink();
    if(supported())registration().catch(()=>{});
  });
  document.addEventListener('synergia:authenticated',afterLogin);
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)afterLogin();});
  // The notifications panel is rendered dynamically when the mailbox opens.
  const observe=document.getElementById('content');
  if(observe){
    const mo=new MutationObserver(()=>{if(document.getElementById('push-enable'))refreshUi().catch(()=>{});});
    mo.observe(observe,{childList:true});
  }
  navigator.serviceWorker?.addEventListener?.('message', event=>{
    if(event.data?.profile&&event.data?.message){
      pending={profile:String(event.data.profile),message:String(event.data.message)};
      openLink().catch(()=>{});
    }
  });
})();
