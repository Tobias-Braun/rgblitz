<script setup lang="ts"></script>

<template>
  <div class="page">
    <AppHeader />
    <main class="content">
      <NuxtLink to="/" class="back-link">← Zurück</NuxtLink>
      <h1 class="title">Datenschutzerklärung</h1>

      <section class="entry">
        <h2 class="question">1. Verantwortlicher</h2>
        <p class="answer">
          Verantwortlicher im Sinne der Datenschutz-Grundverordnung (DSGVO)
          ist:
        </p>
        <p class="answer">
          Tobias Braun<br />
          c/o flexdienst – #21233<br />
          Kurt-Schumacher-Straße 74<br />
          67663 Kaiserslautern<br />
          Deutschland<br />
          E-Mail:
          <a href="mailto:tobiasmarkusbraun@gmail.com">tobiasmarkusbraun@gmail.com</a><br />
          (siehe auch <NuxtLink to="/impressum">Impressum</NuxtLink>)
        </p>
      </section>

      <section class="entry">
        <h2 class="question">2. Reichweitenmessung mit Umami</h2>
        <p class="answer">
          Zur anonymen statistischen Auswertung der Zugriffe auf RGBlitz wird
          Umami eingesetzt – eine selbst gehostete, quelloffene
          Analyse-Software, die auf einem eigenen Server betrieben wird. Es
          findet keine Datenübermittlung an Dritte oder an Umami als
          Unternehmen statt.
        </p>
        <p class="answer">
          Umami setzt keine Cookies und verwendet keine dauerhaften
          Identifikatoren. Erfasst werden lediglich aggregierte,
          anonymisierte Zugriffsdaten wie aufgerufene Seite, Referrer,
          Browser, Betriebssystem, Gerätetyp, Bildschirmauflösung sowie Land,
          Region und Stadt. Für die Ortsangabe wird deine IP-Adresse kurzzeitig
          mit einer lokal auf dem Server liegenden Geodatenbank abgeglichen;
          gespeichert wird nur der ermittelte Ort, nicht die IP-Adresse.
          Deine IP-Adresse wird dabei nicht gespeichert: Sie fließt lediglich
          kurzzeitig zusammen mit einem täglich wechselnden Zufallswert in
          eine Einweg-Hashfunktion ein, um Mehrfachbesuche innerhalb eines
          Tages zu erkennen, ohne dass daraus ein Personenbezug oder eine
          Nachverfolgung über mehrere Tage oder Seiten hinweg möglich ist. Es
          werden keine Nutzungsprofile erstellt und kein Verhalten
          geräte- oder seitenübergreifend nachverfolgt.
        </p>
        <p class="answer">
          Zusätzlich werden anonyme Interaktionen gezählt, z. B. die Auswahl
          eines Suchergebnisses (nur dessen Position und die Länge der
          Eingabe), die Art des angezeigten Ergebnisses mit grob gerundeter
          RGX-Änderung und Wartezeit, das Aufklappen von Details sowie das
          Absenden einer Feedback-Bewertung. Außerdem wird erfasst, ob der
          Energiesparmodus der Seite aktiv ist, ob dein Gerät reduzierte
          Bewegung bevorzugt und ob RGBlitz als installierte App geöffnet
          wurde. Namen von Spieler:innen, Spieler-IDs und Suchbegriffe werden
          dabei nie übermittelt.
        </p>
        <p class="answer">
          Rechtsgrundlage ist Art. 6 Abs. 1 lit. f DSGVO (berechtigtes
          Interesse an der Analyse der Nutzung meines Angebots, um es zu
          verstehen und zu verbessern). Da keine Cookies gesetzt und keine
          personenbezogenen Profile gebildet werden, ist keine Einwilligung
          nach § 25 TTDSG erforderlich.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">3. Technisch notwendige Verarbeitung beim Seitenaufruf</h2>
        <p class="answer">
          Wie bei jedem Webserver üblich, wird beim Aufruf dieser Seite deine
          IP-Adresse technisch benötigt, um die angeforderten Inhalte an
          deinen Browser ausliefern zu können (Art. 6 Abs. 1 lit. f DSGVO,
          berechtigtes Interesse am technischen Betrieb). Diese Daten werden
          von mir nicht gespeichert, protokolliert oder ausgewertet – sie
          fallen ausschließlich zur Dauer der Anfrage an.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">4. Eingabe deines Namens</h2>
        <p class="answer">
          Wenn du in der Suche einen Spielernamen eingibst, wird diese Eingabe
          verwendet, um beim Anbieter playerzone.roundnetgermany.de nach dem
          passenden, öffentlich einsehbaren Profil zu suchen. Deine Eingabe
          wird nicht dauerhaft gespeichert und nicht mit anderen Zwecken
          verknüpft.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">5. Verarbeitung öffentlich zugänglicher Playerzone-Daten</h2>
        <p class="answer">
          RGBlitz verarbeitet ausschließlich Daten, die auf
          playerzone.roundnetgermany.de öffentlich einsehbar sind (Spieler-
          und Teamnamen, Turnier- und Spielergebnisse, RGX-Ranglistenwerte).
          Diese Daten werden automatisiert ausgelesen und rein maschinell zu
          RGX-Differenzwerten für den laufenden Turniertag weiterverarbeitet.
        </p>
        <p class="answer">
          Ein Teil dieser Daten wird technisch zwischengespeichert, um die
          Playerzone nicht unnötig zu belasten und die Anwendung
          performant zu halten:
        </p>
        <ul>
          <li>
            Der wöchentliche Spielerindex (für die Namens-Autovervollständigung)
            wird einmal pro Woche neu aufgebaut.
          </li>
          <li>
            Turnierergebnisse eines laufenden Turniertages werden für maximal
            60 Sekunden zwischengespeichert und danach automatisch neu
            abgerufen; nach Turnierende für bis zu 24 Stunden.
          </li>
          <li>
            Ob ein Turnier zur German Roundnet Tour gehört, wird einmalig
            dauerhaft vermerkt, da sich das nach Turnierstart nicht mehr
            ändert.
          </li>
        </ul>
        <p class="answer">
          Diese Verarbeitung erfolgt ausschließlich zu dem Zweck, Spieler:innen
          während eines laufenden Turniers eine Live-Übersicht ihrer
          RGX-Veränderung als kostenlosen Community-Service zur Verfügung zu
          stellen (Art. 6 Abs. 1 lit. f DSGVO, berechtigtes Interesse an der
          Bereitstellung dieses Dienstes). Es findet keine Weitergabe an
          Dritte, keine Verknüpfung mit anderen Datenquellen und keine
          kommerzielle Nutzung statt.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">6. Widerspruch und Löschung durch Rechteinhaber</h2>
        <p class="answer">
          Sollten die auf playerzone.roundnetgermany.de öffentlich
          zugänglichen Daten Personen betreffen, die mit der Anzeige oder
          automatisierten Weiterverarbeitung durch RGBlitz nicht einverstanden
          sind, kann jederzeit formlos Kontakt über die oben genannte
          E-Mail-Adresse aufgenommen werden. Auf Wunsch der jeweils
          betroffenen bzw. datenverfügungsberechtigten Person werden
          betroffene Daten aus dem Zwischenspeicher entfernt und die
          automatisierte Verarbeitung für diese Person eingestellt.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">7. Deine Rechte als betroffene Person</h2>
        <p class="answer">Du hast nach der DSGVO folgende Rechte gegenüber mir als Verantwortlichem:</p>
        <ul>
          <li>Auskunft (Art. 15 DSGVO) über die zu deiner Person gespeicherten Daten.</li>
          <li>Berichtigung (Art. 16 DSGVO) unrichtiger oder unvollständiger Daten.</li>
          <li>Löschung (Art. 17 DSGVO), sofern keine gesetzlichen Aufbewahrungspflichten entgegenstehen.</li>
          <li>Einschränkung der Verarbeitung (Art. 18 DSGVO) unter bestimmten Voraussetzungen.</li>
          <li>Datenübertragbarkeit (Art. 20 DSGVO) in einem strukturierten, maschinenlesbaren Format.</li>
          <li>Widerspruch (Art. 21 DSGVO) gegen die Verarbeitung deiner Daten.</li>
        </ul>
        <p class="answer">
          Zur Ausübung deiner Rechte genügt eine formlose Nachricht an die oben
          genannte E-Mail-Adresse.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">8. Beschwerderecht bei einer Aufsichtsbehörde</h2>
        <p class="answer">
          Unbeschadet anderweitiger verwaltungsrechtlicher oder gerichtlicher
          Rechtsbehelfe steht dir das Recht auf Beschwerde bei einer
          Datenschutz-Aufsichtsbehörde zu, wenn du der Ansicht bist, dass die
          Verarbeitung deiner personenbezogenen Daten gegen die DSGVO
          verstößt.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">9. Aktualität dieser Datenschutzerklärung</h2>
        <p class="answer">
          Stand: September 2026. Diese Datenschutzerklärung kann bei Bedarf
          angepasst werden, um rechtlichen Anforderungen oder Änderungen am
          Dienst zu entsprechen.
        </p>
      </section>
    </main>
  </div>
</template>

<style scoped>
.page {
  flex: 1;
  min-height: 0;
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 22px;
  padding: 24px 18px 32px;
  padding-left: max(18px, calc((100% - var(--page-container-max-width)) / 2));
  padding-right: max(18px, calc((100% - var(--page-container-max-width)) / 2));
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.back-link {
  align-self: flex-start;
  font: 500 13px/1 var(--font-body);
  color: var(--text-dim);
  text-decoration: none;
}

.back-link:hover {
  color: var(--text-muted);
}

.title {
  font: 700 26px/1 var(--font-display);
  color: var(--text);
  letter-spacing: -0.02em;
  margin: 0;
}

.entry {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px 18px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}

.question {
  font: 600 15px/1.3 var(--font-display);
  color: var(--text);
  margin: 0;
}

.answer {
  font: 400 14px/1.55 var(--font-body);
  color: var(--text-muted);
  margin: 0;
}

.answer a {
  color: var(--text-muted);
  text-decoration: underline;
}

.answer a:hover {
  color: var(--text);
}

.entry ul {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 20px;
  margin: 0;
  list-style: disc;
  font: 400 14px/1.55 var(--font-body);
  color: var(--text-muted);
}
</style>
