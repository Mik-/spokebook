# Konzept: Spokebook (Fahrrad-Wartungs-App)

## Ziel

Eine self-hosted Web-App, mit der man die Wartung mehrerer Fahrräder dokumentiert. Den Kern bildet
ein Katalog geordneter Wartungsschritte mit Drehmomenten und Verschleißgrenzen, gefiltert nach
Wartungsstufe und ergänzt um Module für Merkmale des Rads. Eine Marktrecherche
im Oktober 2026 hat keine passende Lösung ergeben: Kommerzielle Apps (ProBikeGarage, WatchMy.bike,
VeloBuddy u.a.) zählen nur Kilometer aus Strava und erinnern an Termine. Die Open-Source-Projekte sind
entweder für Autos gebaut (LubeLogger) oder unausgereifte Hobby-MVPs. Keine dieser Lösungen kennt
Schrittpläne.

## Anforderungen

| # | Anforderung |
| --- | --- |
| K1 | Mobil und am Rechner nutzbar (responsive Web-App, installierbar als PWA) |
| K2 | Mehrere Fahrräder |
| K3 | Komponenten mit Kaufdatum, Händler, Preis, km-Stand beim Einbau, optional Austauschdatum oder max. Laufleistung |
| K4 | Ein Katalog geordneter Wartungsschritte mit Zusatzinfos (Drehmomente, Verschleißgrenzen), gefiltert nach Wartungsstufe |
| K5 | Wartung dokumentieren: Für ein Rad wird eine Stufe gewählt und die passenden Schritte werden abgearbeitet |
| K6 | Sicher im Internet betreibbar (self-hosted) |
| K7 | Open Source, Veröffentlichung geplant |
| K8 | Regelmäßiger Import der gefahrenen km (Datei-Upload, Garmin Connect; Komoot nur über GPX-Export). Weitere Quellen wie Strava, intervals.icu oder wanderer sind nicht geplant. |

## Entscheidungen

- **Kein Offline-Betrieb.** Die App wird serverseitig gerendert, es gibt keine SPA.
- **Nur SQLite**, aber hinter einer sauberen Abstraktion der Datenschicht. Postgres wird weder umgesetzt
  noch getestet, soll aber später nachrüstbar sein.
- **Einzelnutzer zuerst.** Trotzdem bekommt jede Tabelle von Anfang an eine `owner_id`.
- **Lizenz: AGPL-3.0** (Alternative wäre MIT).

## Stack

| Baustein | Wahl | Warum |
| --- | --- | --- |
| Sprache, HTTP | Go, `net/http` mit dem ServeMux-Routing der Standardbibliothek | Keine Framework-Abhängigkeit, leicht für Mitwirkende |
| UI | `html/template`, htmx, Pico CSS (beide vendored, feste Version) | Mobile-first ohne SPA, Auto-Escaping gegen XSS |
| Datenbank | SQLite, Zugriff über sqlc, Migrationen mit goose | Eine Datei, einfache Backups |
| Dev-Loop | air (Live-Reload) im DevContainer | siehe `.devcontainer/` |
| Auslieferung | Docker-Image auf distroless-Basis in der GitHub Container Registry, gebaut von der CI bei jedem Release (semantic-release), Compose-Beispiel im README | Self-Hoster erwarten Docker; TLS übernimmt ein Reverse-Proxy (Caddy, Traefik ...) |

SQLite-Treiber ist `modernc.org/sqlite` (reines Go, kein CGO), wegen des Cross-Compile.

htmx wird gezielt eingesetzt, wo ein Neuladen stört: Eine offene Wartung speichert jede Änderung
(Abhaken, Messwert, Notiz) sofort und aktualisiert nur die Statuszeile. Ohne JavaScript funktionieren
alle Formulare wie gewohnt. htmx läuft ohne Inline-Styles und ohne `eval`, passend zur strikten CSP.

## Datenmodell (Kern)

- **Bike**: Name, Typ, Kaufdaten, Notizen und eine Liste von **Maßen** (Werte der Art `info`, z.B.
  Sattelhöhe, Stack, Reach). Sie dokumentieren Geometrie und Sitzposition und erscheinen nie in einer
  Wartung; Fahrwerkseinstellungen wie Gabeldruck und SAG sind Werte von Gabel und Dämpfer.
- **km-Stand** (Zeitreihe pro Bike): Datum, km, Quelle (manuell, Import). Daraus wird die Laufleistung
  jeder Komponente berechnet. Ein km-Stand liegt nie vor der Inbetriebnahme des Rads oder unter dem
  Stand dabei. Manuelle km-Stände sind Anker; dazwischen zählen die importierten Fahrten (siehe
  „Import von Fahrten“).
- **Komponente**: Typ (Kette, Kassette, Bremsbelag ...), Hersteller, Modell, Kaufdatum, Händler, Preis,
  Währung, max. Laufleistung, Austauschdatum, Notizen und eine Liste von **Werten** (z.B. Drehmomente,
  Verschleißgrenzen).
- **Komponententyp**: Standard-Typen mit fester ID (übersetzt) und eigene Typen. Ein Typ hat
  **Standardwerte**; eine Komponente überschreibt sie über den Schlüssel mit der Herstellerangabe.
- **Wert**: Schlüssel, Bezeichnung, Art, von, bis, Einheit, Hinweis. Nur „von“ ist ein Minimum
  (Belagstärke min. 1,2 mm), nur „bis“ ein Maximum (Kettenlängung max. 0,5 %), beide ein Bereich
  (5–6 Nm), von = bis ein fester Wert. Der **Schlüssel** ist stabil (aus der Bezeichnung abgeleitet,
  mitgelieferte englisch wie `handlebar_clamp`). Die **Art** steuert das Verhalten: `torque`
  (Drehmoment, nur Anzeige mit Hinweis), `limit` (Grenzwert, genau eine Grenze, Messwert außerhalb =
  nicht ok), `target` (Sollbereich, Messwert innerhalb = ok) und `info` (Angabe, z.B. Reifenbreite,
  nur Anzeige, kommt nie in eine Wartung; eine einzelne Zahl ist der Wert selbst). Der **Hinweis** nennt z.B. das Medium
  (Carbonpaste, Schraubensicherung). Ein Vorbau hat mehrere Werte, je Schraubstelle einen.
- **Einbau**: Komponente, Bike, eingebaut am und bei km, ausgebaut am und bei km. Das ist eine eigene
  Tabelle, damit Teile zwischen Rädern wandern können (z.B. ein zweiter Laufradsatz) und die
  Laufleistung über alle Einsätze summiert wird. In allen Formularen (Einbauen, Ausbauen, Ersetzen,
  Ausmustern) ist der km-Stand optional: leer heißt Stand des Rads zum Datum, ein abweichender Wert
  wird als km-Stand gespeichert. Beim Ersetzen kommt wahlweise eine neue Komponente oder ein vorhandenes,
  nicht verbautes Teil (gleicher Typ zuerst) an die Stelle der alten. Steckt am Rad schon eine Komponente desselben Typs an derselben
  Position, warnt das Formular und baut erst nach Bestätigung ein (außer Typ „Sonstiges“).
- **Schritt** (im Wartungskatalog, siehe unten): Titel, Anleitung, Phase, Baugruppe, Aktion, Stufen,
  Position, eigene **Werte** und optional Komponententypen mit den Wertarten, die der Schritt zeigt.
  Beim Warten zeigt der Schritt zusätzlich die Werte dieser Arten der am Rad verbauten Komponenten
  dieser Typen (z.B. Drehmomente von Vorbau und Lenker).
- **Wartung** (Durchführung): Bike, Stufe, Datum, km, Notiz. Beim Start werden die Schritte der Stufe
  und die angekreuzten Schritte „bei Bedarf“ **als Kopie** übernommen, mit ihren eigenen Werten und den
  Werten der zu diesem Datum verbauten Komponenten ihrer Typen (mit Name und Position, z.B.
  links/rechts). Weitere Schritte lassen sich einer offenen Wartung hinzufügen. Die Schritte werden
  einzeln abgehakt, jeweils mit Notiz; Grenzwerte und Sollbereiche bekommen einen gemessenen Wert. So
  bleibt die Historie korrekt, wenn Katalog oder Komponenten später geändert werden. Werte außerhalb der
  Grenze werden markiert. Eine Wartung bleibt offen, bis sie abgeschlossen wird, und kann wieder
  geöffnet werden.
- **Anhänge**: Fotos und Belege (JPEG, PNG, WebP, GIF, PDF) als BLOB in der Datenbank. Ein Anhang
  gehört zu beliebig vielen Rädern und Komponenten (z. B. eine Rechnung für mehrere Teile) und wird
  gelöscht, wenn er zu nichts mehr gehört. Fotos werden auf 2000 px verkleinert, nach EXIF gedreht
  und als JPEG ohne Metadaten (kein Aufnahmeort) gespeichert, dazu eine Vorschau; PDFs bleiben
  unverändert und öffnen sich im PDF-Viewer des Browsers (eigene CSP ohne Skripte) oder werden
  heruntergeladen. Ein Foto pro Rad ist sein Titelbild (Startseite, Radseite); es lässt sich auch
  wieder abwählen, ohne den Anhang zu löschen. Die Sicherung ist ab Version 2 ein ZIP mit JSON und Dateien.
- **Fahrt** (Import-Eintrag): Quelle, externe ID, Schlüssel des Rads in der Quelle, Start, Ende, Tag,
  Strecke, Name, zugeordnetes Bike (leer = nicht zugeordnet), Status (aktiv, verworfen). Eindeutig über
  Quelle und externe ID.
- **Rad-Zuordnung**: Quelle, Schlüssel des Rads in der Quelle, angezeigter Name, Bike.

## Wartungskatalog

Statt einzelner Wartungspläne gibt es **einen Schritt-Katalog** pro Nutzer, sortiert nach **Phase**
(wann im Ablauf) und **Baugruppe** (wo am Rad), darin nach Position. So arbeiten auch
Werkstattchecklisten (u.a. die VSF-Inspektionscheckliste): feste Reihenfolge, Baugruppe für Baugruppe.
Den Startkatalog mit Quellen enthält der Anhang [konzept-schritte.md](konzept-schritte.md).

- **Phasen** (fest, in dieser Reihenfolge): Aufnahme (`intake`), Reinigung (`cleaning`), Prüfen &
  Messen (`inspection`), Demontage & Tausch (`replacement`), Einstellen & Schmieren (`adjustment`),
  Endkontrolle (`final_check`), Probefahrt & Dokumentation (`test_ride`). Was man vor der Reinigung
  hört und fühlt, ist danach oft weg; Risse und Verschleiß sieht man erst am sauberen Rad; nach dem
  Tausch wird eingestellt, zuletzt alles mit Drehmoment kontrolliert.
- **Baugruppen** (fest): Rahmen & Gabel (`frame_fork`), Steuersatz & Cockpit (`headset_cockpit`),
  Laufräder & Reifen (`wheels_tires`), Bremsen (`brakes`), Antrieb (`drivetrain`), Tretlager, Kurbel &
  Pedale (`bottom_bracket_cranks_pedals`), Sattel & Sattelstütze (`saddle_seatpost`), Federung
  (`suspension`), Elektrik & E-Antrieb (`electrics`), Anbauteile (`accessories`). Ein Schritt kann auch
  keine Baugruppe haben (z.B. „Rad waschen“). Leere Baugruppen werden ausgeblendet.
- **Aktion** (optional): Prüfen, Reinigen, Schmieren, Einstellen, Messen, Tauschen, Anziehen
  (`check`, `clean`, `lubricate`, `adjust`, `measure`, `replace`, `tighten`); später zum Filtern.
- **Stufen sind Filter, keine eigenen Listen**: vor der Fahrt (`pre_ride`), Monatscheck (`monthly`),
  kleine Inspektion (`minor`), große Inspektion (`major`). Ein Schritt gehört zu beliebig vielen Stufen;
  ohne Stufe gilt er als „bei Bedarf“ (z.B. Kette tauschen).
- **Folgeschritte**: Ein Schritt verweist auf Schritte, zu denen er führen kann, in zwei Arten.
  **Bei Befund**: Die Wartung bietet ihn als Button am Schritt an („+ Beläge tauschen“ unter
  „Belagstärke messen“), hervorgehoben, wenn ein Messwert des Schritts außerhalb der Grenze liegt;
  hinzugefügt wird er erst auf Klick. **Immer danach**: Er kommt automatisch mit, sobald der Schritt in
  eine Wartung kommt (Kette tauschen → Schaltwerk einstellen), auch über mehrere Stufen. Ein
  Folgeschritt, den ein Modul ersetzt, wird zum Ersatzschritt (bei Nabenschaltung „Nabenschaltung
  einstellen“). Der kopierte Schritt merkt sich, aus welchem Schritt derselben Wartung er kam.
  Versehentlich hinzugefügte Folgeschritte und nachträglich hinzugefügte Schritte lassen sich in der
  laufenden Wartung wieder entfernen, solange sie nicht abgehakt sind, samt dem, was sie mitgebracht
  haben; die Schritte der Stufe bleiben.
- **Offene Arbeit**: Ein Folgeschritt, der beim Abschließen nicht erledigt ist (z.B. keine Beläge da),
  bleibt am Rad als „offen seit …“ unter „Fällig“ stehen und wird beim Start der nächsten Wartung
  vorgewählt. „Nachholen“ startet eine Wartung nur mit ihm. Er ist erledigt, sobald eine spätere
  abgeschlossene Wartung den Schritt erledigt, oder wenn man ihn verwirft. Mitgelieferte Schritte haben
  sinnvolle Folgeschritte (Belagstärke → Beläge tauschen → Bremse einstellen, Kettenlängung → Kette
  tauschen → Schaltwerk einstellen, Reifen prüfen → Reifen tauschen …). Noch offen: Laufrad aus- und
  eingebaut → Bremse auf Schleifen prüfen; Beläge getauscht → einbremsen.
- **Startkatalog**: Die Basisschritte aus dem Anhang werden mitgeliefert (inzwischen um einige ergänzt;
  maßgeblich ist `catalog/builtin.go`) (ohne Richtwerte; Werte
  trägt man an Typ oder Komponente ein). Sie haben feste englische IDs (z.B. `chain_wear`) und werden
  wie die Standard-Komponententypen übersetzt, solange der Nutzer Titel oder Anleitung nicht ändert.
  Gelöschte mitgelieferte Schritte werden nur ausgeblendet, damit sie nicht wiederkommen; unter dem
  Katalog lassen sie sich wiederherstellen.

**Darstellung**: Der Katalog ist eine **Stufen-Matrix**: pro Phase eine Tabelle, darin je Baugruppe ein
Block mit einer Zeile pro Schritt (Titel, Aktion, Stufen, Intervall). Die vier Stufen stehen als feste
Plätze (Vor, Mon, Kle, Gro; hell = gehört zur Stufe), damit man Schritte auf einen Blick vergleicht; die
Phasenzeile zählt die Schritte je Stufe. Eine **Filterleiste** (Stufe, Modul, Baugruppe, Aktion, Suche;
ein GET-Formular, geht ohne JavaScript) schränkt die Tabelle ein. Jeder Schritt hat eine **Detailseite**
mit den Abschnitten Was (Aktion, Anleitung), Wo (Phase, Baugruppe), Wann (Stufen, eigenes Intervall),
Für welche Räder (Modul, ersetzt / ersetzt durch / entfällt bei, Hinweise) und Werte; das Formular ist
genauso gegliedert. Die **Modul-Übersicht** zeigt je Modul, was es hinzufügt, entfallen lässt und mit
Hinweisen ergänzt.

**Module** schalten sich über **Merkmale am Rad** zu (Checkboxen im Rad-Formular, z.B. Carbon-Rahmen,
Tubeless-Reifen), statt eigene Komplettlisten pro Radtyp zu pflegen. Module werden fest mitgeliefert;
ihre Schritte stehen im Katalog (markiert mit „nur bei …“) und lassen sich dort bearbeiten oder
ausblenden. Auch eigene Schritte können einem Merkmal zugeordnet werden. Ein Modul kann:

- Schritte **hinzufügen**: Sie kommen nur bei Rädern mit dem Merkmal in die Wartung.
- Basisschritte **ersetzen**: Tubeless ersetzt „Schlauch & Ventil“ durch „Dichtmilch prüfen“.
- Basisschritte **entfallen** lassen, ohne Ersatz: Nabenschaltung braucht weder Umwerfer noch
  Schaltröllchen.
- Schritte **ergänzen**: Ein Hinweis wird an Schritte gehängt, ausgewählt über die **Aktion** (Carbon:
  alle Schritte mit Aktion „Anziehen“ bekommen „Drehmomentschlüssel Pflicht, Carbonpaste nur auf
  Klemmflächen“) oder in Einzelfällen über einen bestimmten Schritt (Tubeless: Druckhinweis an
  „Achsen & Reifendruck final“). Tags sind dafür nicht nötig.

Merkmale wirken beim Start einer Wartung und beim Hinzufügen von Schritten; spätere Änderungen am Rad
verändern bestehende Wartungen nicht. Umgesetzt sind Carbon, Tubeless, E-Bike (mit den
Komponententypen Akku, Motor, Display, Bedieneinheit, Ladegerät, Geschwindigkeitssensor) und Federung
(Gabel, Dämpfer, Federsattelstütze; ohne Stundenzähler), hydraulische Scheibenbremse, Nabenschaltung
(Getriebenabe), Riemenantrieb (Riemen, Riemenscheibe) und elektronische Schaltung (Schaltungsakku).
Nabenschaltung und Riemen sind getrennte Merkmale, weil es Nabenschaltungen auch mit Kette gibt. Sobald
zwei Module denselben Schritt ersetzen, braucht es eine feste Priorität.

**Intervalle**: Eine Stufe (außer „vor der Fahrt“) wird nach Monaten oder Kilometern fällig, je
nachdem, was zuerst eintritt. Die Intervalle gelten pro Nutzer und sind auf einer eigenen Seite unter
dem Katalog einstellbar; ohne eigene Angabe gelten Monatscheck 1 Monat, kleine Inspektion 6 Monate
oder 1.500 km, große Inspektion 12 Monate oder 3.000 km. Gezählt wird ab der letzten
**abgeschlossenen** Wartung dieser oder einer **höheren** Stufe (deren Schritte enthalten die der
niedrigeren), sonst ab der Inbetriebnahme des Rads. Stufen ohne Schritte für das Rad entfallen.

Ein Schritt kann zusätzlich ein **eigenes Intervall** haben, unabhängig von seinen Stufen (z.B.
Federsattelstütze warten alle 500 km, Dichtmilch prüfen jeden Monat). Gezählt wird ab dem letzten
Abhaken in einer abgeschlossenen Wartung, in Kilometern des Rads. Ein Intervall direkt an der
Komponente (km der Komponente, auch über Radwechsel) ist zurückgestellt.

Ab 80 % des Intervalls gilt etwas als „bald fällig“, ab 100 % als „überfällig“. Die Rad-Seite zeigt
alle Stufen und Schritte mit Intervall, die Startseite das dringendste. Beim Start einer Wartung sind
die höchste fällige Stufe und die fälligen Schritte außerhalb dieser Stufe vorausgewählt.

## Datenschicht

Die Fachlogik spricht ausschließlich mit Repository-Interfaces (z.B. `BikeStore`, `ComponentStore`,
`MaintenanceStore`). Umgesetzt und getestet wird nur SQLite.

- Die Interfaces liegen im Paket `internal/app/spokebook/store`, die SQLite-Implementierung in `internal/app/spokebook/store/sqlite` (mit sqlc
  generiert). SQLite-Typen und -Fehler bleiben in der Implementierung, nach außen gehen nur eigene
  Fehler wie `store.ErrNotFound`.
- Welche Implementierung genutzt wird, entscheidet eine einzige Stelle beim Start anhand der
  Konfiguration (`APP_DB_PATH`).
- Migrationen laufen mit goose, sind ins Binary eingebettet und werden beim Start ausgeführt. Sie liegen
  in `internal/app/spokebook/store/sqlite/migrations`.
- Portable Typen: IDs als UUIDv7, Zeiten in UTC, Strecken in Metern, Geld in Cent als Ganzzahl plus
  Währung.
- Transaktionen laufen über eine Methode wie `WithTx(ctx, func(Stores) error)`, damit z.B. das
  Abschließen einer Wartung atomar ist.
- Die Tests laufen gegen die Interfaces mit SQLite (Datei bzw. In-Memory). Sie sind so geschrieben, dass
  eine zweite Implementierung sie später wiederverwenden kann.
- Postgres käme später als eigenes Paket `internal/app/spokebook/store/postgres` mit eigenem Query-Set und eigenen Migrationen.

## Import von Fahrten

Importiert werden nur Kilometer pro Rad, keine kompletten Aktivitäten und keine Tracks; hochgeladene
Dateien werden nach dem Auslesen verworfen. Jede Quelle ist ein Provider hinter einem gemeinsamen
Interface. Ein Hintergrundjob fragt die API-Provider einmal am Tag ab; „Jetzt abrufen“ startet ihn sofort.

**Berechneter km-Stand.** Fahrten werden nicht als km-Stände gespeichert, sondern als eigene Einträge
mit ihrer Strecke. Der km-Stand eines Rads an einem Tag ist der letzte manuelle km-Stand an oder vor
diesem Tag (ohne einen: der Stand bei Inbetriebnahme) plus die Strecken der Fahrten nach dessen Tag bis
einschließlich zum Tag. Ein manueller km-Stand gilt für das Ende seines Tages und **gewinnt**: Fahrten
davor verteilen nur km innerhalb des Abschnitts bis zu ihm. Widersprechen die Fahrten einem späteren
manuellen Stand (mehr km als dessen Differenz), bleibt der manuelle Stand gültig; der berechnete Stand
liegt nie über dem nächsten manuellen Stand, so zählt er nur aufwärts. Einbauten, deren km-Stand leer
blieb, folgen dem berechneten Stand ihres Datums (auch nach späteren Importen); eingetragene bleiben fest. Ein Rad zu ändern, eine
Fahrt zu verwerfen oder nachzuimportieren ändert deshalb nur eine Zeile; Laufleistung der Komponenten
und Fälligkeiten rechnen sich neu. Abgeschlossene Wartungen behalten ihren km-Stand (Kopie).

**Tag einer Fahrt**: das lokale Datum des Starts (FIT liefert die lokale Zeit, sonst die Zeitzone des
Servers). Fahrten vor der Inbetriebnahme des Rads werden nicht zugeordnet.

**Doppelte Fahrten.** Ist Quelle und externe ID schon bekannt, wird die Fahrt übersprungen; ihre
Zuordnung bleibt. Überschneidet sich eine neue Fahrt zeitlich mit einer vorhandenen (z.B. dieselbe Fahrt
als FIT vom Garmin und als GPX von Komoot), wird sie als mögliches Duplikat markiert und ist zum
Überspringen vorausgewählt. Verworfene Fahrten bleiben mit Status „verworfen“ gespeichert, damit ein
erneuter Import sie nicht wieder anlegt.

| Format | Externe ID | Strecke | Schlüssel des Rads |
| --- | --- | --- | --- |
| FIT | `file_id` (Seriennummer + Erstellzeit) | `session.total_distance` | UUID der am Ende der Tour gewählten Garmin-Ausrüstung (undokumentierte Nachricht 526, Feld 0; gleich der UUID in Garmin Connect), sonst Name des Bike-Profils, sonst Aktivitätsprofil (`session.sport_profile_name`) mit Seriennummer, sonst Seriennummer des Geräts |
| TCX | Startzeit der Aktivität (`Id`) | Summe `DistanceMeters` der Runden | keiner |
| GPX | Zeit des ersten Trackpunkts | Summe der Abstände der Trackpunkte (Haversine) | keiner |
| Garmin Connect | wie FIT (die Originaldatei wird geladen) | wie FIT | wie FIT |

**Sportart.** Andere Sportarten werden übersprungen: in FIT alles außer Radfahren und E-Biking, in TCX
„Running“, in GPX ein Track-Typ wie `running` oder `hiking`. Indoor-Fahrten (FIT-Unterart Indoor, Spinning,
virtuell) und Aktivitäten mit unklarer Sportart (TCX „Other“, unbekannter GPX-Typ) zeigt die Vorschau
ohne Haken an; GPX ohne Typ (z.B. Komoot) gilt als Radfahrt.

**Rad-Zuordnung.** Über eine Tabelle pro Quelle „Schlüssel des Rads in der Quelle → Rad in der App“. Der
Name in der Quelle dient nur der Anzeige, so darf das Rad dort anders heißen. Das Rad einer Fahrt kommt
in dieser Reihenfolge aus: Auswahl in der Vorschau, Zuordnung über Garmin-Ausrüstung oder Bike-Profil, Zuordnung
über die Seriennummer, Standard-Rad des Uploads, sonst „nicht zugeordnet“. Wer in der Vorschau für einen
unbekannten Schlüssel ein Rad wählt, legt die Zuordnung an („merken“, vorausgewählt). Eine Zuordnung
lässt sich später ändern, wahlweise nur für künftige Fahrten oder auch für die bisherigen mit diesem
Schlüssel; von Hand zugeordnete Fahrten behalten ihr Rad.

**Datei-Upload.** Upload-Seite mit mehreren Dateien (FIT, GPX, TCX) und einem Standard-Rad, danach eine
Vorschau mit einer Zeile je Fahrt (Datum, km, Rad, Duplikat-Hinweis), dann Übernehmen. Eine Fahrtenliste
filtert nach Rad und „nicht zugeordnet“ und erlaubt „Rad ändern“, „Verwerfen“ und „Löschen“. Zuordnungen lassen sich löschen; ihre Fahrten behalten das Rad. Eine gelöschte Fahrt ist weg; ein erneuter Import ihrer
Datei legt sie wieder an.

| Quelle | Zugang | Einschätzung |
| --- | --- | --- |
| Datei-Upload | GPX, FIT, TCX | Funktioniert immer und dient als Rückfallebene für jede Quelle. |
| Endurain | Self-hosted (AGPL-3.0), synchronisiert Garmin Connect und Strava, REST-API unter `/api/v1` | Gestrichen, solange Garmin Connect reicht. Recherche dazu: API-Keys erlauben nur `activities:upload`, Lesen braucht den JWT-Login (Access-Token 15 min, Refresh-Token 7 Tage, wird bei jedem Refresh ersetzt, Wiederverwendung sperrt alle Tokens). Verbinden einmalig mit Benutzer, Passwort und ggf. MFA-Code; gespeichert wird nur das Refresh-Token (verschlüsselt). Der Job erneuert es bei jedem Lauf in einem Schritt mit dem Speichern; schlägt das fehl, zeigt die Quelle „neu verbinden“. Die API ändert sich noch (v0.20.0 hat viele Endpunkte umbenannt), deshalb gegen eine feste Version testen. |
| Strava | Offizielle API mit OAuth, jede Instanz registriert eine eigene API-App | Nicht geplant. Recherche dazu: neue Apps sind für 1 Athleten zugelassen, ohne Review für bis zu 10. Seit Juni 2026 braucht der Entwickler-Account ein Strava-Abo. Daten dürfen nur 7 Tage gecacht werden, deshalb nur km-Summen speichern oder den Gesamtstand des Rads (Gear) übernehmen. |
| Garmin Connect | Personal API (`personal.connectapi.garmin.com`, China `.cn`): jeder Nutzer erzeugt in Garmin Connect einen eigenen API-Key (Laufzeit 30 Tage bis 1 Jahr) und tauscht ihn gegen ein Access-Token (JWT als Text, 24 h, kein Refresh). Nur für die eigenen Daten, nicht für Dienste mit vielen Nutzern; die Business-API ist nur für freigeschaltete Firmen. | Umgesetzt, am 2026-10-31 mit einem echten Konto geprüft. Siehe „Garmin Connect“. |
| intervals.icu | Synchronisiert mit Garmin, API mit persönlichem API-Key (noch nicht verifiziert) | Nicht geplant. Wäre ein guter Sammelpunkt für Garmin und Wahoo. |
| Komoot | Keine offizielle API | Nicht anbinden. Stattdessen GPX-Export hochladen. |
| wanderer | Self-hosted (AGPL-3.0), hat eine API und eigene Import-Plugins für Strava und Komoot | Nicht geplant. Die API-Doku müsste vor einer Umsetzung geprüft werden. |

Umgesetzt sind Datei-Upload und Garmin Connect. Strava, intervals.icu und wanderer sind nicht geplant;
Endurain ist gestrichen, solange Garmin Connect direkt reicht.

### Garmin Connect

- **Verbinden**: API-Key und Startdatum eingeben (vorbelegt mit dem Tag der neuesten Fahrt, ohne Fahrten
  mit der frühesten Inbetriebnahme der Räder). Die Seite zeigt, bis zu welcher Fahrt schon abgerufen wurde;
  ein früheres Startdatum holt die Zeit davor nach.
  Der Key wird mit `APP_SECRET_KEY` verschlüsselt gespeichert, ebenso das Access-Token, das bis kurz vor
  Ablauf wiederverwendet wird. Ohne `APP_SECRET_KEY` ist die Anbindung abgeschaltet. Schlägt das Token mit
  401/403 fehl (Key abgelaufen oder gelöscht), zeigt die Quelle „neuen Key eintragen“.
- **Abruf**: `GET /activitylist-service/activities/search/activities` mit `activityType=cycling` (umfasst
  Unterarten wie `e_bike_fitness`), `startDate`, `sortOrder=asc`, seitenweise. Die Liste hat kein Gear;
  deshalb wird für jede neue Aktivität `GET /download-service/files/activity/{id}` geladen (ZIP mit der
  FIT-Datei) und wie ein FIT-Upload gelesen. Die Fahrten tragen die Quelle `fit`; so erkennt der Abruf
  Fahrten wieder, die schon als Datei hochgeladen wurden, und nutzt dieselben Zuordnungen (`gear:<uuid>`).
- **Übernahme ohne Vorschau**: bekannte Fahrten und zeitliche Überschneidungen werden übersprungen, ebenso
  Indoor-Fahrten und unklare Sportarten. Das Rad kommt aus der Zuordnung; ein unbekanntes Gear legt eine
  Zuordnung ohne Rad an, die Fahrten bleiben „nicht zugeordnet“, bis man sie zuordnet (samt bisheriger
  Fahrten).
- **Fortschritt**: Bearbeitete Aktivitäts-IDs werden gemerkt, auch übersprungene; eine gelöschte Fahrt holt
  der Abruf deshalb nicht wieder (Datei-Upload geht weiter). Jeder Lauf schaut 7 Tage vor die neueste
  bearbeitete Aktivität zurück, damit später synchronisierte Fahrten nicht fehlen.
- **Rate-Limit**: Garmin kann ein Limit durchsetzen, nennt aber keine Zahlen und sendet keine
  Limit-Header. Deshalb ein Abruf pro Tag (stündlich, solange ein Lauf Aktivitäten übrig gelassen hat),
  manuelle Abrufe frühestens 5 Minuten nach dem letzten, mindestens 2 s zwischen zwei Anfragen, höchstens
  100 Downloads pro Lauf (ein Erstimport läuft über mehrere Läufe) und Abbruch bei 429 oder einem anderen
  Fehler; der nächste Lauf macht weiter. Das Zugriffstoken wird bis eine Stunde vor Ablauf wiederverwendet.

## Sicherheit

- **Login**: Passkeys (WebAuthn) als Hauptweg, Passwort mit argon2id als Fallback, später optional OIDC.
  In der Entwicklung immer über `http://localhost:8080` aufrufen (nicht über 127.0.0.1), sonst
  funktionieren die Passkeys nicht.
- **Sessions**: serverseitig gespeichert, Cookies `HttpOnly`, `Secure`, `SameSite=Lax`. CSRF-Schutz über
  `http.CrossOriginProtection` (Go 1.25+).
- **Header**: strikte Content-Security-Policy ohne Inline-Skripte, HSTS, Rate-Limit und Sperre bei
  fehlgeschlagenen Logins.
- **Uploads**: Größenlimit, Typprüfung anhand des Inhalts, Ablage außerhalb des Webroots, Auslieferung
  nur nach Berechtigungsprüfung.
- **Betrieb**: TLS über einen Reverse-Proxy, Container läuft nicht als root, golangci-lint (mit
  `gosec`) und `govulncheck` in der CI, dokumentiertes Backup.
- Zugangsdaten von Fahrtenquellen (Garmin Connect) werden mit AES-256-GCM verschlüsselt gespeichert
  (Schlüssel aus `APP_SECRET_KEY`, nicht in der Datenbank) und nie geloggt. Sie gehören nicht ins Backup.

## Fahrplan

1. **v0.1 (MVP, umgesetzt)**: Login, Bikes, km-Stände, Komponenten mit Einbauten, Wartungskatalog mit Phasen,
   Baugruppen und Stufen samt Startkatalog, Wartung durchführen und abhaken.
2. **v0.2 (umgesetzt)**: Fälligkeitsanzeige nach km und Datum, Anhänge, PWA-Installation (ohne Offline-Modus,
   nur eine Offline-Seite), Import aus GPX-, FIT- und TCX-Dateien und aus Garmin Connect (Personal
   API).
3. **v0.3 (umgesetzt)**: Weitere Module über Merkmale des Rads (nach E-Bike und Federung: Hydraulikbremse,
   Nabenschaltung, Riemenantrieb, elektronische Schaltung); gelöschte Komponententypen
   wiederherstellen, Radfoto entfernen ohne Löschen, PDFs im Browser ansehen, Garmin Connect
   verbinden und trennen ohne auf einen laufenden Abruf zu warten; einfache Einkaufsliste
   (Freitext, abhaken, Gekaufte entfernen; Einträge auch aus einer offenen Wartung und als
   „Nachkaufen“ an einer Komponente).
4. **Später**: Modulverwaltung im UI (eigene Module pro Nutzer, eigene Schritte daran, ggf. eigene
   Hinweise), OIDC, CSV-Export (die Sicherung deckt den Export vorerst
   ab), weitere Nutzer einladen, öffentliche API, Postgres-Implementierung bei Bedarf.
5. **Nicht geplant**: Import über Strava, intervals.icu und wanderer; Import aus Endurain (gestrichen,
   solange Garmin Connect direkt reicht). Datei-Upload und Garmin Connect decken die Fahrten ab.
