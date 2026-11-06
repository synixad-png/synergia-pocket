from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text(encoding='utf8')
def change(old,new,tag):
 global s
 n=s.count(old)
 assert n==1,(tag,'expected unique anchor',n)
 s=s.replace(old,new,1)

change('''    private func submit(alternative: Bool = false) {
        let value = schoolPassword
''','''    // An existing verified account must be explicitly cleared before another
    // credential attempt. Enforced both by button state and at call site.
    private var mayStartSchoolLogin: Bool {
        portal.store.verifiedAccountId == nil &&
        !portal.nativeLoginPhase.isBusy && !portal.isBusy &&
        !portal.coreSyncBusy && !portal.isSyncing && !portal.privacyShieldActive &&
        !schoolLogin.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !schoolPassword.isEmpty
    }

    private func submit(alternative: Bool = false) {
        guard mayStartSchoolLogin else { return }
        let value = schoolPassword
''','submit guard')
change('''                        .focused($focused, equals: .login)
                        .submitLabel(.next)
''','''                        .focused($focused, equals: .login)
                        .disabled(portal.store.verifiedAccountId != nil)
                        .submitLabel(.next)
''','login field')
change('''                        .focused($focused, equals: .password)
                        Button {
''','''                        .focused($focused, equals: .password)
                        .disabled(portal.store.verifiedAccountId != nil)
                        Button {
''','password field')
change('''                    .disabled(portal.nativeLoginPhase.isBusy || portal.isBusy ||
                              portal.coreSyncBusy || portal.isSyncing || portal.privacyShieldActive ||
                              schoolLogin.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                              schoolPassword.isEmpty)
''','''                    .disabled(!mayStartSchoolLogin)
''','primary button')
change('''                            .disabled(schoolPassword.isEmpty || portal.nativeLoginPhase.isBusy)
''','''                            .disabled(!mayStartSchoolLogin)
''','fallback button')
change('''    @State private var askToErase = false

    private func localStatus(for module: SchoolModule) -> String {
''','''    @State private var askToErase = false

    private var mayRevealAccountData: Bool {
        portal.store.verifiedAccountId != nil && !portal.privacyShieldActive
    }
    private var mayEraseAccountData: Bool {
        !portal.isBusy && !portal.isSyncing && !portal.coreSyncBusy &&
        !portal.nativeLoginPhase.isBusy
    }

    private func localStatus(for module: SchoolModule) -> String {
        guard mayRevealAccountData else { return "Zablokowane" }
''','settings privacy')
change('''                HStack { Text("Zachowane moduły JSON"); Spacer(); Text("\\(portal.directOfflineFeedStore.feeds.count)").foregroundStyle(.secondary) }
''','''                HStack { Text("Zachowane moduły JSON"); Spacer(); Text(mayRevealAccountData ? "\\(portal.directOfflineFeedStore.feeds.count)" : "Zablokowane").foregroundStyle(.secondary) }
''','feed count')
change('''                HStack { Text("Pobrane pliki"); Spacer(); Text("\\(portal.downloads.items.count)").foregroundStyle(.secondary) }
                HStack { Text("Zapamiętane moduły"); Spacer(); Text("\\(portal.routes.routes.count)").foregroundStyle(.secondary) }
                Button("Usuń lokalne dane, załączniki oraz cookies", role: .destructive) { askToErase = true }
                    .disabled(portal.isBusy)
''','''                HStack { Text("Pobrane pliki"); Spacer(); Text(mayRevealAccountData ? "\\(portal.downloads.items.count)" : "Zablokowane").foregroundStyle(.secondary) }
                HStack { Text("Zapamiętane moduły"); Spacer(); Text(mayRevealAccountData ? "\\(portal.routes.routes.count)" : "Zablokowane").foregroundStyle(.secondary) }
                Button("Usuń lokalne dane, załączniki oraz cookies", role: .destructive) { askToErase = true }
                    .disabled(!mayEraseAccountData)
''','counts and erase button')
change('''        .confirmationDialog("Usunąć zapisane moduły, załączniki i lokalną sesję WebKit?", isPresented: $askToErase) {
            Button("Usuń z urządzenia", role: .destructive) { portal.signOutAndClear() }
        }
''','''        .confirmationDialog("Usunąć zapisane moduły, załączniki i lokalną sesję WebKit?",
                            isPresented: $askToErase, titleVisibility: .visible) {
            Button("Usuń z urządzenia", role: .destructive) {
                guard mayEraseAccountData else { return }
                portal.signOutAndClear()
            }
            Button("Anuluj", role: .cancel) {}
        } message: {
            Text("Ta operacja usuwa z urządzenia zapisane dane i pliki aplikacji oraz cookies WebKit. " +
                 "Nie wylogowuje konta z serwera Synergii i nie usuwa plików wcześniej udostępnionych w Plikach iOS.")
        }
''','settings confirmation')
change('''struct DownloadsView: View {
    @EnvironmentObject private var portal: SynergiaPortal
    @Environment(\\.horizontalSizeClass) private var horizontalSizeClass
    var body: some View {
''','''struct DownloadsView: View {
    @EnvironmentObject private var portal: SynergiaPortal
    @Environment(\\.horizontalSizeClass) private var horizontalSizeClass
    private var mayRevealDownloads: Bool {
        portal.store.verifiedAccountId != nil && !portal.privacyShieldActive
    }
    var body: some View {
''','downloads policy')
change('''            Section("Zapisane pliki") {
                if portal.downloads.items.isEmpty {
''','''            Section("Zapisane pliki") {
                if !mayRevealDownloads {
                    ContentUnavailableView("Dane zablokowane", systemImage: "lock.shield",
                        description: Text("Zweryfikuj bieżące konto w Synergii, aby wyświetlić lokalne załączniki."))
                } else if portal.downloads.items.isEmpty {
''','downloads list lock')

p.write_text(s,encoding='utf8')
print(f'P57 applied: {p.name}, bytes={p.stat().st_size}, patches=10')
