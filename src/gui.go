package main

import "strings"

// A web-only presentation layer. Existing feature routes, message actions,
// attachment downloads and profile isolation keep their original handlers.
func librusGUI(page string) string {
	shell, e1 := cloudAssets.ReadFile("web/gui-shell.html")
	css, e2 := cloudAssets.ReadFile("web/gui.css")
	js, e3 := cloudAssets.ReadFile("web/gui.js")
	if e1 != nil || e2 != nil || e3 != nil {
		panic("missing web GUI assets")
	}
	start := strings.Index(page, "<body>")
	end := strings.Index(page, "<script>")
	if start < 0 || end < start {
		panic("web GUI mount missing")
	}
	page = page[:start] + "<body class=\"gui\">" + string(shell) + page[end:]
	page = strings.Replace(page, `content="width=device-width, initial-scale=1"`, `content="width=device-width, initial-scale=1, viewport-fit=cover"`, 1)
	page = strings.Replace(page, "</head>", "<style>"+string(css)+"</style><script>try{document.documentElement.dataset.theme=localStorage.getItem('pocket-theme')||'dark';}catch(e){document.documentElement.dataset.theme='dark';}</script></head>", 1)
	return strings.Replace(page, "</body>", "<script>"+string(js)+"</script></body>", 1)
}
