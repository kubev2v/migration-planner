import createAgentStateStore from "./store/agent.js";
import createStore from "./store/index.js";
import FormView from "./views/form.js";
import AgentStatusView from "./views/agent-state.js";

const formContainerElement = document.querySelector("#form-container");
const btnContainerElement = document.querySelector("#btn-container");
const statusContainerElement = document.querySelector(
    "#agent-status-container"
);

const store = createStore();
const agentStore = createAgentStateStore();

// Set things up
document.addEventListener("DOMContentLoaded", function () {
    store.dispatch("fetchAgentState");
    store.dispatch("fetchAgentVersion");
    store.dispatch("fetchPlannerUrl");
    agentStore.dispatch("fetchState");

    new FormView(
        store,
        formContainerElement,
        btnContainerElement,
    ).render();

    new AgentStatusView(
        agentStore,
        statusContainerElement
    );

    setInterval(() => {
        agentStore.dispatch("fetchState");
    }, 5000);

});
