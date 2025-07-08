import Observable from "./observable.js";
import ConnectionStatus from "../components/status.js";
import UpdateStatus from "../components/update_status.js";

export default class AgentStatusView extends Observable {
    constructor(store, statusElement) {
        super({
            store: store,
        });

        let self = this;
        self.statusElement = statusElement;
    }

    clean() {
        let self = this;
        self.statusElement.innerHTML = "";
    }

    render() {
        let self = this;

        self.clean();

        const agentState = self.store.state.agentState;
        self.statusElement.appendChild(
            new ConnectionStatus({
                isConnected: agentState.connected,
            }).render()
        );
        if (agentState.connected == "true") {
            if (agentState.agentStateUpdateSuccessful) {
                self.statusElement.appendChild(
                    new UpdateStatus({
                        updateSuccessful: agentState.agentStateUpdateSuccessful,
                        message: agentState.agentStateUpdateErrMessage,
                        successMessage: "Agent status update successfully.",
                    }).render()
                );
            }
            if (agentState.inventoryUpdateSuccessful) {
                self.statusElement.appendChild(
                    new UpdateStatus({
                        updateSuccessful: agentState.inventoryUpdateSuccessful,
                        message: agentState.inventoryUpdateErrMessage,
                        successMessage:
                            "Inventory successfully uploaded to console.redhat.com.",
                    }).render()
                );
            }
        }
    }
}
