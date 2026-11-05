"""Run manually with Playwright; uses no real subscriptions or school accounts."""
from pathlib import Path
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
with sync_playwright() as pw:
    browser = pw.chromium.launch(headless=True)
    page = browser.new_page()
    state = {'active': False, 'subscribe_error': False}
    def route(req):
        path = req.request.url.split('https://push.test')[-1]
        if path == '/':
            req.fulfill(content_type='text/html', body='<main id="content"><button id="push-enable">Enable</button><button id="push-disable">Disable</button><button id="push-test">Test</button><p id="push-status"></p></main>')
        elif path == '/api/push/config':
            req.fulfill(json={'available': True, 'public_key':'BA'})
        elif path == '/api/push/status':
            req.fulfill(json={'active':state['active'], 'profiles':['Test'], 'last_checked':1})
        elif path == '/api/push/subscribe':
            if state['subscribe_error']:
                req.fulfill(status=503, json={'detail':'Nie można teraz pobrać wiadomości z Synergii.'})
            else:
                state['active'] = True
                req.fulfill(json={'ok':True, 'profiles':['Test']})
        else:
            req.fulfill(status=404,body='missing')
    page.route('https://push.test/**', route)
    page.goto('https://push.test/')
    page.evaluate('''() => {
      const sub={endpoint:'https://web.push.apple.com/mock',toJSON:()=>({keys:{p256dh:'mock',auth:'mock'}})};
      window.PushManager=function(){};
      Object.defineProperty(window.Notification,'permission',{get:()=> 'granted'});
      window.Notification.requestPermission=()=>Promise.resolve('granted');
      const reg={pushManager:{getSubscription:async()=>sub}};
      Object.defineProperty(navigator,'serviceWorker',{value:{register:async()=>reg,ready:Promise.resolve(reg),addEventListener:()=>{}}});
    }''')
    page.add_script_tag(content=(ROOT/'static/push.js').read_text())
    # Trigger the real dynamic-panel observer, without renewing the lost record first.
    page.evaluate("document.getElementById('content').append(document.createElement('span'))")
    page.wait_for_function("document.getElementById('push-status').textContent.includes('Brak aktywnego')")
    assert page.locator('#push-enable').is_visible()
    assert not page.locator('#push-test').is_visible()
    state['subscribe_error'] = True
    page.locator('#push-enable').click()
    page.wait_for_function("document.getElementById('push-status').textContent.includes('Nie można teraz')")
    page.wait_for_timeout(200)
    assert 'Nie można teraz' in page.locator('#push-status').inner_text()
    state['subscribe_error'] = False
    page.locator('#push-enable').click()
    page.wait_for_function("document.getElementById('push-status').textContent.includes('Serwer potwierdza')")
    assert page.locator('#push-test').is_visible()
    assert 'Ostatnie sprawdzenie' in page.locator('#push-status').inner_text()
    browser.close()
print('PASS: missing server registration, persistent error, renewal and confirmed status')
