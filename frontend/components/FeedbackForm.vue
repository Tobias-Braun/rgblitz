<!-- In-page feedback form, swapped in for ResultView within pages/index.vue (see
     the "Single route" note there) so returning to the result is an instant
     state flip, not a navigation/re-fetch. -->
<script setup lang="ts">
import { ref } from "vue";
import type { ApiError, FeedbackRequest, FeedbackResponse } from "~/types/api";

const emit = defineEmits<{
  done: [];
}>();

const rating = ref(0);
const text = ref("");
const submitting = ref(false);
const submitted = ref(false);
const errorMsg = ref("");

const config = useRuntimeConfig();

async function submit() {
  if (rating.value < 1 || submitting.value) return;
  submitting.value = true;
  errorMsg.value = "";
  try {
    await $fetch<FeedbackResponse>("/api/feedback", {
      method: "POST",
      baseURL: config.public.apiBase,
      body: {
        rating: rating.value,
        text: text.value.trim() || undefined,
      } satisfies FeedbackRequest,
    });
    submitted.value = true;
  } catch (err) {
    const apiError = err as { data?: ApiError };
    errorMsg.value =
      apiError.data?.error ?? "Feedback konnte nicht gesendet werden.";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="feedback">
    <template v-if="!submitted">
      <h2 class="title">Dein Feedback</h2>
      <p class="prompt">
        Gib uns hier dein Feedback! Was klappt super? Was funktioniert nicht?
        Was wünscht du dir?
      </p>

      <div class="stars" role="radiogroup" aria-label="Bewertung in Sternen">
        <button
          v-for="n in 5"
          :key="n"
          type="button"
          class="star"
          :class="{ filled: n <= rating }"
          role="radio"
          :aria-checked="n === rating"
          :aria-label="`${n} von 5 Sternen`"
          @click="rating = n"
        >
          ★
        </button>
      </div>

      <textarea
        v-model="text"
        class="text-field"
        rows="5"
        maxlength="4000"
        placeholder="Deine Nachricht (optional)"
      />

      <p v-if="errorMsg" class="error-msg">{{ errorMsg }}</p>

      <button
        type="button"
        class="submit"
        :disabled="rating < 1 || submitting"
        @click="submit"
      >
        {{ submitting ? "Wird gesendet…" : "Feedback senden" }}
      </button>

      <button type="button" class="cancel" @click="emit('done')">
        Zurück zum Ergebnis
      </button>
    </template>

    <template v-else>
      <div class="success">
        <span class="icon">
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path
              d="M5 13l4 4L19 7"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </span>
        <p class="success-text">Danke für dein Feedback!</p>
        <button type="button" class="back" @click="emit('done')">
          Zurück zum Ergebnis
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.feedback {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 18px;
  padding: 24px 22px 40px;
}

.title {
  margin: 0;
  font: 600 20px/1.3 var(--font-display);
  color: var(--text);
  text-align: center;
}

.prompt {
  margin: 0;
  max-width: 420px;
  font: 400 15px/1.55 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
}

.stars {
  display: flex;
  gap: 8px;
}

.star {
  font-size: 34px;
  line-height: 1;
  color: var(--border-strong);
  transition: color 0.15s ease, transform 0.1s ease;
}

.star:active {
  transform: scale(0.92);
}

.star.filled {
  color: var(--amber-1);
}

.text-field {
  width: 100%;
  max-width: 420px;
  min-height: 120px;
  padding: 14px 16px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--text);
  font: 400 14px/1.5 var(--font-body);
  resize: vertical;
}

.text-field::placeholder {
  color: var(--text-dim);
}

.error-msg {
  margin: 0;
  font: 400 13px/1.5 var(--font-body);
  color: var(--red);
  text-align: center;
}

.submit {
  min-height: 44px;
  width: 100%;
  max-width: 260px;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid var(--purple);
  color: var(--purple);
  font: 600 15px/1 var(--font-body);
}

.submit:disabled {
  opacity: 0.6;
}

.cancel {
  min-height: 44px;
  font: 400 13px/1.4 var(--font-body);
  color: var(--text-muted);
}

.success {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.icon {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  border: 1px solid var(--green);
  background: color-mix(in srgb, var(--green) 12%, transparent);
  color: var(--green);
  display: flex;
  align-items: center;
  justify-content: center;
}

.icon svg {
  width: 26px;
  height: 26px;
}

.success-text {
  margin: 0;
  font: 400 16px/1.55 var(--font-body);
  color: rgba(244, 244, 242, 0.85);
  text-align: center;
}

.back {
  min-height: 44px;
  width: 100%;
  max-width: 260px;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid var(--green);
  color: var(--green);
  font: 600 15px/1 var(--font-body);
}
</style>
