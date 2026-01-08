import Store from './store.js';
import FormStates from "../commons/states.js";

export default function createStore() {
    return new Store({
        actions: {
            enableDataSharing(context, payload) {
                context.commit("updateDataSharing", payload.enabled);
            },
            fetchAgentState(context) {
                axios.get('/api/v1/status')
                    .then(function(response) {
                        switch (response.data.status) {
                            case "waiting-for-credentials":
                                context.commit("updateFormState", FormStates.WaitingForCredentials);
                                break;
                            case "up-to-date":
                                // remove interval
                                if (context.state.intervalID) {
                                    clearInterval(context.state.intervalID);
                                    context.commit("setIntervalID", null);
                                }
                                context.commit("updateFormState", FormStates.UpToDate);
                        }
                    })
                    .catch(function(error) {
                        console.log(error)
                    });
            },
            fetchAgentVersion(context) {
                axios.get('/api/v1/version')
                    .then(function(response) {
                        context.commit("setVersion", response.data.version);
                    })
                    .catch(function(error) {
                        console.log("failed to get agent's version" + error)
                    });
            },
            fetchPlannerUrl(context) {
                axios.get('/api/v1/url')
                    .then(function(response) {
                        context.commit("setUrl", response.data.url);
                    })
                    .catch(function(error) {
                        console.log("failed to get agent's version" + error)
                    });
            },
            postCredentials(context, payload) {
                context.commit("updateRequestPending", true);
                axios.put('/api/v1/credentials', payload)
                    .then(() => {
                        context.commit('updateFormState', FormStates.CredentialsAccepted);

                        let c = context;
                        let intervalID = setInterval(() => {
                            c.dispatch('fetchAgentState');
                        }, 1000);
                        context.commit('setIntervalID', intervalID);
                    })
                    .catch((error) => {
                        console.log(error.status);
                        switch (error.status) {
                            case 400:
                                context.commit('updateFormState', FormStates.InvalidCredentials);
                                break;
                            case 401:
                                context.commit('updateFormState', FormStates.CredentialsRejected);
                                break;
                            default:
                                context.commit('updateFormState', FormStates.Error);
                        }
                    })
                    .finally(() => {
                        context.commit("updateRequestPending", false);
                    });
            },
            formValidation(context, payload) {
                context.commit('setFormValidation', payload);
            },
            formCredentials(context, creds) {
                context.commit("setCredentials", creds);
            }
        },
        mutations: {
            updateDataSharing(state, enabled) {
                state.dataSharingAccepted = enabled;
                return state;
            },
            updateFormState(state, formState) {
                state.formState = formState;
                return state;
            },
            setVersion(state, version) {
                state.version = version;
                return state;
            },
            updateRequestPending(state, pending) {
                state.requestPending = pending;
                return state;
            },
            setUrl(state, url) {
                state.url = url;
                return state;
            },
            setIntervalID(state, id) {
                state.intervalID = id;
                return state;
            },
            setCredentials(state, creds) {
                state.creds = creds;
                return state;
            },
            setFormValidation(state, payload) {
                state.formValidation = payload;
                return state;
            }
        },
        state: {
            dataSharingAccepted: true,
            requestPending: false,
            version: "",
            url: "",
            creds: { url: "", username: "", password: "" },
            intervalID: null,
            formState: FormStates.CheckingStatus,
            formValidation: { valid: true, element: "" },
        }
    });
}
