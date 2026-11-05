"""Browser regression for actual embedded restoration code; no account credentials."""
from pathlib import Path
from playwright.sync_api import sync_playwright
source = (Path(__file__).resolve().parents[1] / 'frontend_updates_v13.go').read_text()
script = source.split('const updatesJSV13 = `', 1)[1].rsplit('`', 1)[0]
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    for scenario in ['remembered_external', 'temporary_failure', 'new_browser']:
        context = browser.new_context()
        if scenario != 'new_browser':
            context.add_cookies([dict(name='synergia_local', value='a'*64, domain='pocket.test', path='/', secure=True, sameSite='Strict')])
        seen = dict(profiles=0, bootstrap=0, landing_cookie=False)
        errors = []
        def route(r):
            url = r.request.url
            cookie = r.request.all_headers().get('cookie', '')
            if url.startswith('https://outside.test'):
                r.fulfill(content_type='text/html', body='<a href="https://pocket.test/">Open</a>'); return
            if url == 'https://pocket.test/':
                seen['landing_cookie'] = bool(cookie)
                r.fulfill(content_type='text/html', body='<body><script>window.restored=[];window.statuses=[];window.SynergiaUpdatesHostV13={logged:()=>false,restore:x=>restored.push(x),status:x=>statuses.push(x)};</script><script>'+script+'</script></body>'); return
            if url.endswith('/api/session/bootstrap'):
                seen['bootstrap'] += 1
                assert r.request.method == 'POST'
                r.fulfill(headers={'content-type':'application/json','set-cookie':'synergia_local='+'b'*64+'; Path=/; Secure; HttpOnly; SameSite=Strict'},body='{"ok":true}'); return
            if url.endswith('/api/updates/profiles'):
                seen['profiles'] += 1
                if scenario == 'temporary_failure' and seen['profiles'] == 1:
                    r.fulfill(status=503,json={'error':'temporary'}); return
                if not cookie:
                    r.fulfill(status=401,json={'error':'no session'}); return
                r.fulfill(json={'logged':[cookie.endswith('a'*64),False]}); return
            r.fulfill(status=404,body='')
        context.route('https://**/*',route)
        page = context.new_page(); page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto('https://outside.test/');page.get_by_role('link',name='Open').click()
        page.wait_for_function('restored.length>0')
        assert page.evaluate('restored[0]') == [scenario != 'new_browser', False]
        assert not seen['landing_cookie']
        assert seen['bootstrap'] == (1 if scenario == 'new_browser' else 0)
        assert not errors, errors
        before = seen['profiles']
        page.evaluate('window.dispatchEvent(new PageTransitionEvent("pageshow"))')
        page.wait_for_function('restored.length>1')
        assert seen['profiles'] > before
        print(scenario, 'PASS', seen)
        context.close()
    browser.close()
