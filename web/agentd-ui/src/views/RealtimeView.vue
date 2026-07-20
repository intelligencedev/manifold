<template>
  <main class="h-full overflow-y-auto px-5 py-5 sm:px-7 sm:py-7">
    <div class="mx-auto flex min-h-full max-w-6xl flex-col gap-5">
      <section
        class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"
      >
        <div class="max-w-2xl">
          <div class="mb-2 flex items-center gap-2">
            <MBadge tone="data">Realtime</MBadge>
            <span class="font-mono text-[11px] text-muted-foreground">
              Moonshine STT · Supertonic TTS
            </span>
          </div>
          <h1 class="text-2xl font-semibold tracking-tight text-foreground">
            Realtime voice
          </h1>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">
            Have a continuous, private voice conversation with Manifold. Your
            turns are committed automatically when you pause, and speaking
            interrupts the current response.
          </p>
        </div>

        <div class="flex min-w-0 flex-col gap-2 sm:w-[280px]">
          <label
            for="realtime-conversation"
            class="font-mono text-[10px] uppercase tracking-[0.12em] text-subtle-foreground"
          >
            Conversation
          </label>
          <div class="flex gap-2">
            <select
              id="realtime-conversation"
              class="halo-focus min-h-9 min-w-0 flex-1 rounded-md border border-border bg-input px-3 text-sm text-foreground disabled:cursor-not-allowed disabled:opacity-60"
              :value="realtime.activeSessionId.value"
              :disabled="realtime.callActive.value"
              @change="selectConversation"
            >
              <option value="" disabled>Select a conversation</option>
              <option
                v-for="session in realtime.sessions.value"
                :key="session.id"
                :value="session.id"
              >
                {{ session.name }}
              </option>
            </select>
            <AppButton
              size="sm"
              variant="neutral"
              :disabled="realtime.callActive.value"
              aria-label="Create realtime conversation"
              title="Create realtime conversation"
              @click="realtime.createNewConversation"
            >
              New
            </AppButton>
          </div>
        </div>
      </section>

      <section
        class="realtime-stage halo-surface relative flex min-h-[440px] flex-1 flex-col items-center justify-center overflow-hidden px-5 py-10 text-center"
        :data-phase="realtime.phase.value"
      >
        <div class="realtime-grid" aria-hidden="true"></div>
        <div
          class="realtime-aura"
          :class="{
            'realtime-aura--active': realtime.callActive.value,
            'realtime-aura--speaking':
              realtime.phase.value === 'user-speaking' ||
              realtime.phase.value === 'assistant-speaking',
          }"
          :style="orbStyle"
          aria-hidden="true"
        ></div>

        <div class="relative z-10 flex max-w-xl flex-col items-center">
          <div
            class="realtime-orb grid h-28 w-28 place-items-center rounded-full border sm:h-32 sm:w-32"
            :class="orbClasses"
            :style="orbStyle"
          >
            <RealtimeIcon
              v-if="!realtime.callActive.value"
              class="h-11 w-11"
              aria-hidden="true"
            />
            <SolarMicrophone3Bold
              v-else-if="realtime.phase.value === 'user-speaking'"
              class="h-11 w-11"
              aria-hidden="true"
            />
            <span v-else class="realtime-wave" aria-hidden="true">
              <i></i><i></i><i></i><i></i><i></i>
            </span>
          </div>

          <p
            class="mt-7 font-mono text-[11px] uppercase tracking-[0.18em] text-accent"
          >
            {{ realtime.statusLabel.value }}
          </p>
          <p class="mt-3 max-w-lg text-sm leading-6 text-muted-foreground">
            {{ realtime.statusDetail.value }}
          </p>

          <div
            v-if="realtime.liveTranscript.value"
            class="mt-6 max-w-lg rounded-3 border border-border/80 bg-background/55 px-4 py-3 text-sm text-foreground shadow-1 backdrop-blur"
            aria-live="polite"
          >
            “{{ realtime.liveTranscript.value }}”
          </div>

          <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
            <AppButton
              v-if="!realtime.callActive.value"
              variant="accent"
              size="md"
              :loading="realtime.connecting.value"
              :disabled="!realtime.supported"
              @click="realtime.startCall"
            >
              <SolarMicrophone3Bold class="h-4 w-4" aria-hidden="true" />
              Start conversation
            </AppButton>

            <template v-else>
              <AppButton
                :variant="realtime.muted.value ? 'accent' : 'neutral'"
                size="md"
                :pressed="realtime.muted.value"
                :aria-label="
                  realtime.muted.value ? 'Unmute microphone' : 'Mute microphone'
                "
                @click="realtime.toggleMuted"
              >
                <SolarMicrophone3Bold class="h-4 w-4" aria-hidden="true" />
                {{ realtime.muted.value ? "Unmute" : "Mute" }}
              </AppButton>
              <AppButton
                v-if="
                  realtime.phase.value === 'thinking' ||
                  realtime.phase.value === 'assistant-speaking'
                "
                variant="neutral"
                size="md"
                @click="realtime.interruptAssistant"
              >
                <SolarPause class="h-4 w-4" aria-hidden="true" />
                Interrupt
              </AppButton>
              <AppButton variant="danger" size="md" @click="realtime.endCall()">
                <SolarStopBold class="h-4 w-4" aria-hidden="true" />
                End conversation
              </AppButton>
            </template>
          </div>
        </div>
      </section>

      <section class="grid gap-4 lg:grid-cols-[1fr_auto]">
        <div class="halo-surface min-h-[150px] p-4 sm:p-5">
          <div class="mb-4 flex items-center justify-between gap-3">
            <div>
              <p
                class="font-mono text-[10px] uppercase tracking-[0.12em] text-subtle-foreground"
              >
                Call transcript
              </p>
              <p class="mt-1 text-sm text-muted-foreground">
                The latest turns from this conversation
              </p>
            </div>
            <RouterLink
              :to="{ name: 'chat' }"
              class="halo-focus rounded-md px-2 py-1 text-xs font-medium text-accent hover:bg-surface-muted"
            >
              Open in Chat
            </RouterLink>
          </div>

          <div
            v-if="!realtime.recentMessages.value.length"
            class="rounded-3 border border-dashed border-border px-4 py-6 text-center text-sm text-muted-foreground"
          >
            Start speaking and your conversation will appear here.
          </div>
          <ol v-else class="space-y-3" aria-label="Recent voice turns">
            <li
              v-for="message in realtime.recentMessages.value"
              :key="message.id"
              class="flex gap-3"
            >
              <span
                class="mt-0.5 min-w-16 font-mono text-[10px] uppercase tracking-[0.1em]"
                :class="
                  message.role === 'user'
                    ? 'text-[rgb(var(--data))]'
                    : 'text-accent'
                "
              >
                {{ message.role === "user" ? "You" : "Manifold" }}
              </span>
              <p class="min-w-0 flex-1 text-sm leading-6 text-foreground">
                {{ message.content || (message.streaming ? "…" : "") }}
                <span
                  v-if="message.interrupted"
                  class="ml-2 font-mono text-[10px] uppercase tracking-wide text-warning"
                >
                  interrupted
                </span>
              </p>
            </li>
          </ol>
        </div>

        <aside
          class="halo-surface-2 flex min-w-[220px] flex-col justify-center gap-3 p-4 text-xs text-muted-foreground"
          aria-label="Realtime privacy and backend information"
        >
          <div class="flex items-center gap-2">
            <span class="h-2 w-2 rounded-full bg-success"></span>
            Audio stays on this Manifold host
          </div>
          <div class="flex items-center gap-2">
            <span class="h-2 w-2 rounded-full bg-accent"></span>
            WebGPU preferred automatically
          </div>
          <div class="flex items-center gap-2">
            <span class="h-2 w-2 rounded-full bg-warning"></span>
            CPU fallback remains available
          </div>
        </aside>
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import RealtimeIcon from "@/components/icons/Realtime.vue";
import SolarMicrophone3Bold from "@/components/icons/SolarMicrophone3Bold.vue";
import SolarPause from "@/components/icons/SolarPause.vue";
import SolarStopBold from "@/components/icons/SolarStopBold.vue";
import AppButton from "@/components/ui/AppButton.vue";
import MBadge from "@/components/ui/MBadge.vue";
import { useRealtimeConversation } from "@/composables/realtime/useRealtimeConversation";

const realtime = useRealtimeConversation();

const orbStyle = computed(() => ({
  "--voice-scale": String(1 + realtime.audioLevel.value * 0.16),
  "--voice-opacity": String(0.38 + realtime.audioLevel.value * 0.3),
}));

const orbClasses = computed(() => [
  realtime.callActive.value
    ? "border-accent/60 bg-[rgb(var(--color-accent)/0.14)] text-accent shadow-[0_0_42px_rgb(var(--color-accent)/0.2)]"
    : "border-border bg-surface-muted text-muted-foreground",
  realtime.phase.value === "error" ? "border-danger/60 text-danger" : "",
]);

function selectConversation(event: Event) {
  const target = event.target as HTMLSelectElement;
  realtime.selectConversation(target.value);
}
</script>

<style scoped>
.realtime-stage {
  isolation: isolate;
}

.realtime-grid {
  position: absolute;
  inset: 0;
  z-index: -2;
  opacity: 0.3;
  background-image:
    linear-gradient(rgb(var(--color-border) / 0.24) 1px, transparent 1px),
    linear-gradient(90deg, rgb(var(--color-border) / 0.24) 1px, transparent 1px);
  background-size: 38px 38px;
  mask-image: radial-gradient(circle at center, black, transparent 72%);
}

.realtime-aura {
  position: absolute;
  z-index: -1;
  width: min(68vw, 620px);
  aspect-ratio: 1;
  border-radius: 9999px;
  opacity: 0.35;
  transform: scale(var(--voice-scale, 1));
  background: radial-gradient(
    circle,
    rgb(var(--color-accent) / 0.22),
    rgb(var(--color-accent) / 0.06) 42%,
    transparent 70%
  );
  transition: transform 90ms linear;
}

.realtime-aura--active {
  animation: realtime-breathe 3.2s ease-in-out infinite;
}

.realtime-aura--speaking {
  opacity: var(--voice-opacity, 0.38);
}

.realtime-orb {
  transform: scale(var(--voice-scale, 1));
  transition:
    transform 90ms linear,
    color 180ms ease,
    border-color 180ms ease,
    background-color 180ms ease;
}

.realtime-wave {
  display: flex;
  height: 2.5rem;
  align-items: center;
  gap: 0.28rem;
}

.realtime-wave i {
  display: block;
  width: 0.22rem;
  height: 28%;
  border-radius: 9999px;
  background: currentColor;
  animation: realtime-wave 1.15s ease-in-out infinite;
}

.realtime-wave i:nth-child(2),
.realtime-wave i:nth-child(4) {
  animation-delay: -0.22s;
}

.realtime-wave i:nth-child(3) {
  animation-delay: -0.4s;
}

@keyframes realtime-breathe {
  0%,
  100% {
    filter: saturate(0.85);
  }
  50% {
    filter: saturate(1.25);
  }
}

@keyframes realtime-wave {
  0%,
  100% {
    height: 24%;
  }
  50% {
    height: 92%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .realtime-aura--active,
  .realtime-wave i {
    animation: none;
  }
}
</style>
