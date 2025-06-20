import Store from "./store.js";

export default function createAgentStateStore() {
    return new Store({
        actions: {
            fetchState(context) {
                axios
                    .get("/api/v1/status")
                    .then(function (response) {
                        context.commit("updateAgentState", response.data);
                    })
                    .catch(function (error) {
                        console.log(error);
                    });
            },
        },
        mutations: {
            updateAgentState(state, agentState) {
                state.agentState = agentState;
                return state;
            },
        },
        state: {
            agentState: {},
        },
    });
}
