<template>
  <div class="warpp-timeline">
    <div class="warpp-timeline__status">
      Run: <strong>{{ run.status || "—" }}</strong>
    </div>
    <div class="warpp-timeline__scroll">
      <table>
        <tbody>
          <tr v-for="(ev, i) in run.events" :key="i">
            <td class="warpp-timeline__type">{{ shortType(ev.type) }}</td>
            <td class="warpp-timeline__node">{{ ev.node_path || "" }}</td>
            <td class="warpp-timeline__msg">{{ ev.error || ev.message || "" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useWarppRun } from "@/stores/warppRun";

const run = useWarppRun();

function shortType(t: string): string {
  return t.replace("node_", "").replace("run_", "run:");
}
</script>

<style scoped>
.warpp-timeline {
  display: flex;
  flex-direction: column;
  height: 100%;
  font-size: 12px;
}
.warpp-timeline__status {
  padding: 6px 10px;
  border-bottom: 1px solid var(--halo-border, #2b3242);
}
.warpp-timeline__scroll {
  overflow: auto;
}
.warpp-timeline table {
  width: 100%;
  border-collapse: collapse;
}
.warpp-timeline td {
  padding: 3px 8px;
  border-bottom: 1px solid var(--halo-border, #222835);
  vertical-align: top;
}
.warpp-timeline__type {
  white-space: nowrap;
  opacity: 0.85;
}
.warpp-timeline__node {
  white-space: nowrap;
  opacity: 0.7;
}
</style>
