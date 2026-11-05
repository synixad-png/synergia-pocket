package main

import "strings"

func runtimeHTMLV12() string {
	page := strings.Replace(htmlPage, "</head>", "<style>"+messagesCSSV12+"</style></head>", 1)
	return strings.Replace(page, "</body>", "<script>"+messagesJSV12+"</script></body>", 1)
}

const messagesCSSV12 = `
.msg12-back{min-height:44px}.msg12-back-bottom{margin:14px 0 20px}.gui .msg12-tools .msg12-back{flex-basis:100%;margin-left:0}
.msg12-tools{display:flex;flex-wrap:wrap;gap:8px;margin:12px 0 18px}.msg12-tools .active{background:#275a92;border-color:#68aaff}
.msg12-card{padding:16px;border:1px solid #3c5976;border-radius:12px;background:#102137;margin:10px 0;min-width:0;overflow-wrap:anywhere}
.msg12-card h4{font-size:17px;margin:0 0 8px}.msg12-meta{color:#bfd2e7;font-size:12px;white-space:pre-line;line-height:1.7;margin:8px 0}
.msg12-preview{color:#b5cade;margin:8px 0;white-space:pre-line}.msg12-body{white-space:pre-wrap;line-height:1.8;font-size:15px;overflow-wrap:anywhere;border-top:1px solid #3c5976;padding-top:16px}
.msg12-form{display:grid;gap:14px;max-width:850px}.msg12-form label{display:grid;gap:6px;color:#c8dcf1;font-weight:650}
.msg12-form input,.msg12-form textarea,.msg12-form select{width:100%;min-width:0;border:1px solid #547494;background:#0c1b2d;color:#edf5ff;border-radius:9px;padding:12px;font:inherit;box-sizing:border-box}
.msg12-form textarea{min-height:220px;resize:vertical}.msg12-files{display:flex;gap:10px;flex-wrap:wrap;margin:16px 0}.msg12-files button{max-width:100%;white-space:normal;overflow-wrap:anywhere}
.msg12-form button:focus-visible,.msg12-tools button:focus-visible,.msg12-card button:focus-visible{outline:3px solid #ffcc79;outline-offset:3px}
@media(max-width:600px){.msg12-tools button{flex:1 1 100%;min-width:0;min-height:44px;white-space:normal;overflow-wrap:normal;word-break:normal}.msg12-card{padding:13px}.msg12-form input,.msg12-form select,.msg12-form textarea{font-size:16px}}
`

const messagesJSV12 = `
(() => {
 'use strict';
 const host=window.SynergiaMessageHostV12;
 let epoch=0,box='inbox',chosen=-1;
 const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n;};
 const button=(text,fn,cls='secondary')=>{const b=el('button',text,cls);b.type='button';b.addEventListener('click',fn);return b;};
 const active=(seq)=>seq===epoch&&host.profile()===chosen&&host.active();
 const path=(route,extra={})=>'/api/messages/'+route+'?'+new URLSearchParams({profile:String(chosen),...extra});
 const request=async(route,extra={},init={})=>{
   const response=await fetch(path(route,extra),{credentials:'same-origin',cache:'no-store',...init});
   const data=await response.json().catch(()=>({error:'Nieprawidłowa odpowiedź lokalnego programu.'}));
   if(!response.ok)throw Error(data.error||'Operacja nie powiodła się.');return data;
 };
 function root(){const n=el('div');n.id='msg12';const body=document.getElementById('view-body');body.classList.remove('blank');body.replaceChildren(n);return n;}
 function backButton(mailbox){const b=button('← Wstecz do '+(mailbox==='outbox'?'wysłanych':'odebranych'),()=>list(mailbox),'secondary msg12-back');b.dataset.messageBack=mailbox;return b;}
 function tools(where,backTo=''){const bar=el('div',null,'msg12-tools');if(backTo)bar.append(backButton(backTo));bar.append(button('Odebrane',()=>list('inbox')),button('Wysłane',()=>list('outbox')),button('Nowa wiadomość',()=>compose()));where.append(bar);}
 function error(where,text){where.append(el('p',text,'status error'));}
 async function list(mailbox='inbox'){
   chosen=host.profile();box=mailbox;const seq=++epoch;const wrap=root();tools(wrap);
   host.header('Wiadomości',mailbox==='inbox'?'Odebrane — otwarcie pełnej wiadomości oznacza ją w Librusie jako przeczytaną.':'Wysłane wiadomości wybranego profilu.');
   if(!host.logged()){error(wrap,'Najpierw zaloguj wybrany profil.');return;}
   const waiting=el('p','Pobieranie listy…');wrap.append(waiting);
   try{
     const data=await request('list',{box:mailbox});if(!active(seq))return;waiting.remove();
     if(!Array.isArray(data.items))throw Error('Brak poprawnej listy wiadomości.');
     if(!data.items.length)wrap.append(el('p','Skrzynka jest pusta.'));
     for(const item of data.items){
       const card=el('article',null,'msg12-card');card.append(el('h4',item.subject||'(bez tematu)'));
       card.append(el('p',(mailbox==='outbox'?'Do: '+(item.receiver||'—'):'Od: '+(item.sender||'—'))+' · '+(item.date||'')+(item.unread&&mailbox==='inbox'?' · Nieprzeczytana':''),'msg12-meta'));
       card.append(el('p',String(item.preview||'').slice(0,300),'msg12-preview'));
       if(item.hasAttachments)card.append(el('p','Zawiera załączniki','msg12-meta'));
       const b=button('Otwórz',()=>open(item.id,mailbox));b.dataset.messageOpen=item.id;card.append(b);wrap.append(card);
     }
     host.status('Pobrano '+data.items.length+' wiadomości. Lista obejmuje do 30 ostatnich wpisów.','success');
   }catch(e){if(active(seq)){waiting.remove();error(wrap,e.message);host.status(e.message,'error');}}
 }
 async function open(id,mailbox){
   const seq=++epoch,wrap=root();tools(wrap,mailbox);wrap.append(el('p','Otwieranie wiadomości…'));
   try{
     const data=await request('open',{box:mailbox,id},{method:'POST'});if(!active(seq))return;
     const m=data.items;wrap.replaceChildren();tools(wrap,mailbox);
     const card=el('article',null,'msg12-card');card.append(el('h4',m.subject||'(bez tematu)'),el('p',(m.sender||'')+' · '+(m.date||''),'msg12-meta'),el('div',m.content||'','msg12-body'));
     const files=el('div',null,'msg12-files');
     for(const file of m.attachments||[]){const b=button('Pobierz: '+file.name,()=>download(m,file,b));b.dataset.messageFile=file.id;files.append(b);}
     card.append(files);
     if(mailbox==='inbox'&&!m.noReply&&/^[0-9]+$/.test(m.senderId||'')){card.append(button('Odpowiedz',()=>compose(m)));}
     wrap.append(card);const back=backButton(mailbox);back.classList.add('msg12-back-bottom');wrap.append(back);host.status('Otworzono wiadomość.','success');
   }catch(e){if(active(seq)){wrap.replaceChildren();tools(wrap,mailbox);error(wrap,e.message);host.status(e.message,'error');}}
 }
 function filename(header){
   let m=String(header||'').match(/filename\*=UTF-8''([^;]+)/i);if(m){try{return decodeURIComponent(m[1].trim());}catch(_){}}
   m=String(header||'').match(/filename="([^"]+)"|filename=([^;]+)/i);return m?(m[1]||m[2]).trim():'zalacznik.bin';
 }
 async function download(message,file,b){
   const seq=epoch;b.disabled=true;host.status('Pobieranie załącznika…');
   const ctrl=new AbortController(),timer=setTimeout(()=>ctrl.abort(),155000);
   try{
     const response=await fetch(path('download',{box:message.box,id:message.id,file:file.id}),{method:'POST',credentials:'same-origin',cache:'no-store',signal:ctrl.signal});
     if(!response.ok){const data=await response.json().catch(()=>({}));throw Error(data.error||'Nie udało się pobrać załącznika.');}
     const blob=await response.blob();if(!active(seq))return;
     const url=URL.createObjectURL(blob),a=el('a');a.href=url;a.download=filename(response.headers.get('Content-Disposition'));document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);
     host.status('Pobrano załącznik: '+a.download,'success');
   }catch(e){if(active(seq))host.status(e.name==='AbortError'?'Przekroczono czas pobierania.':e.message,'error');}
   finally{clearTimeout(timer);b.disabled=false;}
 }
 async function compose(reply=null,draft=null){
   const seq=++epoch,wrap=root();tools(wrap);host.header(reply?'Odpowiedź':'Nowa wiadomość','Profil '+(chosen+1)+'. Wiadomość zostanie wysłana dopiero po sprawdzeniu podglądu.');
   if(!host.logged()){error(wrap,'Najpierw zaloguj wybrany profil.');return;}
   const form=el('form',null,'msg12-form');const recipient=el('select');recipient.required=true;
   const subject=el('input');subject.maxLength=200;subject.required=true;
   const text=el('textarea');text.maxLength=10000;text.required=true;
   const label=(name,input)=>{const n=el('label',name);n.append(input);form.append(n);};
   if(reply){const option=el('option',reply.sender);option.value=reply.senderId;recipient.append(option);recipient.disabled=true;}
   else{
     wrap.append(el('p','Pobieranie odbiorców…'));
     try{const data=await request('recipients');if(!active(seq))return;
       recipient.append(el('option','Wybierz osobę…'));recipient.firstChild.value='';
       for(const person of data.items||[]){const o=el('option',(person.name||person.id)+' · '+person.group);o.value=person.id;recipient.append(o);}
       if(recipient.options.length===1)throw Error('Brak dostępnych odbiorców dla tego profilu.');
     }catch(e){if(active(seq)){wrap.replaceChildren();tools(wrap);error(wrap,e.message);}return;}
   }
   if(!active(seq))return;wrap.replaceChildren();tools(wrap);
   label('Odbiorca',recipient);label('Temat',subject);label('Treść',text);
   subject.value=draft?.topic||(reply?(String(reply.subject||'').toLowerCase().startsWith('re:')?reply.subject:'Re: '+(reply.subject||'')).slice(0,200):'');text.value=draft?.text||'';
   if(draft&&!reply)recipient.value=draft.recipient;
   const submit=el('button','Podgląd wysyłki','primary');submit.type='submit';form.append(submit);wrap.append(form);
   form.addEventListener('submit',event=>{
     event.preventDefault();if(!form.reportValidity())return;
     const d={topic:subject.value.trim(),text:text.value.trim(),recipient:recipient.value};
     if(!d.topic||!d.text){error(wrap,'Podaj temat i treść wiadomości.');return;}
     const bytes=new Uint8Array(16);crypto.getRandomValues(bytes);const requestId=Array.from(bytes,x=>x.toString(16).padStart(2,'0')).join('');
     const payload={topic:d.topic,text:d.text,recipients:reply?[]:[d.recipient],replyTo:reply?reply.id:'',requestId};
     wrap.replaceChildren();const card=el('section',null,'msg12-card');
     card.append(el('h4','Sprawdź wiadomość przed wysłaniem'),el('p','Profil '+(chosen+1)+' · Do: '+recipient.selectedOptions[0].textContent,'msg12-meta'),el('h4',d.topic),el('div',d.text,'msg12-body'));
     const actions=el('div',null,'msg12-tools');const edit=button('Wróć do edycji',()=>compose(reply,d));
     const send=button('Wyślij wiadomość',async()=>{
       if(send.disabled||!active(seq))return;send.disabled=true;edit.disabled=true;let outcome='unknown';host.busy(true);
       try{
         const result=await request('send',{}, {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
         if(!active(seq))return;outcome=result.state;
         const output=el('p',result.message,result.state==='sent'?'status success':'status error');card.append(output);
         if(result.state==='not_sent'){edit.disabled=false;}
         actions.append(button('Sprawdź Wysłane',()=>list('outbox')));
         host.status(result.message,result.state==='sent'?'success':'error');
       }catch(e){if(active(seq)){const msg='Nie otrzymano pewnego wyniku. Wiadomość mogła zostać wysłana. Sprawdź Wysłane.';error(card,msg);actions.append(button('Sprawdź Wysłane',()=>list('outbox')));host.status(msg,'error');}}
       finally{host.busy(false);send.disabled=true;edit.disabled=outcome!=='not_sent';}
     },'primary');actions.append(edit,send);card.append(actions);wrap.append(card);
   });
 }
 function leave(){epoch++;if(host.active()){const body=document.getElementById('view-body');body.replaceChildren(el('p','Wybrano inny profil lub sesję. Otwórz Wiadomości ponownie.'));}}
 window.SynergiaMessagesV12={open:()=>list('inbox'),leave};
})();
`
