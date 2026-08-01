// Threshold brackets -> German phrase, per docs/02-ui-requirements.md
// §Mood Text Thresholds. Kept out of components so wording/brackets can be
// tuned without touching view code.

export interface MoodThreshold {
  min: number;
  max: number;
  phrase: string;
}

export const MOOD_THRESHOLDS: MoodThreshold[] = [
  {
    min: 40,
    max: Infinity,
    phrase:
      "Heute läuft alles du absolute:r Baller:in! Wir haben Justus Klapperich Bescheid gesagt, und er ist stolz auf dich!",
  },
  {
    min: 15,
    max: 39.9,
    phrase: "Starke Leistung. Der RGX geht steil nach oben, whoop whoop!",
  },
  {
    min: 5,
    max: 14.9,
    phrase:
      "Solider Tag. Ein paar Punkte mitgenommen, du bist auf dem richtigen Weg!",
  },
  {
    min: -4.9,
    max: 5,
    phrase:
      "Ausgeglichen. Du bist wohl genau da wo du hingehörst. Fun fact: Laura Kunzelmann hat dieses Ergebnis wenn sie im Turnier ein Spiel nur 2:1 gewinnt.",
  },
  {
    min: -14.9,
    max: -5,
    phrase:
      "Nicht der beste Tag. Aber zum Glück nicht so schlimm, von daher mach dir nichts draus.",
  },
  {
    min: -39.9,
    max: 15,
    phrase:
      "Heute war wohl nichts, aber im nächsten Turnier holste das wieder rein.",
  },
  {
    min: -Infinity,
    max: -40,
    phrase:
      "Auaaaa, das tut weh! Kopf hoch, es wird auch wieder besser. Und wenn nicht, dann ab zu Scott Beeks!",
  },
];

export function moodTextFor(totalDelta: number): string {
  const bracket = MOOD_THRESHOLDS.find(
    ({ min, max }) => totalDelta >= min && totalDelta <= max
  );
  if (!bracket) {
    throw new Error(`No mood bracket covers delta ${totalDelta}`);
  }
  return bracket.phrase;
}
