const FormStates = {
    CheckingStatus: "checkingStatus",
    WaitingForCredentials: "waitingForCredentials",
    CredentialsAccepted: "credentialsAccepted",
    CredentialsRejected: "credentialsRejected",
    InvalidCredentials: "invalidCredentials",
    GatheringInventory: "gatheringInventory",
    UpToDate: "upToDate",
    Error: "error",
}

const AgentStates = {
    Connected: "connected",
    NotConnected: "notConnected",
    UpdateSuccessfull: "updateSuccessfull",
    UpdateFailed: "updateFailed",
}

export default FormStates;

