<template>
    <div id="manifold">
      <!-- Fixed header with animated cube -->
      <div id="cb-header" class="position-fixed fixed-top dark-blur mt-0">
        <div id="hgradient"></div>
        <div class="box-container" ref="boxContainer">
          <div class="circle"></div>
          <div class="box" ref="box">
            <div class="panel front" :style="{ '--bg-color': 'var(--et-purple)' }"></div>
            <div class="panel back" :style="{ '--bg-color': 'var(--et-green)' }"></div>
            <div class="panel top" :style="{ '--bg-color': 'var(--et-blue)' }"></div>
            <div class="panel bottom" :style="{ '--bg-color': 'var(--et-red)' }"></div>
            <div class="panel left" :style="{ '--bg-color': 'var(--et-yellow)' }"></div>
            <div class="panel right" :style="{ '--bg-color': 'var(--et-light)' }"></div>
          </div>
        </div>
      </div>
  
      <!-- Main content -->
      <div id="content" class="container-fluid p-0 m-0">
        <div class="main-content">
          <div class="row h-100 pt-5">
            <!-- Tools column -->
            <div id="tool-view" class="col-3">
              <div id="tools-container" class="row h-100 show">
                <div class="col h-100">
                  <div class="panel h-100 px-3 d-flex flex-column fade-it rounded-end-2 border-end border-secondary-subtle"
                       style="background-color: var(--et-galactic-bg);">
                    <h4 class="mb-3 text-center">Tools</h4>
                    <div class="panel-body">
                      <div class="accordion accordion-flush" id="toolsAccordion">
                        <!-- Teams Workflow accordion -->
                        <div class="accordion-item">
                          <h2 class="accordion-header">
                            <button class="accordion-button" type="button" data-bs-toggle="collapse"
                                    data-bs-target="#teamsCollapse" aria-expanded="true">
                              Teams Workflow
                            </button>
                          </h2>
                          <div id="teamsCollapse" class="accordion-collapse collapse show">
                            <div class="accordion-body">
                              <div class="form-check-reverse form-switch d-flex justify-content-between align-items-center mb-2">
                                <label class="form-check-label" for="teams-switch">Enable</label>
                                <input class="form-check-input" type="checkbox" role="switch" id="teams-switch"
                                       @click="toggleTeamsTool">
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- Web Search accordion -->
                        <div class="accordion-item">
                          <h2 class="accordion-header">
                            <button class="accordion-button collapsed" type="button" data-bs-toggle="collapse"
                                    data-bs-target="#webSearchCollapse">
                              Web Search
                            </button>
                          </h2>
                          <div id="webSearchCollapse" class="accordion-collapse collapse">
                            <div class="accordion-body">
                              <div class="form-check-reverse form-switch d-flex justify-content-between align-items-center mb-2">
                                <label class="form-check-label" for="websearch-switch">Enable</label>
                                <input class="form-check-input" type="checkbox" role="switch" id="websearch-switch"
                                       @click="toggleWebSearchTool">
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- Web Retrieval accordion -->
                        <div class="accordion-item">
                          <h2 class="accordion-header">
                            <button class="accordion-button collapsed" type="button" data-bs-toggle="collapse"
                                    data-bs-target="#webRetrievalCollapse">
                              Web Retrieval
                            </button>
                          </h2>
                          <div id="webRetrievalCollapse" class="accordion-collapse collapse">
                            <div class="accordion-body">
                              <div class="form-check-reverse form-switch d-flex justify-content-between align-items-center mb-2">
                                <label class="form-check-label" for="webget-switch">Enable</label>
                                <input class="form-check-input" type="checkbox" role="switch" id="webget-switch"
                                       @click="toggleWebGetTool">
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- Retrieval accordion -->
                        <div class="accordion-item">
                          <h2 class="accordion-header">
                            <button class="accordion-button collapsed" type="button" data-bs-toggle="collapse"
                                    data-bs-target="#retrievalCollapse">
                              Retrieval
                            </button>
                          </h2>
                          <div id="retrievalCollapse" class="accordion-collapse collapse">
                            <div class="accordion-body">
                              <div class="form-check-reverse form-switch d-flex justify-content-between align-items-center mb-2">
                                <label class="form-check-label" for="retrieval-switch">Enable</label>
                                <input class="form-check-input" type="checkbox" role="switch" id="retrieval-switch"
                                       @click="toggleRetrievalTool">
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- End accordion items -->
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
  
            <!-- Chat view column -->
            <div id="chat-view" class="col-6" hx-ext="ws" ws-connect="/ws">
              <div id="chat" class="row chat-container fs-5"></div>
            </div>
  
            <!-- Assistants/info column -->
            <div id="info" class="col-3 show">
              <div id="model-container" class="content-wrapper h-100 d-flex flex-column rounded-start-2 border-start border-secondary-subtle">
                <div class="content h-100">
                  <div class="container-fluid h-100">
                    <div class="row h-100">
                      <div class="col-12 h-100">
                        <div class="panel h-100 d-flex flex-column fade-it" style="background-color: var(--et-galactic-bg);">
                          <div class="panel-header px-4 py-3 border-bottom">
                            <h4 class="mb-3 text-center">Assistants</h4>
                            <div class="text-center mb-3">
                              <img id="assistant-avatar" src="/img/et_female.jpg" alt="Assistant Avatar" class="rounded-circle"
                                   style="width: 100px; height: 100px;">
                            </div>
                            <div class="mb-3">
                              <label for="role-select" class="form-label">Assistant Role</label>
                              <select id="role-select" class="form-select" aria-label="Select Assistant Role" @change="setRole($event.target.value)">
                                <option disabled>Select Role</option>
                                <option v-for="role in roles" :key="role.Name" :value="role.Name">
                                  {{ role.Name }}
                                </option>
                              </select>
                            </div>
                            <select id="model-select" class="form-select" aria-label="Select Language Model" @change="handleModelChange">
                              <option disabled>Language Models</option>
                              <option v-for="model in languageModels" :key="model.name" :value="model.name" :selected="model.name === selectedModel">
                                {{ model.name }}
                              </option>
                            </select>
                          </div>
                          <div id="model-info-container" class="panel-body flex-grow-1 overflow-auto px-4 py-3">
                            <!-- Model info content goes here -->
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <!-- End Assistants/info column -->
          </div>
        </div>
  
        <!-- Bottom bar with prompt -->
        <div class="row">
          <div class="w-25"></div>
          <div class="w-50 mt-2 pb-2 bottom-bar shadow-lg rounded-top-2" style="background-color: var(--et-card-bg);">
            <form>
              <div class="py-1" id="prompt-view">
                <div class="row">
                  <div class="col m-0 p-0">
                    <button class="btn fw-medium w-100 m-0 p-0" data-bs-toggle="/">
                      <span class="fs-4">
                        M
                        <svg width="16" height="16" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                          <!-- SVG content for the icon -->
                        </svg>
                        nifold
                      </span>
                    </button>
                  </div>
                </div>
              </div>
              <div class="hstack">
                <div class="input-group mx-1 dropup dropup-center">
                  <button id="settingsBtn" class="btn btn-secondary bg-gradient" data-bs-target="#modal-settings"
                          data-bs-toggle="tooltip" data-bs-title="Settings" @click="toggleSettings">
                    <svg width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                      <!-- SVG content for settings -->
                    </svg>
                  </button>
                  <button class="btn btn-secondary bg-gradient" id="upload">
                    <svg width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                      <!-- SVG content for upload -->
                    </svg>
                  </button>
                  <input type="file" id="file-input" style="display: none;" />
                  <textarea id="message" name="userprompt" class="col form-control shadow-none"
                            placeholder="Type your message..." rows="2" style="outline: none;">write a haiku</textarea>
                  <input type="hidden" name="role_instructions" :value="roleInstructions">
                  <input type="hidden" name="model" :value="selectedModel">
                  <button id="send" class="btn btn-secondary btn-prompt-send bg-gradient" type="button"
                          hx-post="/v1/chat/submit" hx-target="#chat" hx-swap="beforeend scroll:bottom"
                          @click="handleSend">
                    <div id="send-icon">
                      <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
                        <!-- SVG content for send icon -->
                      </svg>
                    </div>
                  </button>
                </div>
              </div>
            </form>
          </div>
          <div class="w-25"></div>
        </div>
  
        <!-- Scroll-to-bottom button -->
        <div id="scroll-to-bottom-btn" class="btn btn-primary bg-gradient"
             style="display: none; position: absolute; bottom: 100px; left: 50%; transform: translateX(-50%); background-color: var(--et-btn-info);">
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">
            <g fill="currentColor">
              <!-- SVG paths for scroll button -->
            </g>
          </svg>
        </div>
      </div>
    </div>
  </template>
  
  <script>
  export default {
    name: "Manifold",
    data() {
      return {
        r: 45,
        roles: [],          // Populate this with role data as needed
        languageModels: [], // Populate with model data from your API
        selectedModel: null,
        roleInstructions: ""
      };
    },
    mounted() {
      // Set up event listeners for the animated cube
      const boxContainer = this.$refs.boxContainer;
      const box = this.$refs.box;
      if (boxContainer && box) {
        boxContainer.addEventListener("click", () => {
          const newRotation = this.getRandomRotation();
          box.style.transition = "transform 0.5s";
          box.style.transform = newRotation;
        });
        box.addEventListener("mouseover", () => {
          if (box.hasAttribute("data-event-added")) return;
          box.setAttribute("data-event-added", true);
          const newRotation = this.getRandomRotation();
          box.style.transition = "transform 0.5s ease";
          box.style.transform = newRotation;
          setTimeout(() => {
            box.removeAttribute("data-event-added");
          }, 1000);
        });
        // Apply an initial rotation
        const initialRotation = this.getRandomRotation();
        box.style.transition = "transform 0.5s ease";
        box.style.transform = initialRotation;
      }
  
      // Fetch tool configuration data once the component mounts
      this.fetchToolData();
  
      // (Any additional initialization – such as initializing third-party libraries – can be done here)
    },
    methods: {
      getRandomRotation() {
        this.r += -90;
        const rotationX = this.r;
        const rotationY = this.r;
        const rotationZ = -180;
        return `rotateX(${rotationX}deg) rotateY(${rotationY}deg) rotateZ(${rotationZ}deg)`;
      },
      chatRotation() {
        this.r += 10;
        const rotationX = this.r;
        const rotationY = this.r;
        const rotationZ = 0;
        return `rotateX(${rotationX}deg) rotateY(${rotationY}deg) rotateZ(${rotationZ}deg)`;
      },
      fetchToolData() {
        fetch(`/v1/tools/list`)
          .then(response => response.json())
          .then(data => {
            data.forEach(tool => {
              localStorage.setItem(tool.Name, tool.Enabled);
              this.setToolToggleState(tool.Name, tool.Enabled);
            });
          })
          .catch(error => {
            console.error("Error fetching tool data:", error);
            alert("Failed to fetch tool data. Please try again.");
          });
      },
      setToolToggleState(toolName, isEnabled) {
        const switchElement = document.getElementById(`${toolName}-switch`);
        if (switchElement) {
          switchElement.checked = isEnabled;
        }
      },
      toggleTool(toolName, isEnabled) {
        console.log(`Toggling ${toolName} to ${isEnabled ? "enabled" : "disabled"}`);
        localStorage.setItem(toolName, isEnabled);
        fetch(`/v1/tools/${toolName}/toggle`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled: isEnabled })
        })
          .then(response => response.json())
          .then(data => {
            console.log(`${toolName} is now ${isEnabled ? "enabled" : "disabled"}`);
          })
          .catch(error => {
            console.error(`Error toggling ${toolName}:`, error);
            alert(`Failed to toggle ${toolName}. Please try again.`);
          });
      },
      toggleTeamsTool() {
        const isEnabled = document.getElementById("teams-switch").checked;
        this.toggleTool("teams", isEnabled);
      },
      toggleWebSearchTool() {
        const isEnabled = document.getElementById("websearch-switch").checked;
        this.toggleTool("websearch", isEnabled);
      },
      toggleWebGetTool() {
        const isEnabled = document.getElementById("webget-switch").checked;
        this.toggleTool("webget", isEnabled);
      },
      toggleRetrievalTool() {
        const isEnabled = document.getElementById("retrieval-switch").checked;
        this.toggleTool("retrieval", isEnabled);
      },
      setRole(role) {
        // Update the selected role and related UI elements
        this.selectedRole = role;
        this.updateAvatar(role);
        const selected = this.roles.find(r => r.Name === role);
        if (selected && selected.Instructions) {
          this.roleInstructions = selected.Instructions;
          console.log("roleInstructions:", selected.Instructions);
        }
        console.log("Selected Role:", role);
      },
      updateAvatar(role) {
        const avatarElement = document.getElementById("assistant-avatar");
        const avatarPaths = {
          chat: "/img/et_female.jpg",
          summary: "/img/et_male.jpg",
          cot: "/img/et_female.jpg",
          cot_advanced: "/img/et_male.jpg",
          software_dev: "/img/et_female.jpg",
          code_review: "/img/et_male.jpg",
          image_bot: "/img/et_female.jpg"
        };
        if (avatarElement) {
          avatarElement.src = avatarPaths[role] || "/img/et_female.jpg";
        }
      },
      handleModelChange(event) {
        this.selectedModel = event.target.value;
        localStorage.setItem("selectedModel", this.selectedModel);
        // Update any additional state if necessary
      },
      handleSend() {
        // Implement your send-message logic here.
        // Then clear the textarea.
        const messageEl = document.getElementById("message");
        if (messageEl) {
          messageEl.value = "";
        }
      },
      toggleSettings() {
        // Implement settings toggle logic as needed.
        console.log("Toggling settings");
      }
    }
  };
  </script>
  
  <style scoped>
  #tools,
  #chat-view {
    height: 100%;
    overflow-y: auto;
  }
  
  body,
  html {
    height: 100%;
    margin: 0;
  }
  
  #content {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }
  
  .bottom-bar {
    flex-shrink: 0;
  }
  
  /* Additional component-specific styles can go here */
  </style>
  