"""Chromium: odczyt kontrastu w rzeczywistym HTML/CSS/JS z fikcyjnymi danymi."""
from pathlib import Path
import shutil
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1] / "static"
html = (ROOT / "index.html").read_text().replace('<script src="/static/app.js" defer></script>', '').replace('<link rel="stylesheet" href="/static/style.css">', '')
fake = r"""
window.fetch = async (url,init={}) => {
  const path=String(url).split('?')[0];
  const profile={authenticated:true, mode:'demo', name:'Testowy uczeń', active_profile_id:'demo',
                 profiles:[{id:'demo',name:'Testowy uczeń',mode:'demo'}]};
  const sections={
    grades:{subjects:[],count:0},
    timetable:{days:[]},attendance:{stats:{total:0}},
    agenda:{items:[]},homework:{items:[]},
    messages:{items:[{id:'fake1',sender:'Nauczyciel testowy',subject:'Projekt testowy',
                      preview:'Wiadomość testowa z załącznikiem',date:'2026-11-03',unread:true}],
              unread:1},
    notes:{items:[]},announcements:{items:[]}
  };
  let data={}, status=200;
  if(path==='/api/session') data={authenticated:false};
  else if(path==='/api/demo') data=profile;
  else if(path==='/api/message/fake1') data={
    subject:'Projekt testowy',sender:'Nauczyciel testowy',date:'2026-11-03',
    body:'Dzień dobry,\\nTo jest jasny tekst z dwoma wierszami.\\nPozdrawiam.',
    attachments:['przykladowy_plik.jpeg']
  };
  else if(path.startsWith('/api/section/')) data=sections[path.split('/').at(-1)]||{items:[]};
  else { status=404;data={detail:'Test: nieznana ścieżka'}; }
  return new Response(JSON.stringify(data),{status,headers:{'Content-Type':'application/json'}});
};
"""

JS_CONTRAST = """(selector) => {
  const el=document.querySelector(selector);
  if(!el) throw Error('Missing '+selector);
  const parse=(css)=> {
    const m=css.match(/rgba?\\(([^)]*)\\)/);
    if(!m) throw Error('Bad CSS color '+css);
    return m[1].split(',').slice(0,3).map(v=>Number(v.trim()));
  };
  const luminance=(rgb)=>{
    const linear=rgb.map(v=>{
      const s=v/255;
      return s<=0.04045?s/12.92:Math.pow((s+0.055)/1.055,2.4);
    });
    return linear[0]*0.2126+linear[1]*0.7152+linear[2]*0.0722;
  };
  let node=el,bg='rgb(255, 255, 255)';
  while(node) {
    const c=getComputedStyle(node).backgroundColor;
    if(c && c!== 'transparent' && !c.endsWith(', 0)') {bg=c;break;}
    node=node.parentElement;
  }
  const fg=getComputedStyle(el).color;
  const l1=luminance(parse(fg)),l2=luminance(parse(bg));
  return {fg,bg,ratio:(Math.max(l1,l2)+0.05)/(Math.min(l1,l2)+0.05)};
}"""

with sync_playwright() as p:
    browser_path = shutil.which("chromium") or shutil.which("chromium-browser")
    launch = {"executable_path":browser_path} if browser_path else {}
    browser=p.chromium.launch(headless=True,args=['--no-sandbox','--disable-dev-shm-usage'],**launch)
    for width,height in [(390,844),(1280,900)]:
        page=browser.new_page(viewport={'width':width,'height':height},device_scale_factor=1)
        errors=[]
        page.on("pageerror",lambda e:errors.append(str(e)))
        page.set_content(html)
        page.add_style_tag(content=(ROOT/"style.css").read_text())
        page.add_script_tag(content=(ROOT/"theme.js").read_text())
        page.evaluate(fake)
        # Test uruchamia HTML w about:blank, więc podsuwamy testową bazę URL.
        page.add_script_tag(content=(ROOT/"app.js").read_text().replace('window.location.origin', '"https://testing.invalid"'))
        page.locator("#demo-btn").click()
        page.locator("#workspace:not(.hidden)").wait_for()
        page.evaluate("navigate('messages')")
        page.locator('button[data-action="message"]').first.click()
        expect(page.locator("#dialog-title")).to_have_text("Projekt testowy")
        expect(page.locator("#dialog-body .message-body-text")).to_contain_text("Dzień dobry")
        expect(page.locator("#dialog-body .message-modal-attachments")).to_contain_text("przykladowy_plik.jpeg")
        for theme in ["dark","light"]:
            page.evaluate(f"window.SynergiaTheme.set('{theme}')")
            for selector,minimum in [
                ("#dialog-body .message-body-text",7),
                ("#dialog-body .message-modal-meta",4.5),
                ("#dialog-body .message-modal-attachments",4.5),
                (".message-row p",4.5),
                (".message-row time",4.5),
                (".bottom-link:not(.active)",4.5),
            ]:
                # Obliczamy rzeczywisty kolor CSS i najbliższe nieprzezroczyste tło.
                if selector.startswith(".bottom-link") and width>620:
                    continue
                value=page.evaluate(JS_CONTRAST,selector)
                assert value["ratio"]>=minimum,(width,theme,selector,value)
                print(f"PASS {width}px {theme} {selector}: {value['ratio']:.2f}:1")
        assert not errors,errors
        page.close()
    browser.close()
