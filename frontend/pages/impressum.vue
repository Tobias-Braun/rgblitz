<script setup lang="ts"></script>

<template>
  <div class="page">
    <AppHeader />
    <main class="content">
      <NuxtLink to="/" class="back-link">← Zurück</NuxtLink>
      <h1 class="title">Impressum</h1>

      <section class="entry">
        <h2 class="question">Angaben gemäß § 5 TMG</h2>
        <p class="answer">
          Tobias Braun<br />
          c/o flexdienst – #21233<br />
          Kurt-Schumacher-Straße 76<br />
          67663 Kaiserslautern<br />
          Deutschland
        </p>
        <p class="answer">
          RGBlitz ist ein privates, nicht-kommerzielles Community-Projekt ohne
          Gewinnerzielungsabsicht.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Kontakt</h2>
        <p class="answer">
          E-Mail:
          <a href="mailto:tobiasmarkusbraun@gmail.com">tobiasmarkusbraun@gmail.com</a>
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Verantwortlich für den Inhalt nach § 55 Abs. 2 RStV</h2>
        <p class="answer">Tobias Braun (Anschrift wie oben)</p>
      </section>

      <section class="entry">
        <h2 class="question">Haftung für Inhalte</h2>
        <p class="answer">
          Als Diensteanbieter bin ich gemäß § 7 Abs. 1 TMG für eigene Inhalte
          auf diesen Seiten nach den allgemeinen Gesetzen verantwortlich. Nach
          §§ 8 bis 10 TMG bin ich als Diensteanbieter jedoch nicht
          verpflichtet, übermittelte oder gespeicherte fremde Informationen zu
          überwachen oder nach Umständen zu forschen, die auf eine
          rechtswidrige Tätigkeit hinweisen. Verpflichtungen zur Entfernung
          oder Sperrung der Nutzung von Informationen nach den allgemeinen
          Gesetzen bleiben hiervon unberührt. Eine diesbezügliche Haftung ist
          jedoch erst ab dem Zeitpunkt der Kenntnis einer konkreten
          Rechtsverletzung möglich. Bei Bekanntwerden von entsprechenden
          Rechtsverletzungen werde ich diese Inhalte umgehend entfernen.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Haftung für Links</h2>
        <p class="answer">
          Dieses Angebot enthält Links zu externen Websites Dritter (u. a.
          playerzone.roundnetgermany.de), auf deren Inhalte ich keinen
          Einfluss habe. Deshalb kann ich für diese fremden Inhalte auch keine
          Gewähr übernehmen. Für die Inhalte der verlinkten Seiten ist stets
          der jeweilige Anbieter oder Betreiber der Seiten verantwortlich. Bei
          Bekanntwerden von Rechtsverletzungen werde ich derartige Links
          umgehend entfernen.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Urheberrecht und Quelldaten</h2>
        <p class="answer">
          Die durch den Seitenbetreiber erstellten Inhalte und Werke (Design,
          Quellcode) auf diesen Seiten unterliegen dem deutschen
          Urheberrecht. RGBlitz ist kein offizielles Angebot des Deutschen
          Roundnet Verbands und steht in keiner Verbindung zu playerzone.roundnetgermany.de.
          Angezeigte Spieler-, Turnier- und Ranglistendaten stammen von dort und
          werden ausschließlich automatisiert ausgelesen und zu RGX-Werten
          weiterverarbeitet, um Spieler:innen eine Live-Übersicht ihres
          Turniertages zu bieten – siehe
          <NuxtLink to="/datenschutz">Datenschutzerklärung</NuxtLink>. Wer
          Rechte an den zugrunde liegenden Daten hält und deren Anzeige oder
          Weiterverarbeitung nicht wünscht, kann sich jederzeit über die oben
          genannte E-Mail-Adresse an mich wenden.
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
</style>
