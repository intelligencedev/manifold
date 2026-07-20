import { apiClient } from "@/api/clientCore";

export async function transcribeRealtimeAudio(audio: Blob): Promise<string> {
  const form = new FormData();
  form.set("audio", audio, "realtime.wav");
  const { data } = await apiClient.post<{ text?: string }>("/stt", form, {
    // /stt is intentionally outside the /api namespace, but still uses the
    // application's shared authenticated Axios client.
    baseURL: "",
    timeout: 120_000,
  });
  return String(data?.text || "").trim();
}
