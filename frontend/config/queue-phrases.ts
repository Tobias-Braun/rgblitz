// Queue-position -> German phrase, shown while polling a queued/running
// RgxJob. Kept out of components so wording/brackets can be tuned without
// touching view code, mirroring config/mood-phrases.ts's threshold lookup.

export interface QueueThreshold {
  min: number;
  max: number;
  phrase: string;
}

export const QUEUE_THRESHOLDS: QueueThreshold[] = [
  { min: -Infinity, max: 0, phrase: "Wird gerade abgerufen..." },
  { min: 1, max: 1, phrase: "Fast geschafft, du bist gleich dran..." },
  { min: 2, max: 3, phrase: "Nur noch ein kleines Stück..." },
  { min: 4, max: 10, phrase: "Es dauert noch etwas, wir bleiben dran..." },
  { min: 11, max: Infinity, phrase: "Ganz schön was los — bitte hab noch etwas Geduld." },
];

export function queuePhrase(position: number): string {
  const bracket = QUEUE_THRESHOLDS.find(
    ({ min, max }) => position >= min && position <= max
  );
  if (!bracket) {
    throw new Error(`No queue bracket covers position ${position}`);
  }
  return bracket.phrase;
}
