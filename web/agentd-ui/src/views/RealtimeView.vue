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

        <div class="flex min-w-0 flex-col gap-3 sm:w-[320px]">
          <div class="flex flex-col gap-2">
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

          <div class="flex flex-col gap-2">
            <label
              for="realtime-responder"
              class="font-mono text-[10px] uppercase tracking-[0.12em] text-subtle-foreground"
            >
              Responder
            </label>
            <select
              id="realtime-responder"
              class="halo-focus min-h-9 w-full rounded-md border border-border bg-input px-3 text-sm text-foreground disabled:cursor-not-allowed disabled:opacity-60"
              :value="realtime.selectedResponder.value"
              :disabled="
                !realtime.activeSessionId.value ||
                realtime.callActive.value ||
                realtime.connecting.value ||
                realtime.respondersLoading.value ||
                realtime.responderUpdating.value
              "
              @change="selectResponder"
            >
              <option value="specialist:orchestrator">Main orchestrator</option>
              <optgroup
                v-if="realtime.availableTeams.value.length"
                label="Teams"
              >
                <option
                  v-for="team in realtime.availableTeams.value"
                  :key="`team:${team.name}`"
                  :value="`team:${team.name}`"
                >
                  {{ team.name }}
                </option>
              </optgroup>
              <optgroup
                v-if="realtime.availableSpecialists.value.length"
                label="Specialists"
              >
                <option
                  v-for="specialist in realtime.availableSpecialists.value"
                  :key="`specialist:${specialist.name}`"
                  :value="`specialist:${specialist.name}`"
                >
                  {{ specialist.name }}
                </option>
              </optgroup>
              <option
                v-if="responderUnavailable"
                :value="realtime.selectedResponder.value"
              >
                {{ realtime.selectedResponderLabel.value }} (unavailable)
              </option>
            </select>
            <p v-if="realtime.responderError.value" class="text-xs text-danger">
              {{ realtime.responderError.value }}
            </p>
            <p v-else class="text-xs text-subtle-foreground">
              Answers every voice turn in this conversation.
            </p>
          </div>
        </div>
      </section>

      <section
        class="halo-surface-2 grid gap-4 p-4 md:grid-cols-[minmax(0,1.4fr)_minmax(180px,0.8fr)_auto] md:items-end"
        aria-label="Realtime audio setup"
      >
        <div class="min-w-0">
          <label
            for="realtime-microphone"
            class="font-mono text-[10px] uppercase tracking-[0.12em] text-subtle-foreground"
          >
            Microphone
          </label>
          <select
            id="realtime-microphone"
            class="halo-focus mt-2 min-h-9 w-full rounded-md border border-border bg-input px-3 text-sm text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            :value="realtime.audioSettings.value.inputDeviceId"
            :disabled="realtime.callActive.value || realtime.connecting.value"
            @change="selectInputDevice"
          >
            <option value="">System microphone</option>
            <option
              v-for="(device, index) in realtime.audioInputs.value"
              :key="device.deviceId || `microphone-${index}`"
              :value="device.deviceId"
            >
              {{ device.label || `Microphone ${index + 1}` }}
            </option>
          </select>
        </div>

        <div>
          <label
            for="realtime-noise-mode"
            class="font-mono text-[10px] uppercase tracking-[0.12em] text-subtle-foreground"
          >
            Noise suppression
          </label>
          <select
            id="realtime-noise-mode"
            class="halo-focus mt-2 min-h-9 w-full rounded-md border border-border bg-input px-3 text-sm text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            :value="realtime.audioSettings.value.suppressionMode"
            :disabled="realtime.callActive.value || realtime.connecting.value"
            @change="selectSuppressionMode"
          >
            <option value="automatic">Automatic</option>
            <option value="standard">Standard</option>
            <option value="strong">Strong</option>
          </select>
        </div>

        <div class="flex flex-wrap items-center gap-3 md:justify-end">
          <label
            class="halo-focus flex min-h-9 cursor-pointer items-center gap-2 rounded-md border border-border bg-input px-3 text-xs text-muted-foreground"
            :class="{
              'cursor-not-allowed opacity-60':
                realtime.callActive.value || realtime.connecting.value,
            }"
          >
            <input
              type="checkbox"
              class="h-3.5 w-3.5 accent-[rgb(var(--color-accent))]"
              :checked="realtime.audioSettings.value.autoGainControl"
              :disabled="realtime.callActive.value || realtime.connecting.value"
              @change="toggleAutoGain"
            />
            Auto gain
          </label>
          <AppButton
            v-if="realtime.callActive.value"
            size="sm"
            variant="neutral"
            :loading="realtime.calibrating.value"
            :disabled="realtime.phase.value !== 'listening'"
            @click="realtime.calibrateRoomNoise"
          >
            Calibrate room
          </AppButton>
        </div>

        <p
          class="text-xs leading-5 text-muted-foreground md:col-span-3"
          aria-live="polite"
        >
          {{ suppressionDescription }}
          <span
            v-if="realtime.captureCapabilityWarning.value"
            class="ml-1 text-warning"
          >
            {{ realtime.captureCapabilityWarning.value }}
          </span>
        </p>
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
            v-if="realtime.calibrating.value"
            class="mt-5 w-64 max-w-full"
            aria-label="Room noise calibration progress"
          >
            <div class="h-1.5 overflow-hidden rounded-full bg-surface-muted">
              <div
                class="h-full rounded-full bg-accent transition-[width] duration-100"
                :style="{
                  width: `${Math.round(realtime.calibrationProgress.value * 100)}%`,
                }"
              ></div>
            </div>
          </div>

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
          <div class="my-1 h-px bg-border"></div>
          <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2">
            <dt class="text-subtle-foreground">Input</dt>
            <dd class="truncate text-right text-foreground">
              {{ realtime.selectedInputLabel.value }}
            </dd>
            <dt class="text-subtle-foreground">Responder</dt>
            <dd class="truncate text-right text-foreground">
              {{ realtime.selectedResponderLabel.value }}
            </dd>
            <dt class="text-subtle-foreground">Isolation</dt>
            <dd class="text-right text-foreground">
              {{ voiceIsolationLabel }}
            </dd>
            <dt class="text-subtle-foreground">RNNoise</dt>
            <dd class="text-right text-foreground">
              {{ denoiserLabel }}
            </dd>
            <template v-if="realtime.callActive.value">
              <dt class="text-subtle-foreground">Direction</dt>
              <dd class="text-right text-foreground">
                {{
                  realtime.audioMetrics.value.beamforming
                    ? "Array focus"
                    : "Selected mic"
                }}
              </dd>
              <dt class="text-subtle-foreground">Voice</dt>
              <dd class="text-right font-mono text-foreground">
                {{
                  Math.round(
                    realtime.audioMetrics.value.speechProbability * 100,
                  )
                }}%
              </dd>
              <dt class="text-subtle-foreground">SNR</dt>
              <dd class="text-right font-mono text-foreground">
                {{ realtime.audioMetrics.value.snrDb.toFixed(1) }} dB
              </dd>
              <dt class="text-subtle-foreground">Noise blocked</dt>
              <dd class="text-right font-mono text-foreground">
                {{ realtime.audioMetrics.value.rejectedNoiseEvents }}
              </dd>
            </template>
          </dl>
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
import type { NoiseSuppressionMode } from "@/lib/realtime/settings";

const realtime = useRealtimeConversation();

const responderUnavailable = computed(() => {
  const target = realtime.selectedResponder.value;
  if (target === "specialist:orchestrator") return false;
  if (target.startsWith("team:")) {
    return !realtime.availableTeams.value.some(
      (team) => `team:${team.name}` === target,
    );
  }
  return !realtime.availableSpecialists.value.some(
    (specialist) => `specialist:${specialist.name}` === target,
  );
});

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

const suppressionDescription = computed(() => {
  switch (realtime.audioSettings.value.suppressionMode) {
    case "strong":
      return "Strong uses local RNNoise and disables overlapping browser suppression.";
    case "standard":
      return "Standard uses the browser's built-in echo and noise processing.";
    default:
      return "Automatic prefers native voice isolation and activates RNNoise when isolation is unavailable.";
  }
});

const voiceIsolationLabel = computed(() => {
  const applied = realtime.appliedCaptureSettings.value?.voiceIsolation;
  if (applied === true) return "Active";
  if (!realtime.captureCapabilities.value.voiceIsolation) return "Unavailable";
  return realtime.callActive.value ? "Not applied" : "Supported";
});

const denoiserLabel = computed(() => {
  switch (realtime.denoiserStatus.value) {
    case "loading":
      return "Loading";
    case "active":
      return "Active";
    case "fallback":
      return "Bypassed";
    default:
      if (
        !realtime.callActive.value &&
        (realtime.audioSettings.value.suppressionMode === "strong" ||
          (realtime.audioSettings.value.suppressionMode === "automatic" &&
            !realtime.captureCapabilities.value.voiceIsolation))
      ) {
        return "On start";
      }
      return "Not needed";
  }
});

function selectConversation(event: Event) {
  const target = event.target as HTMLSelectElement;
  realtime.selectConversation(target.value);
}

function selectResponder(event: Event) {
  void realtime.setResponder((event.target as HTMLSelectElement).value);
}

function selectInputDevice(event: Event) {
  realtime.setInputDevice((event.target as HTMLSelectElement).value);
}

function selectSuppressionMode(event: Event) {
  realtime.setSuppressionMode(
    (event.target as HTMLSelectElement).value as NoiseSuppressionMode,
  );
}

function toggleAutoGain(event: Event) {
  realtime.setAutoGainControl((event.target as HTMLInputElement).checked);
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
