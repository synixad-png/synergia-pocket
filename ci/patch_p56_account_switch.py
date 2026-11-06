from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text(encoding="utf-8")
pairs=[
("""    @State private var advancedExpanded = false
    @FocusState private var focused: Field?
""","""    @State private var advancedExpanded = false
    @State private var confirmAccountChange = false
    @FocusState private var focused: Field?
"""),
("""                    .disabled(portal.nativeLoginPhase.isBusy ||
                              schoolLogin.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                              schoolPassword.isEmpty)
""","""                    .disabled(portal.nativeLoginPhase.isBusy || portal.isBusy ||
                              portal.coreSyncBusy || portal.isSyncing || portal.privacyShieldActive ||
                              schoolLogin.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                              schoolPassword.isEmpty)
"""),
("""                .padding(16)
                .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 14))

                DisclosureGroup("Narzędzia połączenia i diagnostyka", isExpanded: $advancedExpanded) {
""","""                .padding(16)
                .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 14))

                // Do not silently replace a verified local account with another.
                // The explicit wipe uses the existing protected cleanup sequence.
                if portal.store.verifiedAccountId != nil {
                    VStack(alignment: .leading, spacing: 10) {
                        Label("Na urządzeniu jest już zweryfikowane konto", systemImage: "person.crop.circle.badge.checkmark")
                            .font(.subheadline.weight(.medium))
                        Text("Przed użyciem innego konta usuń cookies i dane poprzedniego konta. " +
                             "Nie dotyczy to plików wcześniej skopiowanych do aplikacji Pliki.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Button(role: .destructive) { confirmAccountChange = true } label: {
                            Label("Przygotuj zmianę konta", systemImage: "person.crop.circle.badge.xmark")
                        }
                        .buttonStyle(.bordered)
                        .disabled(portal.isBusy || portal.isSyncing || portal.coreSyncBusy ||
                                  portal.nativeLoginPhase.isBusy)
                        .accessibilityHint("Przed zalogowaniem na inne konto wymaga potwierdzenia usunięcia lokalnych danych.")
                    }
                    .padding(16)
                    .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 14))
                }

                DisclosureGroup("Narzędzia połączenia i diagnostyka", isExpanded: $advancedExpanded) {
"""),
("""        .onDisappear {
            schoolPassword = ""
            if portal.nativeLoginPhase.isBusy { portal.cancelNativeSchoolLogin() }
        }
    }
}

struct MoreView: View {
""","""        .onDisappear {
            schoolPassword = ""
            if portal.nativeLoginPhase.isBusy { portal.cancelNativeSchoolLogin() }
        }
        .confirmationDialog("Zmienić konto na tym urządzeniu?",
                            isPresented: $confirmAccountChange, titleVisibility: .visible) {
            Button("Usuń sesję, dane lokalne i cookies", role: .destructive) {
                schoolPassword = ""
                schoolLogin = ""
                showPassword = false
                focused = nil
                portal.signOutAndClear()
            }
            Button("Anuluj", role: .cancel) {}
        } message: {
            Text("Usunięte zostaną cookies, zapisane dane i załączniki tego urządzenia oraz ustawienia powiadomień. " +
                 "Nie wyloguje to konta na serwerze Synergii. Po zakończeniu czyszczenia wpisz nowe dane logowania.")
        }
    }
}

struct MoreView: View {
"""),
("""                NavigationLink(destination: ConnectionView()) {
                    Label("Otwórz logowanie i połączenie", systemImage: "lock.shield")
                }
                Text("Po powrocie z tła dostęp do lokalnych danych wymaga ponownego potwierdzenia. Brak internetu nie oznacza wylogowania z serwera.")
""","""                NavigationLink(destination: ConnectionView()) {
                    Label("Otwórz logowanie i połączenie", systemImage: "lock.shield")
                }
                if portal.store.verifiedAccountId != nil {
                    Text("Aby zmienić konto, otwórz Połączenie i wybierz „Przygotuj zmianę konta”. " +
                         "Usunięcie danych poprzedniego konta wymaga osobnego potwierdzenia.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                Text("Po powrocie z tła dostęp do lokalnych danych wymaga ponownego potwierdzenia. Brak internetu nie oznacza wylogowania z serwera.")
""")
]
for i,(old,new) in enumerate(pairs,1):
    n=s.count(old)
    assert n==1,("P56 patch anchor",i,n)
    s=s.replace(old,new,1)
p.write_text(s,encoding="utf-8")
print("P56 account-switch confirmation applied:",len(pairs),"verified replacements")
