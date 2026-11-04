"""Chromium: pobieranie załącznika Wiadomości na telefonie i desktopie, bez Librusa."""
from pathlib import Path
import shutil
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1] / "static"
html = (ROOT / "index.html").read_text()
fake = r"""
window.fetch = async (url,init={}) => {
  const path=String(url).split('?')[0];
  const profile={authenticated:true,mode:'demo',name:'Uczeń testowy',
                 active_profile_id:'demo',profiles:[{id:'demo',name:'Uczeń testowy',mode:'demo'}]};
  const respond=(data,status=200)=>new Response(JSON.stringify(data),{
    status,headers:{'Content-Type':'application/json'}});
  if(path==='/api/session') return respond({authenticated:false});
  if(path==='/api/demo') return respond(profile);
  if(path==='/api/message/m1') return respond({
    id:'m1',subject:'Sprawdzian',sender:'Nauczyciel testowy',
    date:'2026-11-04',body:'Testowa wiadomość z załącznikami.',
    attachments:['Zgoda na wycieczkę','<img src=x onerror=alert(1)>'],
    attachment_files:[
      {id:'a1',filename:'Zgoda na wycieczkę'},
      {id:'a2',filename:'<img src=x onerror=alert(1)>'}
    ]
  });
  if(path==='/api/message-attachments/m1/a1/download') return new Response(
    new TextEncoder().encode('Demo file'),
    {status:200,headers:{
      'Content-Type':'application/pdf',
      'Content-Disposition':"attachment; filename=\"Zgoda.pdf\"; filename*=UTF-8''Zgoda%20na%20wycieczk%C4%99.pdf"
    }}
  );
  if(path.startsWith('/api/section/')) {
    const section=path.split('/').at(-1);
    if(section==='messages') return respond({items:[{
       id:'m1',sender:'Nauczyciel',subject:'Sprawdzian',date:'2026-11-04',
       preview:'Materiały',unread:false
    }],unread:0});
    return respond({items:[],subjects:[],count:0,days:[],stats:{total:0}});
  }
  return respond({detail:'Błąd demonstracyjnego endpointu'},404);
};
"""
with sync_playwright() as p:
    binary = shutil.which("chromium") or shutil.which("chromium-browser")
    opts = {"executable_path": binary} if binary else {}
    browser = p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'],**opts)
    for width,height in [(390,844),(1280,900)]:
        page=browser.new_page(viewport={'width':width,'height':height},accept_downloads=True)
        errors=[]
        page.on("pageerror",lambda error:errors.append(str(error)))
        page.set_content(html)
        page.add_style_tag(content=(ROOT/"style.css").read_text())
        page.add_style_tag(content=(ROOT/"message-attachments.css").read_text())
        page.evaluate(fake)
        page.add_script_tag(content=(ROOT/"message-attachments.js").read_text())
        page.add_script_tag(content=(ROOT/"app.js").read_text().replace(
            'window.location.origin', '"https://testing.invalid"'))
        page.locator("#demo-btn").click()
        page.locator("#workspace:not(.hidden)").wait_for()
        page.evaluate("navigate('messages')")
        page.locator('button[data-action="message"]').first.click()
        expect(page.locator(".message-files-title")).to_contain_text("Załączniki (2)")
        expect(page.locator(".message-files-info").first).to_have_text("Zgoda na wycieczkę")
        assert page.locator(".message-files-row img").count()==0
        with page.expect_download(timeout=12000) as info:
            page.locator(".message-files-row button").first.click()
        assert info.value.suggested_filename=="Zgoda na wycieczkę.pdf", info.value.suggested_filename
        expect(page.locator(".message-files-status")).to_contain_text("Zgoda na wycieczkę.pdf")
        page.locator("#dialog-close").click()
        expect(page.locator("#message-dialog.hidden")).to_have_count(1)
        page.evaluate("navigate('grades')")
        expect(page.locator("#content h1")).to_have_text("Oceny")
        assert not errors, errors
        page.close()
    browser.close()
print("PASS Chromium: message attachments, download filename, mobile+desktop, HTML injection, grades fallback")
