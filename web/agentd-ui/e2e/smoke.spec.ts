import { test, expect } from "@playwright/test";

test("renders the overview dashboard", async ({ page }) => {
  await page.goto("/overview");
  await expect(
    page.getByText("Agents, throughput, recent work, and queue operations."),
  ).toBeVisible();
});

test("renders the dedicated realtime voice view", async ({ page }) => {
  await page.goto("/realtime");
  await expect(
    page.getByRole("heading", { name: "Realtime voice" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Start conversation" }),
  ).toBeVisible();
  await expect(page.getByLabel("Microphone")).toBeVisible();
  await expect(page.getByLabel("Responder")).toHaveValue(
    "specialist:orchestrator",
  );
  await expect(page.getByLabel("Noise suppression")).toHaveValue("automatic");
  await expect(page.getByText("RNNoise", { exact: true })).toBeVisible();
});

test("runs the realtime capture worklet in Chromium", async ({ page }) => {
  await page.goto("/realtime");

  const result = await page.evaluate(async () => {
    const context = new AudioContext({ sampleRate: 48_000 });
    await context.audioWorklet.addModule("/realtime-capture-worklet.js");
    const capture = new AudioWorkletNode(context, "manifold-realtime-capture", {
      numberOfInputs: 1,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    });
    const oscillator = context.createOscillator();
    const inputGain = context.createGain();
    const silentOutput = context.createGain();
    oscillator.frequency.value = 180;
    inputGain.gain.value = 0.08;
    silentOutput.gain.value = 0;
    oscillator.connect(inputGain).connect(capture);
    capture.connect(silentOutput).connect(context.destination);

    const message = new Promise<{
      type: string;
      samples: Float32Array;
      sequence: number;
    }>((resolve, reject) => {
      const timeout = window.setTimeout(
        () => reject(new Error("Capture worklet did not emit audio")),
        2_000,
      );
      capture.port.onmessage = (event) => {
        window.clearTimeout(timeout);
        resolve(event.data);
      };
    });

    await context.resume();
    oscillator.start();
    const firstFrame = await message;
    oscillator.stop();
    capture.disconnect();
    await context.close();
    return {
      type: firstFrame.type,
      sampleLength: firstFrame.samples.length,
      sequence: firstFrame.sequence,
    };
  });

  expect(result).toEqual({
    type: "audio",
    sampleLength: 960,
    sequence: 0,
  });
});

test("starts RNNoise with a browser microphone stream", async ({ page }) => {
  await page.addInitScript(() => {
    const mediaDevices = navigator.mediaDevices;
    Object.defineProperty(mediaDevices, "getSupportedConstraints", {
      configurable: true,
      value: () => ({
        channelCount: true,
        deviceId: true,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
        voiceIsolation: false,
      }),
    });
    Object.defineProperty(mediaDevices, "enumerateDevices", {
      configurable: true,
      value: async () => [],
    });
    Object.defineProperty(mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        const context = new AudioContext({ sampleRate: 48_000 });
        const oscillator = context.createOscillator();
        const gain = context.createGain();
        const destination = context.createMediaStreamDestination();
        oscillator.frequency.value = 180;
        gain.gain.value = 0.08;
        oscillator.connect(gain).connect(destination);
        await context.resume();
        oscillator.start();
        return destination.stream;
      },
    });
  });
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === "GET" && pathname === "/api/setup/status") {
      await route.fulfill({ json: { ready: true } });
      return;
    }
    if (request.method() === "GET" && pathname === "/api/chat/sessions") {
      await route.fulfill({
        json: [
          {
            id: "realtime-e2e",
            name: "Realtime test",
            createdAt: "2026-07-20T00:00:00Z",
            updatedAt: "2026-07-20T00:00:00Z",
          },
        ],
      });
      return;
    }
    await route.fulfill({ json: [] });
  });
  await page.goto("/realtime");
  await expect(page.getByLabel("Conversation", { exact: true })).toContainText(
    "Realtime test",
  );

  await page.getByRole("button", { name: "Start conversation" }).click();
  const backendInformation = page.getByLabel(
    "Realtime privacy and backend information",
  );
  await expect(
    backendInformation.getByText("Active", { exact: true }),
  ).toBeVisible({
    timeout: 10_000,
  });
  await expect(
    page.getByRole("button", { name: "End conversation" }),
  ).toBeVisible();

  await page.getByRole("button", { name: "End conversation" }).click();
  await expect(
    page.getByRole("button", { name: "Start conversation" }),
  ).toBeVisible();
});
