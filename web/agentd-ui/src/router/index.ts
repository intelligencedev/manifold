import { createRouter, createWebHistory } from "vue-router";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/",
      name: "overview",
      component: () => import("@/views/OverviewView.vue"),
    },
    {
      path: "/projects",
      name: "projects",
      component: () => import("@/views/ProjectsView.vue"),
    },
    {
      path: "/specialists",
      name: "specialists",
      component: () => import("@/views/SpecialistsView.vue"),
    },
    {
      path: "/chat",
      name: "chat",
      component: () => import("@/views/ChatView.vue"),
    },
    {
      path: "/flow",
      name: "flow",
      component: () => import("@/views/FlowView.vue"),
    },
    {
      path: "/settings",
      name: "settings",
      component: () => import("@/views/SettingsView.vue"),
    },
    {
      path: "/playground",
      component: () => import("@/views/playground/PlaygroundLayoutView.vue"),
      children: [
        {
          path: "",
          name: "playground-overview",
          component: () =>
            import("@/views/playground/PlaygroundOverviewView.vue"),
        },
        {
          path: "prompts",
          name: "playground-prompts",
          component: () =>
            import("@/views/playground/PlaygroundPromptsView.vue"),
        },
        {
          path: "prompts/:promptId",
          name: "playground-prompt-detail",
          component: () =>
            import("@/views/playground/PlaygroundPromptDetailView.vue"),
        },
        {
          path: "datasets",
          name: "playground-datasets",
          component: () =>
            import("@/views/playground/PlaygroundDatasetsView.vue"),
        },
        {
          path: "experiments",
          name: "playground-experiments",
          component: () =>
            import("@/views/playground/PlaygroundExperimentsView.vue"),
        },
        {
          path: "experiments/:experimentId",
          name: "playground-experiment-detail",
          component: () =>
            import("@/views/playground/PlaygroundExperimentDetailView.vue"),
        },
      ],
    },
    {
      path: "/:pathMatch(.*)*",
      name: "not-found",
      component: () => import("@/views/NotFoundView.vue"),
    },
  ],
});

export default router;
