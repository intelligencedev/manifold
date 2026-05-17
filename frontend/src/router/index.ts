import { createRouter, createWebHistory } from "vue-router";

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/fleet" },
    { path: "/fleet", component: () => import("@/views/FleetMapView.vue") },
    { path: "/intent", component: () => import("@/views/IntentConsoleView.vue") },
    { path: "/inbox", component: () => import("@/views/InterruptInboxView.vue") },
    { path: "/telemetry", component: () => import("@/views/TelemetryView.vue") },
    { path: "/memory", component: () => import("@/views/MemoryGardenView.vue") },
    { path: "/replay", component: () => import("@/views/ReplayView.vue") },
    { path: "/constitution", component: () => import("@/views/ConstitutionView.vue") },
    { path: "/settings", component: () => import("@/views/SettingsView.vue") },
  ],
});
