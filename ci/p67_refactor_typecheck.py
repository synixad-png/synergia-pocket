from pathlib import Path
import sys
root=Path(sys.argv[1])
p=root/'Sources'/'MainView.swift'
s=p.read_text()
a=s.index('    @ViewBuilder\n    private var adaptiveNavigation: some View {')
b=s.index('    var body: some View {',a)
new='''    /// Keep the desktop/sidebar and phone/tab navigations in small,
    /// independently typechecked SwiftUI expressions for Xcode 16.4.
    private func pocketSidebarButton(_ page: DirectPocketSidebarPage) -> some View {
        let active = page == sidebarPage
        return Button {
            sidebarPage = page
        } label: {
            HStack(spacing: 12) {
                Image(systemName: page.symbol).frame(width: 23)
                Text(page.title).font(.subheadline.weight(.semibold))
                Spacer(minLength: 4)
                if active { Image(systemName: "chevron.right").font(.caption) }
            }
            .foregroundStyle(active ? Color.white : Color(red: 170/255, green: 185/255, blue: 209/255))
            .padding(13)
            .background(active ? DirectPocketStyle.blue : .clear,
                        in: RoundedRectangle(cornerRadius: 12))
        }
        .buttonStyle(.plain)
    }

    private var pocketSidebar: some View {
        VStack(alignment: .leading, spacing: 16) {
            DirectPocketWordmark(onDark: true)
                .padding(.horizontal, 12)
                .padding(.top, 16)
            Text("MENU GŁÓWNE")
                .font(.system(size: 10, weight: .bold))
                .tracking(1.4)
                .foregroundStyle(Color(red: 143/255, green: 161/255, blue: 193/255))
                .padding(.horizontal, 16)
            ScrollView {
                VStack(spacing: 4) {
                    ForEach(DirectPocketSidebarPage.allCases) { page in
                        pocketSidebarButton(page)
                    }
                }
                .padding(.horizontal, 10)
            }
            NavigationLink(destination: ConnectionView()) {
                Label("Konto i połączenie", systemImage: "person.crop.circle")
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(Color(red: 170/255, green: 185/255, blue: 209/255))
                    .padding(12)
            }
            .padding(.horizontal, 10)
            .padding(.bottom, 14)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(DirectPocketStyle.navy)
    }

    private var pocketTabletNavigation: some View {
        NavigationSplitView {
            pocketSidebar
                .navigationTitle("Synergia Pocket")
                .navigationBarTitleDisplayMode(.inline)
        } detail: {
            NavigationStack { wideContent(for: sidebarPage) }
                .id(sidebarPage)
        }
    }

    private var pocketPhoneNavigation: some View {
        TabView(selection: $selectedBottom) {
            ForEach(DirectPocketBottomSection.allCases) { section in
                NavigationStack { rootContent(for: section) }
                    .tabItem { Label(section.title, systemImage: section.symbol) }
                    .tag(section)
            }
        }
    }

    @ViewBuilder
    private var adaptiveNavigation: some View {
        if DirectAdaptiveNavigationPolicy.style(isRegularWidth: horizontalSizeClass == .regular) == .sidebar {
            pocketTabletNavigation
        } else {
            pocketPhoneNavigation
        }
    }

'''
s=s[:a]+new+s[b:]
p.write_text(s)
print('MainView.swift refactored',len(s),'bytes')
