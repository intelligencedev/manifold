import { createRouter, createWebHistory } from "vue-router";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/",
      name: "overview",
      meta: {
        nav: true,
        order: 1,
        label: "Overview",
        glyph: "OV",
        title: "Overview",
        purpose: "Platform observability and monitoring",
      },
      component: () => import("@/views/OverviewView.vue"),
    },
    {
      path: "/projects",
      name: "projects",
      meta: {
        nav: true,
        order: 2,
        label: "Projects",
        glyph: "PR",
        title: "Projects",
        purpose: "Isolated workspaces for agents, tools, and data",
      },
      component: () => import("@/views/ProjectsView.vue"),
    },
    {
      path: "/specialists",
      name: "specialists",
      meta: {
        nav: true,
        order: 3,
        label: "Specialists",
        glyph: "SP",
        title: "Specialists",
        purpose: "Specialist configurations and management",
      },
      component: () => import("@/views/SpecialistsView.vue"),
    },
    {
      path: "/chat",
      name: "chat",
      meta: {
        nav: true,
        order: 4,
        label: "Chat",
        glyph: "CH",
        title: "Chat",
        purpose: "Chat sessions with agents, tools, skills and projects",
      },
      component: () => import("@/views/ChatView.vue"),
    },
    {
      path: "/pulse",
      name: "pulse",
      meta: {
        nav: true,
        order: 5,
        label: "Pulse",
        glyph: "PU",
        title: "Pulse",
        purpose: "Scheduled and recurring work",
      },
      component: () => import("@/views/PulseView.vue"),
    },
    {
      path: "/matrix",
      redirect: "/pulse",
    },
    {
      path: "/flow",
      name: "flow",
      meta: {
        nav: true,
        order: 7,
        label: "Flow",
        glyph: "FL",
        title: "Flow",
        purpose: "Visual workflow builder and execution (beta)",
      },
      component: () => import("@/views/FlowView.vue"),
    },
    {
      path: "/durable",
      name: "durable",
      meta: {
        nav: false,
        order: 8,
        label: "Durable",
        glyph: "DU",
        title: "Durable",
        purpose: "Long-horizon durable tasks",
      },
      component: () => import("@/views/DurableView.vue"),
    },
    {
      path: "/durable/tasks/:taskId",
      name: "durable-task",
      component: () => import("@/views/DurableTaskView.vue"),
    },
    {
      path: "/codeqa/:runId?",
      name: "codeqa",
      meta: {
        nav: true,
        order: 9,
        label: "Code QA",
        glyph: "QA",
        title: "Code QA",
        purpose: "(beta)",
      },
      component: () => import("@/views/CodeQaView.vue"),
    },
    {
      path: "/beliefs",
      name: "beliefs",
      meta: {
        nav: true,
        order: 10,
        label: "Beliefs",
        glyph: "BL",
        title: "Beliefs",
        purpose: "(beta)",
      },
      component: () => import("@/views/BeliefsView.vue"),
    },
    {
      path: "/setup",
      name: "setup",
      meta: {
        nav: false,
        title: "Setup",
        purpose: "First-run provider configuration",
      },
      component: () => import("@/views/SetupView.vue"),
    },
    {
      path: "/settings",
      name: "settings",
      meta: {
        nav: true,
        order: 99,
        label: "Settings",
        glyph: "⚙",
        title: "Settings",
        purpose: "Configuration",
      },
      component: () => import("@/views/SettingsView.vue"),
    },
    {
      path: "/playground",
      name: "playground",
      meta: {
        nav: true,
        order: 6,
        label: "Playground",
        glyph: "PL",
        title: "Playground",
        purpose: "Prompt & experiment lab",
      },
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

let setupChecked = false;
let setupReady = true;

router.beforeEach(async (to) => {
  if (to.name === "setup") {
    return true;
  }
  if (!setupChecked) {
    try {
      const res = await fetch("/api/setup/status", { credentials: "include" });
      if (res.ok) {
        const data = await res.json();
        setupReady = Boolean(data?.ready);
      } else {
        setupReady = true;
      }
    } catch {
      setupReady = true;
    }
    setupChecked = true;
  }
  if (!setupReady) {
    return { name: "setup" };
  }
  return true;
});

export default router;
