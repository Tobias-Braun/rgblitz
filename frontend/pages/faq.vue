<script setup lang="ts"></script>

<template>
  <div class="page">
    <AppHeader />
    <main class="content">
      <NuxtLink to="/" class="back-link">← Zurück</NuxtLink>
      <h1 class="title">FAQ</h1>

      <section class="entry">
        <h2 class="question">Was ist RGBlitz?</h2>
        <p class="answer">
          RGBlitz ist ein inoffizielles Tool für Roundnet-Spieler:innen, das dir
          während eines laufenden Turniers live zeigt, wie sich dein RGX-Wert
          durch die gespielten Spiele verändert. Du gibst einfach deinen Namen
          ein und siehst pro Spiel, wie viele RGX-Punkte du gewonnen oder
          verloren hast – sowie deine Gesamtbilanz für den Tag.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Wie funktioniert das (woher kommen die Daten)?</h2>
        <p class="answer">
          RGBlitz führt keine eigene Ergebnisliste. Sobald du deinen Namen
          eingibst, schaut das Tool auf der PlayerZone
          (playerzone.roundnetgermany.de) nach, in welchem laufenden Turnier du
          gerade spielst, findet dein Team in der passenden Division und liest
          den Spielplan deines Teams aus. Für jedes bereits gespielte Spiel wird
          anschließend die RGX-Veränderung anhand der offiziellen RGX-Formel
          berechnet – auf Basis der RGX-Werte, die du und dein Gegner vor
          Turnierbeginn hattet.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Funktioniert RGBlitz auch für die Liga?</h2>
        <p class="answer">
          Ja, für die reguläre Saison der Deutschen Roundnet Liga. An einem
          Spieltag zeigt RGBlitz jede Partie, die du gespielt hast, als eigenes
          Spiel – mit deinem Partner bzw. deiner Partnerin und dem gegnerischen
          Paar. Die RGX-Veränderung wird wie bei Turnieren aus den RGX-Werten
          beider Paare vor dem Spieltag berechnet. Die Finalturniere im Frühjahr
          werden noch nicht unterstützt.
        </p>
      </section>

      <section class="entry">
        <h2 class="question">Warum gibt es eine Warteschlange?</h2>
        <p class="answer">
          Wir wollen die Playerzone nicht überlasten. Deshalb ruft RGBlitz im
          Hintergrund maximal eine Seite alle 2 Sekunden ab – und zwar für alle
          Nutzer:innen gemeinsam über eine einzige globale Warteschlange, nicht
          pro Person. Wenn also viele Leute gleichzeitig ihren RGX abrufen
          wollen, kann es sein, dass du kurz warten musst, bis du an der Reihe
          bist.
        </p>
        <p class="answer">
          Selbst bei sehr großen Turnieren bleibt die Wartezeit überschaubar:
          Beim bisher größten Turnier mit rund 400 Teams würde mit diesem Tempo
          jedes Team sein Ergebnis in unter 30 Minuten bekommen – selbst wenn
          alle 400 Teams genau gleichzeitig ihre Anfrage schicken (was ziemlich
          unrealistisch ist).
        </p>
      </section>

      <section class="entry">
        <h2 class="question">
          Wie kann ich einen Bug melden oder einen Feature-Wunsch äußern?
        </h2>
        <p class="answer">
          Am liebsten als Issue in unserem GitHub-Repository – das wird in Kürze
          öffentlich gemacht. Sobald es online ist, verlinken wir es hier
          direkt.
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
</style>
