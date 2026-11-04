# Konzept-Anhang: Schritt-Katalog

Stand: 2026-10-27. Ergänzt [konzept.md](konzept.md), Abschnitt „Wartungskatalog“. Startkatalog der
Basis-Schritte, Modul-Schritte und Intervalle als Grundlage für den mitgelieferten Katalog. In der App
haben Phasen, Baugruppen, Stufen und Wert-Schlüssel englische IDs (siehe konzept.md). Die Basis-Schritte
sind in `internal/app/spokebook/catalog/builtin.go` umgesetzt, ihre Texte stehen unter `catalog.<id>` in
`web/locales`.

## Basis-Schritte

53 Schritte als Startkatalog (die App liefert inzwischen einige mehr, siehe `builtin.go`), nach Phase und Baugruppe sortiert. Stufen: **V** = vor der Fahrt, **M** = monatlich, **K** = kleine Inspektion, **G** = große Inspektion. Grenzwerte sind gängige Richtwerte; die Herstellerangabe der jeweiligen Komponente hat immer Vorrang.

### 1 Aufnahme

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 1.1 | – | Kilometerstand & Probleme erfassen | Messwert km; Freitext Auffälligkeiten | – | K, G |
| 1.2 | Bremsen | Bremsen im Stand prüfen | Hebel ziehen, Rad schieben: greift, kein Durchziehen | Bremshebel | V, M, K, G |
| 1.3 | Steuersatz & Cockpit | Steuersatzspiel prüfen | Vorderbremse ziehen, Rad vor/zurück wippen: kein Klacken | Steuersatz | M, K, G |
| 1.4 | Laufräder & Reifen | Reifendruck & Achsen | Druck prüfen; Schnellspanner/Steckachse fest | Reifen · Steckachse | V, M, K, G |
| 1.5 | – | Geräusch-Check | Rad anheben, fallen lassen, Kurbel drehen: Klappern, Knacken notieren | – | K, G |

### 2 Reinigung

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 2.1 | Antrieb | Kette & Ritzel entfetten | Entfetter, Bürste; trocknen lassen | Kette, Kassette | K, G |
| 2.2 | – | Rad waschen | Von oben nach unten, kein harter Strahl auf Lager | Rahmen | K, G |
| 2.3 | Bremsen | Bremsflächen reinigen | Scheiben/Felgenflanken fettfrei (Isopropanol) | Bremsscheibe | G |

### 3 Prüfen & Messen

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 3.1 | Rahmen & Gabel | Rahmen & Gabel Sichtprüfung | Risse, Dellen, Lackabplatzer an Schweißnähten und Ausfallenden | Rahmen, Gabel | M, K, G |
| 3.2 | Rahmen & Gabel | Schaltauge prüfen | Gerade? (Schaltaugen-Lehre) | Schaltauge | G |
| 3.3 | Steuersatz & Cockpit | Lenker & Vorbau prüfen | Risse, Riefen an Klemmung, Lenker gerade zum Rad | Lenker, Vorbau | M, K, G |
| 3.4 | Steuersatz & Cockpit | Griffe/Lenkerband | Fest, Endkappen vorhanden | Griffe | K, G |
| 3.5 | Laufräder & Reifen | Reifen prüfen | Profil, Schnitte, Fremdkörper, Seitenwände | Reifen | M, K, G |
| 3.6 | Laufräder & Reifen | Rundlauf & Speichen | Seiten-/Höhenschlag; Speichenspannung mit zwei Fingern | Felge, Speichen | K, G |
| 3.7 | Laufräder & Reifen | Nabenlager prüfen | Spiel quer zur Achse, rauer Lauf | Nabe | K, G |
| 3.8 | Laufräder & Reifen | Felgenverschleiß (Felgenbremse) | Verschleißindikator / Rille | Felge | G |
| 3.9 | Laufräder & Reifen | Schlauch & Ventil | Ventil gerade, Kappe, kein Druckverlust | Schlauch | K, G |
| 3.10 | Bremsen | Belagstärke messen | Messwert mm; Grenzwert laut Hersteller | Bremsbelag | M, K, G |
| 3.11 | Bremsen | Scheibenstärke messen | Messwert mm; typ. Mindeststärke \~1,5 mm (Herstellerangabe) | Bremsscheibe | G |
| 3.12 | Bremsen | Züge/Leitungen prüfen | Knicke, Scheuerstellen, Leckage | Bremszug/-leitung | K, G |
| 3.13 | Antrieb | Kettenlängung messen | Messwert %; Richtwert tauschen ab 0,5 % (11/12-fach) bzw. 0,75 % (≤ 10-fach) | Kette | M, K, G |
| 3.14 | Antrieb | Ritzel & Kettenblätter | Haifischzähne, Kette rutscht durch | Kassette, Kettenblatt | G |
| 3.15 | Antrieb | Schaltzüge & Hüllen | Ausfransen, Rost, schwergängig | Schaltzug | K, G |
| 3.16 | Antrieb | Schaltwerk & Schaltröllchen | Lagerspiel, Verschleiß Röllchen | Schaltwerk | G |
| 3.17 | Tretlager, Kurbel & Pedale | Tretlagerspiel | Kurbel seitlich wippen; Laufgeräusch | Innenlager | K, G |
| 3.18 | Tretlager, Kurbel & Pedale | Pedale | Spiel, Lauf, Cleats/Bindung | Pedal | K, G |
| 3.19 | Sattel & Sattelstütze | Sattel & Stütze | Gestell, Klemmung, Mindesteinstecktiefe | Sattel, Sattelstütze | K, G |
| 3.20 | Elektrik & E-Antrieb | Beleuchtung | Vorder-/Rücklicht, Kabel, Ausrichtung | Licht | V, M, K, G |
| 3.21 | Anbauteile | Anbauteile prüfen | Schutzbleche, Gepäckträger, Ständer, Klingel, Reflektoren fest | Anbauteile | K, G |

### 4 Demontage & Tausch

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 4.1 | Antrieb | Kette tauschen (bei Bedarf) | Ergebnis „getauscht“ → neue Komponente | Kette · Kettenschloss | bei Bedarf |
| 4.2 | Antrieb | Kassette/Kettenblatt tauschen (bei Bedarf) | Kassettenabschlussring mit Drehmoment | Kassette · Abschlussring | bei Bedarf |
| 4.3 | Bremsen | Beläge tauschen (bei Bedarf) | Kolben zurückdrücken, Sicherungssplint | Bremsbelag | bei Bedarf |
| 4.4 | Antrieb | Züge/Hüllen erneuern (bei Bedarf) | Endkappen, Zugendhülsen | Schaltzug | bei Bedarf |
| 4.5 | Sattel & Sattelstütze | Sattelstütze ausbauen, reinigen, neu fetten | Bei Carbon: Paste statt Fett (Modul) | Sattelstütze · Sattelklemme | G |
| 4.6 | Laufräder & Reifen | Reifen tauschen (bei Bedarf) | Laufrichtung beachten | Reifen | bei Bedarf |

### 5 Einstellen & Schmieren

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 5.1 | Steuersatz & Cockpit | Steuersatz einstellen | Spielfrei, leichtgängig; danach Vorbau ausrichten | Steuersatz · Ahead-Kappe | bei Bedarf |
| 5.2 | Laufräder & Reifen | Laufrad zentrieren | Nur bei Schlag | Felge | bei Bedarf |
| 5.3 | Bremsen | Bremsen einstellen | Sattel zentrieren, schleiffrei; Hebelweg | Bremssattel · Sattelbefestigung | K, G |
| 5.4 | Antrieb | Schaltwerk einstellen | Anschläge, Zugspannung, B-Schraube | Schaltwerk · Befestigung | K, G |
| 5.5 | Antrieb | Umwerfer einstellen | Anschläge, Höhe, Winkel | Umwerfer | K, G |
| 5.6 | Antrieb | Kette schmieren | Öl auf Rollen, Überschuss abwischen | Kette | M, K, G |
| 5.7 | – | Gelenke & Zugeinführungen ölen | Schaltwerk-Gelenke, Bremshebelachsen | – | G |

### 6 Endkontrolle

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 6.1 | Steuersatz & Cockpit | Cockpit-Schrauben mit Drehmoment | Lenkerklemmung, Schaftklemmung, Bremshebel, Schalthebel | Vorbau, Lenker · alle Schraubstellen | M, K, G |
| 6.2 | Sattel & Sattelstütze | Sattelstützen- & Gestellklemmung | Drehmoment; Sattelstützenklemmung am Rahmen und Gestellklemmung des Sattels | Sattelklemme, Sattelstütze, Sattel | M, K, G |
| 6.3 | Tretlager, Kurbel & Pedale | Kurbel & Pedale anziehen | Kurbelschrauben, Kettenblattschrauben, Pedale | Kurbel, Pedal | K, G |
| 6.4 | Bremsen | Bremssattel & Scheiben-Schrauben | Drehmoment, Schraubensicherung laut Hersteller | Bremssattel, Bremsscheibe | K, G |
| 6.5 | Antrieb | Schaltwerk- & Umwerfer-Schrauben | Drehmoment der Befestigungs- und Zugklemmschrauben | Schaltwerk, Umwerfer | K, G |
| 6.6 | Laufräder & Reifen | Achsen & Reifendruck final | Steckachse Drehmoment; Druck nach Reifenangabe | Steckachse, Reifen | K, G |
| 6.7 | Anbauteile | Anbauteile-Schrauben | Flaschenhalter, Gepäckträger, Schutzbleche | Anbauteile | G |

### 7 Probefahrt & Dokumentation

| Nr. | Baugruppe | Schritt | Details / Ergebnis | Komponente · Schraubstelle | Stufen |
| --- | --- | --- | --- | --- | --- |
| 7.1 | Bremsen | Bremstest & Einbremsen | Mehrere Bremsungen; neue Beläge einbremsen | Bremse | K, G |
| 7.2 | Antrieb | Schaltung durchschalten | Alle Gänge unter Last, kein Springen | Schaltwerk | K, G |
| 7.3 | – | Geräusche & Fahrverhalten | Knacken, Klappern, Geradeauslauf | – | K, G |
| 7.4 | – | Wartung abschließen | Messwerte, getauschte Teile, nächster Termin speichern | – | K, G |

## Modul-Schritte

Jedes Modul nennt pro Schritt die Operation (**+** hinzufügen, **⇄** ersetzen, **ℹ** ergänzen, **–** entfällt) und den Einfügepunkt (Phase.Baugruppe).

### E-Bike

E-Bike-Inspektionen prüfen zusätzlich Akku, Display, Ladegerät, Verkabelung und spielen Software-Updates ein; Diagnose und Batteriemanagement-Test brauchen Werkstatt-Equipment ([linexo](https://www.linexo.de/index%2ephp/node/344)). Die erste Inspektion empfiehlt sich nach spätestens einem Jahr bzw. rund 2.000 km ([Fahrrad XXL via biketrailspfannenstiel](https://biketrailspfannenstiel.ch/auskunft/e-bike-inspektionsintervall/)).

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| E1 | ℹ | 2 Reinigung | Reinigung E-Bike-gerecht | Akku vorher entnehmen, kein Hochdruck auf Motor/Stecker | K, G |
| E2 | + | 3 · Elektrik | Akku Sichtprüfung | Gehäuse auf Risse/Verformung, Kontakte sauber und trocken | M, K, G |
| E3 | + | 3 · Elektrik | Akkuhalterung & Schloss | Sitzt spielfrei, Schloss funktioniert | K, G |
| E4 | + | 3 · Elektrik | Kabel & Stecker | Scheuerstellen, Steckverbindungen fest, Korrosion | K, G |
| E5 | + | 3 · Elektrik | Display & Bedieneinheit | Anzeige, Tasten, Halterung | K, G |
| E6 | + | 3 · Elektrik | Geschwindigkeitssensor | Abstand Sensor–Magnet, Halter fest | K, G |
| E7 | + | 3 · Elektrik | Diagnose & Software-Update | Fehlerspeicher auslesen, Update Motor/Akku/Display; Protokoll ablegen (Werkstatt) | G |
| E8 | + | 3 · Elektrik | Ladegerät & Kabel | Gehäuse, Kabel, Stecker | G |
| E9 | + | 6 · Elektrik | Motorbefestigung | Drehmoment laut Rahmen-/Motorhersteller | G |
| E10 | ℹ | 3 · Antrieb (3.13) | Kettenlängung häufiger prüfen | Höherer Verschleiß durch Motorkraft; Intervall verkürzen | M |
| E11 | + | 7 Probefahrt | Unterstützungsstufen testen | Alle Stufen, Schiebehilfe, Abschaltung bei 25 km/h | K, G |
| E12 | + | 7 Dokumentation | Akkudaten erfassen | Ladezyklen, Kapazität (falls Diagnose verfügbar) | G |

### Carbon

Carbon verträgt Überdrehen schlecht, deshalb ist der Drehmomentschlüssel Pflicht; an Klemmflächen gehört Carbon-Montagepaste statt Fett ([Cycle Tech Review](https://cycletechreview.com/2019/bikes-frames/looking-after-your-carbon-frame/), [bike-magazin](https://www.bike-magazin.de/en/care-or-lubricant/carbon-care-for-enthusiasts/)).

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| C1 | ℹ | alle Schritte mit Tag `klemmung` | Drehmomentschlüssel Pflicht | Drehmoment-Wert der Komponente anzeigen, nie nach Gefühl | alle |
| C2 | ℹ | alle Schritte mit Tag `carbon-kontakt` | Carbon-Montagepaste | Nur auf Klemmflächen, nie auf Gewinde | alle |
| C3 | ℹ | 1 Aufnahme | Montageständer richtig klemmen | An der Sattelstütze, nicht am Rahmenrohr | K, G |
| C4 | + | 3 · Rahmen | Hotspots prüfen | Steuerrohr, Tretlagerbereich, Ausfallenden, Sattelrohr-Schlitz, Unterrohr (Steinschlag) | M, K, G |
| C5 | + | 3 · Rahmen | Klopftest | Mit Münze abklopfen: hell = ok, dumpf = Fachprüfung | G |
| C6 | + | 3 · Rahmen | Zug-Scheuerstellen | Stellen, an denen Hüllen oder Kette am Rahmen reiben; Schutzfolie prüfen | K, G |
| C7 | + | 3 · Sattel | Mindesteinstecktiefe | Markierung an Stütze und Rahmen beachten | K, G |
| C8 | + | außerplanmäßig | Nach Sturz: Tiefenprüfung | Eigener Wartungstyp „Nach Sturz“ mit C4, C5, 3.3, 3.19 | bei Bedarf |

### Tubeless

Dichtmilch bleibt je nach Produkt und Klima etwa 2–7 Monate flüssig; prüfe alle 2–4 Monate und tausche zu Saisonbeginn ([Cycling Weekly](https://www.cyclingweekly.com/products/how-often-should-you-replace-tubeless-tyre-sealant), [bikeparts.com](https://team.bikeparts.com/tag/sealant)). Den Reifen alle 12–18 Monate ganz abnehmen und innen prüfen ([BikeRadar](https://bikeradar.com/advice/workshop/tubeless-tyre-maintenance)).

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| T1 | ⇄ 3.9 | 3 · Laufräder | Dichtmilch prüfen | Schütteln/hören oder Messstab durch Ventil; Ergebnis: ausreichend / nachfüllen / erneuern | M, K, G |
| T2 | + | 3 · Laufräder | Dichtmilch-Austritt | Reste an Reifenwand, Wulst, Ventil | K, G |
| T3 | + | 4 · Laufräder | Dichtmilch nachfüllen | Über Ventil mit Spritze; Menge laut Hersteller | bei Bedarf |
| T4 | + | 4 · Laufräder | Komplettwechsel | Reifen ab, alte Klumpen entfernen, Felgenband prüfen, neu befüllen | G (12–18 Monate) |
| T5 | + | 4 · Laufräder | Ventileinsatz reinigen/tauschen | Verklebter Ventileinsatz | G |
| T6 | ℹ | 6 · Laufräder (6.6) | Tubeless-Druck | Niedrigerer Druck möglich; Herstellerangabe Felge/Reifen beachten | V, M, K, G |
| T7 | + | außerplanmäßig | Nach CO₂-Reparatur | Dichtmilch zu Hause prüfen und ggf. erneuern | bei Bedarf |

### Federung

Gabel, Dämpfer und Federsattelstütze brauchen saubere Standrohre und intakte Abstreifer; den Service
von Tauchrohren und Luftkammer gibt der Hersteller in Betriebsstunden vor – dafür wäre später ein
Stundenzähler am Rad statt nur Kilometer nötig.

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| F1 | ℹ | 2 Reinigung | Reinigung federungsgerecht | Kein Wasserstrahl auf Dichtungen | K, G |
| F2 | + | 2 · Federung | Standrohre reinigen | Abwischen, Tropfen Federgabelöl an die Abstreifer | M, K, G |
| F3 | + | 3 · Federung | Abstreifer & Dichtungen | Ölringe, Schmutz, Kratzer an Standrohren | M, K, G |
| F4 | + | 3 · Federung | Buchsen & Lager | Spiel in Gabelbuchsen, Dämpferaufnahmen, Hinterbaulagern | K, G |
| F5 | + | 4 · Federung | Gabel & Dämpfer warten | Öl und Dichtungen nach Betriebsstunden laut Hersteller | bei Bedarf |
| F6 | + | 5 · Federung | Luftdruck & SAG | Messwert (Sollwert an Gabel/Dämpfer/Federsattelstütze) | M, K, G |
| F7 | + | 6 · Federung | Gabel- & Dämpferschrauben | Drehmoment an Gabel/Dämpfer/Federsattelstütze | K, G |

### Hydraulische Scheibenbremse

Hydraulikbremsen brauchen keine Zugeinstellung, dafür dichte Leitungen und einen festen Druckpunkt.
Bremsflüssigkeit altert; entlüftet wird nach Herstellerintervall (bei DOT meist jährlich) oder bei
schwammigem Druckpunkt. Mineralöl und DOT dürfen nie gemischt werden.

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| H1 | ⇄ 3.12 | 3 · Bremsen | Leitungen & Dichtheit | Leitungen, Anschlüsse, Sattel und Hebel auf Öl, Knicke, Scheuerstellen | K, G |
| H2 | + | 3 · Bremsen | Druckpunkt prüfen | Hebel mehrmals ziehen: fester Druckpunkt, wandert nicht zum Lenker | M, K, G |
| H3 | + | 4 · Bremsen | Bremse entlüften | Flüssigkeit laut Bremse (Mineralöl oder DOT); Intervall 12 Monate | bei Bedarf |
| H4 | ℹ | 4 · Bremsen (4.3) | Beläge bei Hydraulik | Kolben vorsichtig zurückdrücken; Hebel ohne Beläge oder Spacer nicht ziehen | – |

### Nabenschaltung

Getriebenaben sind gekapselt; statt Schaltwerk und Umwerfer gibt es nur die Zugspannung und einen
Ölwechsel bzw. ein Wartungsbad nach Herstellerangabe (z.B. Rohloff jährlich oder alle 5.000 km).

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| N1 | + | 3 · Antrieb | Getriebenabe prüfen | Ölaustritt, Drehmomentstütze und Achsmuttern fest | K, G |
| N2 | + | 4 · Antrieb | Nabenöl wechseln | Ölwechsel oder Wartungsbad laut Hersteller; Intervall 12 Monate oder 5.000 km | bei Bedarf |
| N3 | ⇄ 5.4 | 5 · Antrieb | Nabenschaltung einstellen | Zugspannung nach Markierung am Schaltgehäuse | K, G |
| N4 | – | 3.2, 3.16, 5.5, 6.5 | Schaltauge, Schaltwerk & Röllchen, Umwerfer, deren Schrauben | entfallen | – |

### Riemenantrieb

Ein Zahnriemen wird nicht geschmiert und nicht entfettet; er wird gespannt (Frequenz per App) und auf
Risse und fehlende Zähne geprüft, die Riemenscheiben auf Verschleiß.

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| R1 | ⇄ 2.1 | 2 · Antrieb | Riemen reinigen | Wasser und Bürste, kein Entfetter und kein Öl | K, G |
| R2 | ⇄ 3.13 | 3 · Antrieb | Riemen prüfen | Risse, fehlende oder abgenutzte Zähne, ausgefranste Kanten | M, K, G |
| R3 | ⇄ 3.14 | 3 · Antrieb | Riemenscheiben prüfen | Zahnform, Verschleiß, Schmutz in den Zahnlücken | G |
| R4 | ⇄ 4.1 | 4 · Antrieb | Riemen tauschen | Riemen nie knicken oder verdrehen | bei Bedarf |
| R5 | ⇄ 5.6 | 5 · Antrieb | Riemenspannung | Messwert Hz (Sollwert am Riemen) | K, G |

### Elektronische Schaltung

Elektronische Schaltungen (z.B. Di2, AXS) haben statt Zügen Akkus, Kabel oder Funk und eine
Firmware; eingestellt wird per Taste oder App.

| Nr. | Op | Phase · Baugruppe | Schritt | Details / Ergebnis | Stufen |
| --- | --- | --- | --- | --- | --- |
| S1 | ⇄ 3.15 | 3 · Antrieb | Schaltungsakku | Ladestand (Messwert %), Kontakte sauber | M, K, G |
| S2 | + | 3 · Antrieb | Kabel & Stecker | Stecker fest, Kabel ohne Scheuerstellen; bei Funk ausblenden | K, G |
| S3 | + | 3 · Antrieb | Firmware | Update per App | G |
| S4 | ℹ | 5 · Antrieb (5.4, 5.5) | Elektronisch einstellen | Per Taste oder App, keine Zugspannung | – |
| S5 | – | 4.4 | Züge/Hüllen erneuern | entfällt | – |

## Intervalle

Die Stufen ziehen Schritte aus dem Katalog; fällig wird eine Stufe nach Zeit **oder** Kilometern, je nachdem, was zuerst eintritt.

| Stufe | Kürzel | Intervall (Richtwert) | Quelle |
| --- | --- | --- | --- |
| Vor der Fahrt | V | jede Fahrt, 1–2 Minuten | [Canyon](https://canyon.com/en-fr/blog-content/advice/bike-maintenace-checklist/b20022025.html) |
| Monatscheck | M | monatlich; Schrauben prüfen, nicht überdrehen | [biketrailspfannenstiel](https://biketrailspfannenstiel.ch/auskunft/fahrrad-service-kosten/) |
| Kleine Inspektion | K | halbjährlich bei intensiver Nutzung | [linexo](https://www.linexo.de/magazin/pflege-wartung/fahrrad-inspektion-so-bleibt-ein-zweirad-fit-und-verkehrssicher) |
| Große Inspektion | G | mindestens 1× jährlich, v. a. nach der Winterpause | [linexo](https://www.linexo.de/magazin/pflege-wartung/fahrrad-inspektion-so-bleibt-ein-zweirad-fit-und-verkehrssicher) |
| E-Bike-Inspektion | G + Modul | spätestens nach 1 Jahr bzw. ca. 2.000 km | [biketrailspfannenstiel](https://biketrailspfannenstiel.ch/auskunft/e-bike-inspektionsintervall/) |
| Tubeless-Check | T1 | alle 2–4 Monate; Komplettwechsel 12–18 Monate | [Cycling Weekly](https://www.cyclingweekly.com/products/how-often-should-you-replace-tubeless-tyre-sealant), [BikeRadar](https://bikeradar.com/advice/workshop/tubeless-tyre-maintenance) |

Mitgeliefert sind: Monatscheck 1 Monat, kleine Inspektion 6 Monate oder 1.500 km, große Inspektion 12 Monate oder 3.000 km; „Dichtmilch prüfen“ hat ein eigenes Intervall von 1 Monat. Teile mit eigenem Intervall bekommen es über einen eigenen Schritt mit Intervall, nicht an der Stufe: mitgeliefert sind „Bremse entlüften“ (12 Monate) und „Nabenöl wechseln“ (12 Monate oder 5.000 km); den Federungsservice legt man selbst an.

## Quellen

- [BikeX: Fahrrad-Inspektion – Checkliste](https://www.bike-x.de/blog/fahrrad-inspektion-so-wird-dein-rad-wieder-fit/)
- [bike24: Reparatur & Pflege](https://www.bike24.de/themen/reparatur-pflege)
- [mobile.de: E-Bike-Wartung](https://www.mobile.de/magazin/artikel/elektromobilitaet/e-bike-wartung)
- [linexo: E-Bike Inspektion](https://www.linexo.de/index%2ephp/node/344)
- [linexo: Fahrrad-Inspektion](https://www.linexo.de/magazin/pflege-wartung/fahrrad-inspektion-so-bleibt-ein-zweirad-fit-und-verkehrssicher)
- [Bums on the Saddle: 30-Punkte-Check (M-Check)](https://blog.bumsonthesaddle.com/2018/12/24/30point-checkup-checklist)
- [Canyon: Bike maintenance checklist](https://canyon.com/en-fr/blog-content/advice/bike-maintenace-checklist/b20022025.html)
- [Cycling Weekly: Tubeless-Dichtmilch erneuern](https://www.cyclingweekly.com/products/how-often-should-you-replace-tubeless-tyre-sealant)
- [BikeRadar: Tubeless-Wartung](https://bikeradar.com/advice/workshop/tubeless-tyre-maintenance)
- [bikeparts.com: Sealant](https://team.bikeparts.com/tag/sealant)
- [Cycle Tech Review: Carbonrahmen pflegen](https://cycletechreview.com/2019/bikes-frames/looking-after-your-carbon-frame/)
- [bike-magazin: Carbon-Pflege](https://www.bike-magazin.de/en/care-or-lubricant/carbon-care-for-enthusiasts/)
- [ctyeh: Carbon-Montagepaste & Drehmoment](https://ctyeh.com/articles/1651?lang=en)
