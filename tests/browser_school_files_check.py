"""Chromium: nowe Dokumenty szkoły na mobile i desktop, bez prawdziwego konta."""
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
  if(path==='/api/session') return new Response(JSON.stringify({authenticated:false}),{
    status:200,headers:{'Content-Type':'application/json'}});
  if(path==='/api/demo') return new Response(JSON.stringify(profile),{
    status:200,headers:{'Content-Type':'application/json'}});
  if(path==='/api/school-files') return new Response(JSON.stringify({demo:true,items:[
    {id:'demo-szkolny-dokument',name:'Szkoła <script>alert(1)</script>',added:'2026-11-04',can_download:true}
  ]}),{status:200,headers:{'Content-Type':'application/json'}});
  if(path==='/api/school-files/demo-szkolny-dokument/download')
    return new Response(new TextEncoder().encode('Fikcyjny dokument'),{status:200,headers:{
      'Content-Type':'text/plain','Content-Disposition':"attachment; filename=\"dokument.txt\"; filename*=UTF-8''Za%C5%82%C4%85cznik%20szkolny.txt"}});
  if(path.startsWith('/api/section/')) return new Response(JSON.stringify({
    items:[],subjects:[],count:0,days:[],stats:{total:0}
  }),{status:200,headers:{'Content-Type':'application/json'}});
  return new Response(JSON.stringify({detail:'Nieznany adres demonstracyjny'}),{
    status:404,headers:{'Content-Type':'application/json'}});
};
"""
with sync_playwright() as playwright:
    binary = shutil.which("chromium") or shutil.which("chromium-browser")
    opts = {"executable_path": binary} if binary else {}
    browser = playwright.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'],**opts)
    for width, height in [(390,844), (1280,900)]:
        page = browser.new_page(viewport={"width":width,"height":height},accept_downloads=True)
        errors=[]
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.set_content(html)
        page.add_style_tag(content=(ROOT/"style.css").read_text())
        page.add_style_tag(content=(ROOT/"school-files.css").read_text())
        page.add_script_tag(content=(ROOT/"school-files.js").read_text())
        page.evaluate(fake)
        page.add_script_tag(content=(ROOT/"app.js").read_text().replace(
            'window.location.origin', '"https://testing.invalid"'
        ))
        page.locator("#demo-btn").click()
        page.locator("#workspace:not(.hidden)").wait_for()
        page.evaluate("navigate('school-files')")
        expect(page.locator(".school-file-meta h3")).to_contain_text("Szkoła <script>alert(1)</script>")
        assert page.locator(".school-file-meta script").count() == 0
        expect(page.locator(".school-file-demo")).to_contain_text("demonstracyjny")
        with page.expect_download(timeout=10000) as info:
            page.locator('button[data-school-download]').click()
        download=info.value
        assert download.suggested_filename == "Załącznik szkolny.txt", download.suggested_filename
        assert not errors, errors
        page.evaluate("navigate('grades')")
        expect(page.locator("h1")).to_have_text("Oceny")
        page.close()
    browser.close()
print("PASS: Chromium school docs, filename*, mobile/desktop, XSS, other tabs")
