from pathlib import Path
import sys
main=Path(sys.argv[1])
s=main.read_text(encoding='utf-8')
old='''    private var compactSelection: Binding<DirectPrimarySection> {
        Binding(
            get: { DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection) },
            set: { selectedSection = $0 }
        )
    }
'''
new='''    private var compactSelection: Binding<DirectPrimarySection> {
        Binding(
            get: { DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection) },
            set: { selectedSection = $0 }
        )
    }

    // A two-column NavigationSplitView can clear its optional selection when
    // the iPad window changes width or sidebar visibility. Keep the detail
    // and highlighted sidebar row in agreement: Start is always a real row.
    private var sidebarSelection: Binding<DirectPrimarySection?> {
        Binding(
            get: { DirectAdaptiveNavigationPolicy.selectedOrStart(selectedSection) },
            set: { selectedSection = DirectAdaptiveNavigationPolicy.selectedOrStart($0) }
        )
    }
'''
assert s.count(old)==1,('compact anchor',s.count(old))
s=s.replace(old,new,1)
old='''                List(DirectPrimarySection.allCases, selection: $selectedSection) { section in'''
new='''                List(DirectPrimarySection.allCases, selection: sidebarSelection) { section in'''
assert s.count(old)==1
s=s.replace(old,new,1)
old='''            TabView(selection: compactSelection) {
                NavigationStack { DashboardView() }
                    .tabItem { Label("Start", systemImage: "house.fill") }
                    .tag(DirectPrimarySection.start)
                NavigationStack { ModuleView(module: .grades) }
                    .tabItem { Label("Oceny", systemImage: "graduationcap.fill") }
                    .tag(DirectPrimarySection.grades)
                NavigationStack { ModuleView(module: .timetable) }
                    .tabItem { Label("Plan", systemImage: "calendar") }
                    .tag(DirectPrimarySection.timetable)
                NavigationStack { ModuleView(module: .messages) }
                    .tabItem { Label("Wiadomości", systemImage: "envelope.fill") }
                    .tag(DirectPrimarySection.messages)
                NavigationStack { MoreView() }
                    .tabItem { Label("Więcej", systemImage: "square.grid.2x2.fill") }
                    .tag(DirectPrimarySection.more)
            }
'''
new='''            TabView(selection: compactSelection) {
                // Both navigation surfaces come from the same five canonical
                // destinations, preventing title/icon/route drift between
                // narrow iPhone and regular iPad windows.
                ForEach(DirectPrimarySection.allCases) { section in
                    NavigationStack { rootContent(for: section) }
                        .tabItem { Label(section.title, systemImage: section.symbol) }
                        .tag(section)
                }
            }
'''
assert s.count(old)==1,('tab anchor',s.count(old))
s=s.replace(old,new,1)
main.write_text(s,encoding='utf-8')
print('P54 applied: common five destinations, sidebar selection normalization')
