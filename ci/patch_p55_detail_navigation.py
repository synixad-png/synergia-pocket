from pathlib import Path
import sys
p=Path(sys.argv[1])
s=p.read_text(encoding='utf-8')
old="""            } detail: {
                NavigationStack { rootContent(for: DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection)) }
            }
"""
new="""            } detail: {
                // Changing the main sidebar section must also reset any previously
                // pushed detail screen. Without a distinct identity, SwiftUI can
                // reuse the prior NavigationStack even when its root changes.
                NavigationStack { rootContent(for: DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection)) }
                    .id(DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection))
            }
"""
assert s.count(old)==1, 'P54 NavigationStack anchor count must be 1'
s=s.replace(old,new,1)
p.write_text(s, encoding='utf-8')
print('P55: reset iPad detail stack on main-section change')
