/* School message composer; no passwords or drafts stored on the phone. */
(()=>{
'use strict';
const $=id=>document.getElementById(id);
const ui={mode:'new',profile:null,message:null,contacts:[],picked:'',checked:false,nonce:'',sending:false,refresh:null};
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
async function api(url,init={}){
 const r=await fetch(url,{credentials:'same-origin',cache:'no-store',...init});
 const body=await r.json().catch(()=>({detail:'Nieprawidłowa odpowiedź serwera'}));
 if(!r.ok)throw new Error(body.detail||'Błąd '+r.status);
 return body;
}
function error(t){$('compose-error').textContent=t||'';}
function close(){if(ui.sending)return;$('compose-dialog').classList.add('hidden');$('compose-form').reset();ui.profile=null;ui.checked=false;ui.contacts=[];ui.picked='';$('compose-preview').classList.add('hidden');error('');}
function resetCheck(){ui.checked=false;ui.nonce='';$('compose-preview').classList.add('hidden');$('compose-submit').textContent='Sprawdź przed wysłaniem';}
function description(r){const x=[r.role];if(r.class_name)x.push('Wychowawca klasy '+r.class_name);if(r.subjects&&r.subjects.length)x.push('Przedmioty: '+r.subjects.join(', '));return x.filter(Boolean).join(' · ');}
function draw(){
 const q=$('compose-find').value.toLocaleLowerCase('pl-PL').trim();
 const rows=ui.contacts.filter(x=>(x.name+' '+x.role+' '+(x.subjects||[]).join(' ')+' '+x.class_name).toLocaleLowerCase('pl-PL').includes(q));
 const box=$('compose-contacts');box.replaceChildren();
 if(!rows.length){const p=document.createElement('p');p.textContent='Brak pasujących pracowników szkoły.';box.append(p);return;}
 for(const r of rows){
  const label=document.createElement('label');label.className='contact-choice';
  const radio=document.createElement('input');radio.type='radio';radio.name='recipient';radio.value=r.id;radio.checked=(ui.picked===r.id);
  const info=document.createElement('span');info.className='contact-description';
  const title=document.createElement('strong');title.textContent=r.name;
  const sub=document.createElement('small');sub.textContent=description(r);
  info.append(title,sub);label.append(radio,info);box.append(label);
 }
}
async function compose(profile,refresh){
 if(!profile?.id||ui.sending)return;
 ui.mode='new';ui.profile=profile;ui.message=null;ui.contacts=[];ui.picked='';ui.refresh=refresh;
 $('compose-form').reset();resetCheck();error('');
 $('compose-dialog').classList.remove('hidden');$('compose-title').textContent='Nowa wiadomość';
 $('compose-profile').textContent='Konto: '+profile.name;
 $('compose-recipient-area').classList.remove('hidden');$('compose-subject-area').classList.remove('hidden');$('compose-reply-summary').classList.add('hidden');
 $('compose-submit').disabled=true;$('compose-contacts').textContent='Pobieranie kontaktów…';
 try{
  const d=await api('/api/contacts');
  if(!ui.profile||ui.profile.id!==profile.id||$('compose-dialog').classList.contains('hidden'))return;
  if(d.profile_id!==profile.id)throw new Error('Profil został zmieniony. Otwórz formularz jeszcze raz.');
  ui.contacts=d.items||[];draw();$('compose-submit').disabled=ui.contacts.length===0;
  if(!ui.contacts.length)error('Librus nie udostępnił adresatów dla tego konta.');
 }catch(e){$('compose-submit').disabled=true;error(e.message);}
}
function reply(profile,message,refresh){
 if(!profile?.id||!message?.id||ui.sending)return;
 ui.mode='reply';ui.profile=profile;ui.message=message;ui.contacts=[];ui.refresh=refresh;
 $('compose-form').reset();resetCheck();error('');
 $('compose-dialog').classList.remove('hidden');$('compose-title').textContent='Odpowiedź';
 $('compose-profile').textContent='Konto: '+profile.name;
 $('compose-recipient-area').classList.add('hidden');$('compose-subject-area').classList.add('hidden');
 $('compose-reply-summary').classList.remove('hidden');
 $('compose-reply-summary').textContent='Do: '+(message.sender||'Nadawca')+' · Re: '+(message.subject||'Bez tematu');
 $('compose-submit').disabled=false;$('compose-text').focus();
}
function values(){
 if(!ui.profile)throw Error('Nie wybrano profilu dziecka.');
 const text=$('compose-text').value.trim();
 if(!text)throw Error('Wpisz treść wiadomości.');
 if(ui.mode==='reply')return {profile_id:ui.profile.id,message_id:ui.message.id,text};
 const who=ui.contacts.find(r=>r.id===ui.picked);
 if(!who)throw Error('Wybierz odbiorcę z listy.');
 const subject=$('compose-subject').value.trim();
 if(!subject)throw Error('Wpisz temat wiadomości.');
 return {profile_id:ui.profile.id,recipient_id:who.id,subject,text};
}
function preview(data){
 const box=$('compose-preview');box.replaceChildren();
 for(const [title,val] of [
  ['Konto',ui.profile.name],
  ['Do',ui.mode==='reply'?ui.message.sender:ui.contacts.find(x=>x.id===ui.picked)?.name],
  ['Temat',ui.mode==='reply'?'Re: '+ui.message.subject:data.subject],
  ['Treść',data.text]]){
  const p=document.createElement('p');const strong=document.createElement('strong');strong.textContent=title+': ';
  p.append(strong,document.createTextNode(val||''));box.append(p);
 }
 const note=document.createElement('small');
 note.textContent='Kliknięcie Wyślij spowoduje prawdziwą wysyłkę przez Librus. Nie ponawiaj bez sprawdzenia folderu Wysłane.';
 box.append(note);box.classList.remove('hidden');ui.checked=true;
 const bytes=new Uint8Array(24);crypto.getRandomValues(bytes);
 ui.nonce=Array.from(bytes,x=>x.toString(16).padStart(2,'0')).join('');
 $('compose-submit').textContent='Wyślij wiadomość';
}
async function submit(e){
 e.preventDefault();if(ui.sending)return;error('');
 let data;try{data=values();}catch(err){error(err.message);return;}
 if(!ui.checked){preview(data);return;}
 const button=$('compose-submit');button.disabled=true;button.textContent='Wysyłanie…';ui.sending=true;
 try{
  const path=ui.mode==='reply'?'/api/reply-message':'/api/send-message';
  const result=await api(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...data,nonce:ui.nonce})});
  if(result.profile_id!==ui.profile.id)throw Error('Profil odpowiedzi niezgodny. Sprawdź Wysłane.');
  ui.sending=false;const refresh=ui.refresh;close();
  if(typeof window.statusToast==='function')window.statusToast('Wiadomość wysłana przez Librus.');
  if(typeof refresh==='function')try{await refresh();}catch(_){}
 }catch(err){
  ui.sending=false;resetCheck();error((err.message||'Błąd wysyłki')+' Sprawdź Wysłane przed ponowieniem.');
 }finally{button.disabled=false;}
}
document.addEventListener('DOMContentLoaded',()=>{
 $('compose-form').addEventListener('submit',submit);
 $('compose-close').addEventListener('click',close);
 $('compose-dialog').addEventListener('click',e=>{if(e.target===$('compose-dialog'))close();});
 $('compose-find').addEventListener('input',draw);
 $('compose-contacts').addEventListener('change',e=>{if(e.target.name==='recipient'){ui.picked=e.target.value;resetCheck();draw();}});
 $('compose-form').addEventListener('input',e=>{if(e.target.id!=='compose-find')resetCheck();});
 document.addEventListener('keydown',e=>{if(e.key==='Escape')close();});
 document.addEventListener('click',e=>{if(e.target.closest('[data-profile-switch],[data-profile-remove],#logout'))close();},true);
});
window.SynergiaMessaging={compose,reply,close};
})();
